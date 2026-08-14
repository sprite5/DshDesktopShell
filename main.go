package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"dshshell/internal/api"
	"dshshell/internal/host"
	"dshshell/internal/settings"
	"dshshell/internal/tray"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"gopkg.in/yaml.v3"
)

//go:embed all:assets
var assetsFS embed.FS

//go:embed assets/logo.png
var logoPNG []byte

//go:embed assets/tray.png
var trayPNG []byte

//go:embed assets/tray-template.png
var trayTemplatePNG []byte // macOS menu-bar template icon (monochrome)

//go:embed build/config.yml
var buildConfigYAML []byte

// buildConfig mirrors the wails build/config.yml info block so the app
// version has a single source of truth.
type buildConfig struct {
	Info struct {
		Version string `yaml:"version"`
	} `yaml:"info"`
}

// appVersion extracts info.version from the embedded build/config.yml.
func appVersion(configYAML []byte) string {
	var cfg buildConfig
	if err := yaml.Unmarshal(configYAML, &cfg); err != nil {
		log.Printf("[main] parse build/config.yml version: %v", err)
		return ""
	}
	return cfg.Info.Version
}

// resolveDataRoot picks where shell data lives: a portable data/ directory
// next to the executable on Windows/Linux (whole app + data movable as one
// folder), the system user-data directory on macOS. Override with
// DSH_SHELL_DATA_ROOT.
func resolveDataRoot(goos, exePath string) string {
	if root := os.Getenv("DSH_SHELL_DATA_ROOT"); root != "" {
		return root
	}
	if goos == "darwin" {
		return filepath.Join(application.Path(application.PathDataHome), "app.dsh.desktop")
	}
	return filepath.Join(filepath.Dir(exePath), "data")
}

