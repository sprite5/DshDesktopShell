# DSH Desktop Shell 构建文档

> 极简 Wails v3 + Go 桌面壳：webview 直连 DeepSeek Harness Web GUI。
> 本文件覆盖 **Windows** 与 **macOS** 两个平台的完整构建步骤：环境准备（Go / Node.js / wails3 CLI / 系统依赖）、构建命令、产物与自测。

## 0. 一句话流程

- **Windows**（已验证）：装 Go → `go install` 装 wails3 CLI → `.\build.ps1` → 产物 `bin\dsh-shell.exe`
- **macOS**（须在 Mac 上，未实测）：装 Xcode CLT + Go → `go install` 装 wails3 CLI → `wails3 build -clean -platform darwin -skipbindings` → 产物 `build/bin/DSH Desktop.app`

> macOS 无法在 Windows 上交叉编译（Wails v3 走 cgo / Cocoa），必须在 Mac 上构建。

---

## 1. 环境要求（三件套 + 系统依赖）

| 组件 | 版本 | 是否必需 | 官方下载 / 安装 |
|---|---|---|---|
| **Go** | ≥ 1.26（本仓库 go.mod 为 1.26.4） | ✅ 必需 | <https://go.dev/dl/> |
| **wails3 CLI** | v3.0.0-beta.6（与 go.mod 一致） | ✅ 必需 | `go install`（见 1.2，无独立安装包） |
| **Node.js** | ≥ 20（LTS 即可） | ⭕ 可选，仅图标再生成需要 | <https://nodejs.org/> |
| **WebView2 Runtime**（Windows） | 常青版 | ✅ 必需（Win10/11 一般自带） | <https://developer.microsoft.com/microsoft-edge/webview2/> |
| **Xcode Command Line Tools**（macOS） | 最新 | ✅ 必需（编译要 clang/ld） | `xcode-select --install` |

### 1.1 Go（核心编译工具，必装）

去官方下载页选择当前平台安装包，安装后**重开终端**再验证：

| 平台 | 安装包 | 官方地址 |
|---|---|---|
| Windows x64 | `go1.26.x.windows-amd64.msi`（一路下一步，自动写 PATH） | <https://go.dev/dl/> |
| macOS Apple Silicon | `go1.26.x.darwin-arm64.pkg` | <https://go.dev/dl/> |
| macOS Intel | `go1.26.x.darwin-amd64.pkg` | <https://go.dev/dl/> |

验证：

```powershell
go version   # 期望 go version go1.26.x windows/amd64（或 darwin/arm64）
```

### 1.2 wails3 CLI（Wails v3 命令行，必装）

