# DSH Desktop Shell

极简 Wails v3 + Go 桌面壳：webview 直连 DeepSeek Harness Web GUI（方案 A）。

## 原则

1. **只做壳**：默认不托管 Node/dsh 进程（外部连接模式）；可选 `--managed`
   时由壳托管本地 dsh（直接 dsh 命令或 `npx @deepseek-ai/dsh`）。不实现
   /api 反代，不改 DSH 前端。连接目标 = 用户已运行的 dsh 或远端已反代地址。
2. **设置页零构建**：assets/ 是手写 HTML/CSS/JS，经 `//go:embed all:assets`
   内嵌，不引入 vite/npm。改动后直接 `go build` 即可。
3. **控制路由只在本地设置页可达**：/__api/* 挂在 Wails AssetServer
   Middleware 上；窗口导航到 DSH 地址后请求不再经过它。
4. **版本单一来源**：build/config.yml 的 info.version（main.go 启动时读取）。
5. **数据安全**：settings.json 用 tmp + rename 原子写；数据目录可整体拷贝
   （Windows 便携 data/ 在 exe 旁）。

## 命令

- 构建：`go build -o bin\dsh-shell-debug.exe .`（或 `.\build.ps1` 出发布版）
- 测试/检查：`go test ./...` · `go vet ./...`
- 运行：`bin\dsh-shell-debug.exe --url http://127.0.0.1:3080`
  （--settings 强制设置页；--reset 清除地址；--managed 托管本地 dsh）
- macOS 构建（须在 Mac 上，wails3 走 cgo）：`wails3 build -clean -platform darwin -skipbindings`

## 结构

- main.go：窗口（application.New + NewWithOptions）、启动 URL 决策（--url /
  --settings / --reset / 保存值+探测回退）、尺寸记忆（WindowDidResize）、
  托盘创建 + 关闭守卫（关窗隐藏到托盘，托盘 Quit 才退出）
- internal/tray：托盘图标/菜单（显示隐藏、重新加载、连接设置、当前地址只读、
  打开数据目录、退出）；UpdateURL 供 connect 时刷新地址；macOS 用模板图标
  （tray-template.png）且左键弹原生菜单（Windows/Linux 左键切换窗口）
- internal/host：托管 dsh 子进程（Resolve 支持 DSH_BIN / npm 全局 / PATH dsh /
  npx @deepseek-ai/dsh；启动/重启/退出清理）
- internal/settings：settings.json（url / recent / window），SetURL 最新前置
  去重、recent 上限 5
- internal/api：Middleware(store, version, navigate)；路由 state / connect /
  probe / version；ProbeURL 供 main 启动探测复用；URL 校验用
  application.ValidateAndSanitizeURL
- assets/：设置页（输入地址、测试连接、连接、最近列表）

## 图标

- 图标 = DSH 官方 favicon（`dsh-web-frontend/dist/favicon.svg`），由
  `scripts/gen-icon.cjs`（sharp + 手写 ICO 打包）生成：appicon.png / icon.ico / logo.png。
- exe 资源通过 `wails3 generate syso` 产出 `wails_windows_amd64.syso`（build.ps1 自动）。
- 应用内图标：main.go 的 `Options.Icon`（embed assets/logo.png）；设置页 header logo。

## 基线

- Go 1.26，wails/v3 v3.0.0-beta.6（与 Coplin 一致），gopkg.in/yaml.v3
- Windows 已验证；macOS 已按 Coplin 模式写好：resolveDataRoot、托盘模板图标 +
  左键菜单、close-to-tray 守卫、build/darwin/Info.plist（ATS 局域网放行）。
  **未实测**：需在 Mac 上 `wails3 build` 验证（本仓库在 Windows 上开发，无 Mac 工具链）

## 已知边界

- WebView2 加载远程失败时显示浏览器错误页，无自定义提示（后续可在
  NavigationFailed 类事件上加强）
- 无通知/深链（后续 M 阶段）