func main() {
	var (
		flagURL      = flag.String("url", "", "DSH Web 地址：本次启动直接打开并保存为外部连接")
		flagManaged  = flag.Bool("managed", false, "托管模式：随程序启动并管理本地 dsh 进程")
		flagSettings = flag.Bool("settings", false, "忽略保存的连接，强制打开设置页")
		flagReset    = flag.Bool("reset", false, "清除保存的连接并打开设置页")
	)
	flag.Parse()

	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("locate executable: %v", err)
	}
	dataRoot := resolveDataRoot(runtime.GOOS, exe)

	// Log to a file next to the data (no console attached in GUI builds).
	logPath := filepath.Join(dataRoot, ".dsh-shell", "dsh-shell.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		log.Printf("[main] mkdir log dir: %v", err)
	}
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		log.SetOutput(f)
	}
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("[main] data root: %s", dataRoot)
	log.Printf("[main] env: DSH_BIN=%q APPDATA=%q", os.Getenv("DSH_BIN"), os.Getenv("APPDATA"))
	if r, err := host.Resolve(); err != nil {
		log.Printf("[main] dsh resolve: NOT FOUND (%v)", err)
		log.Printf("[main] PATH=%s", os.Getenv("PATH"))
	} else {
		log.Printf("[main] dsh resolve: source=%s bin=%q prefix=%v", r.Source, r.Bin, r.Prefix)
	}

	store, err := settings.NewStore(dataRoot)
	if err != nil {
		log.Fatalf("open settings: %v", err)
	}
	version := appVersion(buildConfigYAML)

	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		log.Fatalf("embed assets: %v", err)
	}

	var win *application.WebviewWindow
	var tr *tray.Tray
	var currentTarget string // "" = local settings page

	// managed is the optional dsh child the shell owns. Callbacks run on
	// internal goroutines; Wails window/menu methods are concurrency-safe.
	managed := host.New(func(ev host.Event) {
		switch ev.State {
		case host.StateRunning:
			if err := store.SetURL(ev.URL, settings.ModeManaged); err != nil {
				log.Printf("[host] save managed url: %v", err)
			}
			currentTarget = ev.URL
			if win != nil {
				win.SetURL(ev.URL)
			}
			if tr != nil {
				tr.Update(settings.ModeManaged, ev.URL, true)
			}
			log.Printf("[host] dsh up at %s", ev.URL)
		case host.StateExited:
			log.Printf("[host] dsh exited: %v", ev.Err)
			if tr != nil {
				tr.Update(settings.ModeManaged, "(dsh 已退出)", false)
			}
			// If the window was pointed at the managed dsh, fall back to the
			// settings page so the user sees the status instead of a dead page.
			if win != nil && ev.URL != "" && currentTarget == ev.URL {
				win.SetURL("/")
				currentTarget = ""
			}
		case host.StateFailed:
			log.Printf("[host] dsh failed: %v", ev.Err)
			if tr != nil {
				tr.Update(settings.ModeManaged, "(dsh 启动失败)", false)
			}
			if win != nil {
				win.SetURL("/")
				currentTarget = ""
			}
		}
	})

	// startManagedAsync launches the managed dsh without blocking startup.
	startManagedAsync := func() {
		if managed.Running() {
			return
		}
		if _, err := host.Resolve(); err != nil {
			log.Printf("[host] start requested but dsh not resolvable: %v", err)
			return
		}
		if tr != nil {
			tr.Update(settings.ModeManaged, "(正在启动 dsh…)", false)
		}
		go func() {
			if err := managed.Start(); err != nil {
				log.Printf("[host] start failed: %v", err)
				if tr != nil {
					tr.Update(settings.ModeManaged, "(dsh 启动失败)", false)
				}
				if win != nil {
					win.SetURL("/")
					currentTarget = ""
				}
			}
		}()
	}

	// Decide the start URL.
	startURL := "/" // settings page
	switch {
	case *flagReset:
		if err := store.ResetURL(); err != nil {
			log.Printf("[main] reset url: %v", err)
		}
	case *flagURL != "":
		u, err := application.ValidateAndSanitizeURL(*flagURL)
		if err != nil {
			log.Fatalf("invalid --url: %v", err)
		}
		if err := store.SetURL(u, settings.ModeExternal); err != nil {
			log.Printf("[main] save url: %v", err)
		}
		startURL = u
		currentTarget = u
	case *flagManaged:
		if _, err := host.Resolve(); err != nil {
			log.Printf("[main] --managed requested but dsh not resolvable: %v", err)
		} else {
			startManagedAsync()
		}
	case !*flagSettings:
		if store.Mode() == settings.ModeManaged {
			if _, err := host.Resolve(); err != nil {
				log.Printf("[main] managed restore but dsh not resolvable: %v", err)
			} else {
				// Restore managed mode: spawn dsh again, window follows via callback.
				startManagedAsync()
			}
		} else {
			saved := store.URL()
			if saved != "" {
				if res := api.ProbeURL(saved); res.OK {
					startURL = saved
					currentTarget = saved
				} else {
					log.Printf("[main] saved url %s unreachable (%s), opening settings", saved, res.Error)
				}
			}
		}
	}
	log.Printf("[main] start url: %s", startURL)


	app := application.New(application.Options{
		Name:        "DSH Desktop",
		Description: "DeepSeek Harness Web 桌面壳",
		Icon:        logoPNG,
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(sub),
			Middleware: api.Middleware(api.Env{
				Store:    store,
				Version:  version,
				Navigate: func(u string) {
					currentTarget = u
					if win != nil {
						win.SetURL(u)
					}
					if tr != nil {
						tr.Update(settings.ModeExternal, u, false)
					}
				},
				DSHDetected: host.DetectDSH,
				DSHResolve: func() (string, string) {
					r, err := host.Resolve()
					if err != nil {
						return "", err.Error()
					}
					return r.Source, ""
				},
				DSHRunning:  func() bool { return managed.Running() },
				DSHState:    func() string { return string(managed.State()) },
				DSHLogs:     managed.Logs,
				StartDSH: func() error {
					if managed.Running() {
						return nil
					}
					if _, err := host.Resolve(); err != nil {
						return err
					}
					startManagedAsync()
					return nil
				},
			}),
		},
		Mac: application.MacOptions{
			// Tray-persistent app: closing (or hiding) the window keeps the app
			// alive so the tray menu stays available. The close guard in the
			// WindowClosing hook handles hide-to-tray on every platform.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		OnShutdown: func() {
			// Managed dsh dies with the app; external/remote dsh is left alone.
			managed.Stop()
		},
	})

	savedWin := store.Window()
	win = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "DSH Desktop",
		Width:            savedWin.Width,
		Height:           savedWin.Height,
		MinWidth:         900,
		MinHeight:        600,
		BackgroundColour: application.NewRGB(15, 17, 21),
		URL:              startURL,
	})

	tr = tray.New(app, win, trayPNG, trayTemplatePNG, dataRoot, func() {
		go func() {
			if err := managed.Restart(); err != nil {
				log.Printf("[host] restart failed: %v", err)
			}
		}()
	})
	if tr != nil {
		tr.Update(store.Mode(), store.URL(), managed.Running())
	}

	// Persist window size on every resize (WindowClosing sees Size()==0).
	win.RegisterHook(events.Common.WindowDidResize, func(event *application.WindowEvent) {
		w, h := win.Size()
		if w > 0 && h > 0 {
			if err := store.SetWindow(settings.WindowState{Width: w, Height: h}); err != nil {
				log.Printf("[window] save size: %v", err)
			}
		}
	})

	// System tray: close hides to tray instead of quitting; Quit lives in the tray menu.
	if tr != nil {
		win.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
			event.Cancel()
			win.Hide()
		})
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}