Wails v3 没有独立安装包，官方安装方式就是 Go 自带的 `go install`。**版本务必与 go.mod 里的 Wails 库一致**（v3.0.0-beta.6）：

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.6
```

- 安装位置：`%GOPATH%\bin`（Windows）/ `$GOPATH/bin`（macOS），**需确保该目录在 PATH 中**（Windows 上默认 `C:\Users\<你>\go\bin`）。
- 验证：

```bash
wails3 --help                  # 能看到 v3 系列命令
wails3 tool buildinfo          # 显示 Version: v3.0.0-beta.6
```

### 1.3 Node.js（可选）

普通构建**不需要** Node.js。只有在需要**重新生成应用图标**（改了品牌图标时）才用：`node scripts/gen-icon.cjs`（依赖 DSH checkout 里的 sharp，路径在脚本头部硬编码；图标产物 `build/appicon.png`、`build/windows/icon.ico`、`assets/*.png` 已入库，跳过不影响构建）。

官方下载：<https://nodejs.org/>（选 LTS 即可），或 `winget install OpenJS.NodeJS.LTS`（Windows）。

### 1.4 系统依赖

- **Windows — WebView2 Runtime**：Win10/11 一般随 Edge 自带。若应用打开白屏/报错，去 <https://developer.microsoft.com/microsoft-edge/webview2/> 装 Evergreen（常青版）即可。
- **macOS — Xcode Command Line Tools**：Wails v3 编译要走 cgo，必须有 clang/ld：

```bash
xcode-select --install   # 弹窗确认；已有则提示 already installed
```

---

## 2. 环境自检（开始前跑一遍）

```bash
go version        # 1.26+
node --version    # 可选；20+
wails3 --help     # v3 可用
go env GOPATH     # wails3 应装在 GOPATH\bin
```

缺哪个装哪个，全部就绪再进下一步。

---

## 3. Windows 构建（已验证 ✅）

### 3.1 发布构建（推荐）

在仓库根目录执行：

```powershell
.\build.ps1
```

脚本自动做两件事：

1. `wails3 generate syso`：生成 exe 资源（图标 / 清单 / 版本信息），输出 `wails_windows_amd64.syso` 供链接；
2. `go build -tags production -ldflags "-s -w -H windowsgui"`：无控制台窗口的发布版。

**产物**：

| 文件 | 说明 |
|---|---|
| `bin\dsh-shell.exe` | 发布版（production tag，关闭 devtools） |
| `bin\dsh-shell-debug.exe` | 调试版（无控制台窗口，日志写 `bin\data\.dsh-shell\dsh-shell.log`） |

### 3.2 只想要调试 / 控制台变体

```powershell
# 调试版（无控制台窗口）
go build -ldflags "-H windowsgui" -o bin\dsh-shell-debug.exe .

# 要实时看日志的控制台变体
go build -o bin\dsh-shell-console.exe .
```

### 3.3 运行

```text
bin\dsh-shell.exe                                # 直连本机 dsh（地址不可达则打开设置页）
bin\dsh-shell.exe --url http://127.0.0.1:3080    # 指定地址并保存
bin\dsh-shell.exe --settings                     # 忽略保存的地址，打开设置页
bin\dsh-shell.exe --reset                        # 清除保存的地址并打开设置页
bin\dsh-shell.exe --managed                      # 托管模式：由壳启动并管理本地 dsh
```

数据目录：exe 旁的 `bin\data\`（可整体拷贝即便携）；覆盖用环境变量 `DSH_SHELL_DATA_ROOT`。

### 3.4 自测

```powershell
go test ./...
go vet ./...
```

---

## 4. macOS 构建（须在 Mac 上，⚠ 未实测）

### 4.1 步骤

```bash
# 1. Xcode 命令行工具
xcode-select --install

# 2. Go（官方 pkg 安装包）
#    https://go.dev/dl/  → go1.26.x.darwin-arm64.pkg（Apple Silicon）或 darwin-amd64.pkg（Intel）

# 3. wails3 CLI（版本与 go.mod 一致）
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.6

# 4. 在仓库根目录构建 .app
wails3 build -clean -platform darwin -skipbindings
```

**产物**：`build/bin/DSH Desktop.app`
**数据目录**：`~/Library/Application Support/app.dsh.desktop`
**ATS**：`build/darwin/Info.plist` 已配置 `NSAllowsLocalNetworking`，放行局域网 http（127.0.0.1 本就豁免）。

### 4.2 注意事项（务必先看）

- **版本匹配**：本仓库依赖 Wails v3.0.0-beta.6，wails3 CLI 也要装同版本，否则可能行为不一致。
- **Taskfile**：beta.6 的 `wails3 build` 通过 Taskfile（task runner）派发。本仓库是手写的最小壳、**未提交 Taskfile.yml**。若报 `task: No Taskfile found`，二选一：
  1. 先 `wails3 generate build-assets -dir .` 生成 Taskfile.yml 与各平台构建资产（注意：生成的 `build/darwin/Info.plist` 可能与仓库手写版不同，需手工补回 ATS 局域网放行），再重试 `wails3 build`；
  2. 或直接用 4.3 的“go build + 手动打包 .app”备选方案。
- **未签名**：本仓库未配置 Developer ID 签名。双击打开若被 Gatekeeper 拦截：右键 → 打开；或 `xattr -dr com.apple.quarantine "build/bin/DSH Desktop.app"`。分发正式版需 Apple 开发者账号签名 + 公证（notarization）。

### 4.3 备选：直接 go build + 手动打包 .app（不依赖 Taskfile）

如果 wails3 打包链路不顺，可以用最小步骤出可运行的 .app（仓库已提供 Info.plist）：

```bash
# 1. 编译 darwin 二进制（在本机 Mac 上，CGO 默认开启）
go build -tags production -ldflags "-s -w" -o build/bin/dsh-shell .

# 2. 组装 .app 目录
APP="build/bin/DSH Desktop.app"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp build/bin/dsh-shell "$APP/Contents/MacOS/dsh-shell"
cp build/darwin/Info.plist "$APP/Contents/Info.plist"
# 图标（可选）：把 build/appicon.png 转成 icons.icns 放入 Resources/，
# 并确保 Info.plist 的 CFBundleIconFile 指向它。

# 3. 运行
open "$APP"
```

> 该备选路径结构正确但未在本仓库实测，第一次请以 4.1 官方链路为准。

---

## 5. 版本与产物速查

- **版本单一来源**：`build/config.yml` 的 `info.version`（main.go 启动时读取；`build/darwin/Info.plist` 的 `CFBundleShortVersionString` 与之对应）。
- **改版本号**：编辑 `build/config.yml` → Windows 重跑 `build.ps1`（自动重生成 syso 版本资源）。
- **重新生成图标**（可选）：`node scripts/gen-icon.cjs`，需要 DSH checkout 的 sharp。
- **自测命令**：`go test ./...`、`go vet ./...`。

---

## 6. 常见问题（FAQ）

| 现象 | 原因 / 解决 |
|---|---|
| `'wails3' 不是内部或外部命令` | `%GOPATH%\bin`（`go env GOPATH` 查）不在 PATH；加到 PATH 后重开终端 |
| `go: go.mod requires go >= 1.26.4` | Go 版本太低，去 <https://go.dev/dl/> 装新版 |
| 应用打开白屏 / WebView2 报错 | 装 WebView2 Evergreen：<https://developer.microsoft.com/microsoft-edge/webview2/> |
| 想交叉编译 macOS 报 cgo 错误 | Wails v3 走 cgo，**不支持**跨平台编译，请在 Mac 上构建 |
| exe 图标/版本没更新 | 删掉根目录 `wails_windows_amd64.syso` 后重跑 `build.ps1`（会自动 `wails3 generate syso`） |
| 托盘/设置页改动后没生效 | assets/ 是 `//go:embed` 内嵌，改动后**必须重新 `go build`**，无需任何前端打包 |
| `task: No Taskfile found`（macOS） | 见 4.2，用 `wails3 generate build-assets` 或 4.3 备选方案 |

---

## 7. 官方下载链接汇总

| 组件 | 链接 |
|---|---|
| Go | <https://go.dev/dl/> |
| Node.js | <https://nodejs.org/> |
| WebView2 Runtime（Windows） | <https://developer.microsoft.com/microsoft-edge/webview2/> |
| Xcode（macOS） | <https://developer.apple.com/xcode/>（CLT 用 `xcode-select --install`） |
| Wails v3（wails3 CLI） | `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.6`（文档：<https://v3.wails.io/>） |

