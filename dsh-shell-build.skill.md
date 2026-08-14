---
name: dsh-shell-build
description: 构建 DSH Desktop Shell（Wails v3 + Go 桌面壳）的完整流程：Windows/macOS 环境准备（Go、Node.js、wails3 CLI，缺则指引去官方下载安装）、执行构建、验证产物与自测。
whenToUse: 当需要在 Windows 或 macOS 上构建、重新构建或排查 DSH Desktop Shell（本仓库 dshshell）的构建问题时使用。完整手册见仓库根目录 BUILD.md。
metadata:
  version: 1.0.0
---

# 构建 DSH Desktop Shell

本仓库是 Wails v3 + Go 桌面壳（webview 直连 DeepSeek Harness Web GUI），Go 模块名 dshshell，
依赖 Go 1.26.4+ 与 github.com/wailsapp/wails/v3 v3.0.0-beta.6。
完整手册在仓库根目录 **BUILD.md**（含官方下载链接、FAQ），本技能是执行版，冲突时以 BUILD.md 为准。

## 第一步：确定目标平台

- Windows：本机直接构建（已验证）。
- macOS：**必须在 Mac 上构建**（Wails v3 走 cgo/Cocoa，Windows 无法交叉编译）。
  若当前不在 Mac 上，向用户说明需要 Mac，结束。

## 第二步：环境准备（缺啥装啥）

依次检查，缺失时**先提示用户去官方下载安装，或由你执行官方命令**，装完重开终端再验证：

1. **Go ≥ 1.26**：`go version` 验证。
   - 缺失 → 官方下载 <https://go.dev/dl/>（Windows 用 msi；macOS 用 darwin-arm64/pkg 或 darwin-amd64/pkg）。
2. **wails3 CLI v3.0.0-beta.6**：`wails3 --help` / `wails3 tool buildinfo` 验证（版本须与 go.mod 一致）。
   - 缺失或版本不对 → 官方安装方式就是 go install：
     ```bash
     go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.6
     ```
   - 注意：装到 `%GOPATH%\bin`（Windows）/ `$GOPATH/bin`（macOS），需在 PATH 中。
3. **Node.js（可选）**：仅重新生成图标时需要（`node scripts/gen-icon.cjs`，依赖 DSH checkout 的 sharp）。
   缺失且有图标需求 → 官方 <https://nodejs.org/>（LTS）。
4. **系统依赖**：
   - Windows：WebView2 Runtime（Win10/11 一般自带；缺失去 <https://developer.microsoft.com/microsoft-edge/webview2/>）。
   - macOS：`xcode-select --install` 安装 Xcode Command Line Tools（cgo 需要 clang/ld）。

## 第三步：执行构建

### Windows（在仓库根目录，PowerShell）

发布构建（自动执行 `wails3 generate syso` 生成 exe 图标/清单/版本资源，再 go build 发布版）：

```powershell
.\build.ps1
```

产物：`bin\dsh-shell.exe`（发布版）、`bin\dsh-shell-debug.exe`（调试版）。
只调试或看控制台日志时：

```powershell
go build -ldflags "-H windowsgui" -o bin\dsh-shell-debug.exe .
go build -o bin\dsh-shell-console.exe .   # 实时日志用
```

运行示例：`bin\dsh-shell.exe --url http://127.0.0.1:3080`（参数：--url/--settings/--reset/--managed）。
数据目录：exe 旁 `bin\data\`；覆盖用 `DSH_SHELL_DATA_ROOT`。

### macOS（在 Mac 上，仓库根目录，bash）

```bash
wails3 build -clean -platform darwin -skipbindings
```

产物：`build/bin/DSH Desktop.app`；数据目录 `~/Library/Application Support/app.dsh.desktop`。
注意：
- 若报 `task: No Taskfile found`（本仓库未提交 Taskfile.yml），二选一：
  1. 先 `wails3 generate build-assets -dir .` 生成 Taskfile.yml 与各平台构建资产（生成物可能与手写 build/darwin/Info.plist 不同，需手工补回 ATS 局域网放行），再重试；
  2. 或直接 go build + 手动打包 .app（见 BUILD.md 4.3）。
- 未签名 app 被 Gatekeeper 拦截：右键打开，或 `xattr -dr com.apple.quarantine "build/bin/DSH Desktop.app"`。

## 第四步：验证

```bash
go test ./...     # 单测
go vet ./...      # 静态检查
```

- Windows：确认 `bin\dsh-shell.exe` 存在、双击可启动、图标正常（版本取自 `build/config.yml` 的 info.version）。
- macOS：确认 `build/bin/DSH Desktop.app` 存在且 `open` 可启动。

## 第五步：汇报

向用户报告：目标平台、使用的构建命令、产物路径、自测结果（go test/vet 是否通过）、以及任何已安装的工具（含来源）。
