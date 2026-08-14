package tray

import (
	"os/exec"
	"runtime"

	"dshshell/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Tray owns the system-tray icon and menu. The shell keeps running in the
// tray when the window is closed; only the tray Quit item really exits.
type Tray struct {
	app         *application.App
	win         *application.WebviewWindow
	menu        *application.Menu
	stateLabel  *application.MenuItem
	restartItem *application.MenuItem
	restart     func()
}

// New creates the tray with a DSH-brand icon, wires the toggle-click and
// menu actions, and returns the handle (nil when the tray is unavailable).
// restart is invoked by the "重启 dsh" item (only enabled in managed mode).
//
// Platform differences:
//   - Windows/Linux: left-click toggles the window, right-click opens the menu.
//   - macOS: the menu bar icon uses a monochrome template image (the system
//     tints it for light/dark menu bars); left-click opens the native menu,
//     matching usual macOS menu-bar-extra behavior — the window is toggled
//     from the menu instead.
func New(app *application.App, win *application.WebviewWindow, icon, templateIcon []byte, dataRoot string, restart func()) *Tray {
	tray := app.SystemTray.New()
	if tray == nil {
		return nil // tray unavailable on this platform — app keeps working without it
	}
	t := &Tray{app: app, win: win, restart: restart}
	if runtime.GOOS == "darwin" {
		if len(templateIcon) > 0 {
			tray.SetTemplateIcon(templateIcon)
		} else {
			tray.SetIcon(icon)
		}
	} else {
		tray.SetIcon(icon)
		tray.OnClick(func() { t.toggleWindow() })
	}
	tray.SetTooltip("DSH Desktop")

	menu := app.NewMenu()
	menu.Add("显示/隐藏窗口").OnClick(func(*application.Context) { t.toggleWindow() })
	menu.Add("重新加载").OnClick(func(*application.Context) { win.Reload() })
	menu.Add("连接设置…").OnClick(func(*application.Context) { win.SetURL("/") })
	t.restartItem = menu.Add("重启 dsh")
	t.restartItem.OnClick(func(*application.Context) {
		if t.restart != nil {
			t.restart()
		}
	})
	menu.AddSeparator()
	t.stateLabel = menu.Add("状态: 未连接")
	t.stateLabel.SetEnabled(false)
	menu.Add("打开数据目录").OnClick(func(*application.Context) { openFolder(dataRoot) })
	menu.AddSeparator()
	menu.Add("退出").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)
	t.menu = menu
	return t
}

// Update refreshes the state line and the managed-restart availability.
// managedRunning only enables "重启 dsh" in managed mode — external/remote
// connections are never restarted by this shell.
func (t *Tray) Update(mode, url string, managedRunning bool) {
	if t == nil {
		return
	}
	prefix := "外部连接"
	if mode == settings.ModeManaged {
		prefix = "托管 dsh"
	}
	if url == "" {
		url = "(未连接)"
	}
	t.stateLabel.SetLabel("状态: " + prefix + " · " + url)
	t.restartItem.SetEnabled(mode == settings.ModeManaged && managedRunning)
	t.menu.Update()
}

func (t *Tray) toggleWindow() {
	if t.win == nil {
		return
	}
	if t.win.IsVisible() {
		t.win.Hide()
	} else {
		t.win.Show()
		t.win.Focus()
	}
}

// openFolder reveals a directory in the platform file manager.
func openFolder(dir string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	_ = cmd.Start()
}
