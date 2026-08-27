# 决策记录（ADR）

记录项目关键决策及其背景，便于追溯"为什么这么做"。

## ADR-001：Fork onWatch 作为聚合服务底座

- **日期**：2026-08-27
- **状态**：已采纳
- **背景**：需要跨平台（Windows 优先）的 AI 用量聚合服务，且要支持本机登录态复用。
- **决策**：Fork [onWatch](https://github.com/onllm-dev/onWatch)（Go 守护进程 + SQLite + Web 面板），在其上扩展通用适配器。
- **理由**：onWatch 是唯一同时满足"Windows 官方支持 + Go 单二进制 + 本机登录态自动探测 + 内置 8+ 平台"的轮子；openusage 偏终端优先且官方不做 Windows 优先支持。
- **代价**：GPL-3.0 协议约束，本项目需保持 GPL-3.0 开源。

## ADR-002：Go 模块代理使用 goproxy.cn

- **日期**：2026-08-27
- **状态**：已采纳
- **背景**：国内网络无法访问 proxy.golang.org，`go build` 下载依赖超时。
- **决策**：`go env -w GOPROXY=https://goproxy.cn,direct`
- **理由**：国内稳定镜像，`direct` 兜底未收录模块。

## ADR-003：删除 server/.git 并入父仓库（而非 submodule/subtree）

- **日期**：2026-08-27
- **状态**：已采纳
- **背景**：`git clone` 使 server/ 成为嵌套仓库，父仓库只显示为 gitlink，无法正常纳入版本管理。
- **决策**：删除 `server/.git`，让父仓库直接跟踪 server/ 下所有文件。
- **理由**：本项目以自研扩展为主，onWatch 是底座而非持续同步的上游依赖；submodule 会增加使用复杂度。
- **代价**：后续合并 onWatch 上游更新需手动 diff 或改用 git subtree。

## ADR-004：根目录 .gitignore 策略

- **日期**：2026-08-27
- **状态**：已采纳
- **背景**：monorepo 含 server（Go）/widget（Tauri）/firmware（ESP32）三个子项目。
- **决策**：根目录 .gitignore 只覆盖 monorepo 层面（编译产物、凭证、各子项目构建目录），server/ 内部由 onWatch 自带 .gitignore 负责。
- **理由**：避免重复规则，各子项目构建产物隔离管理。

## ADR-005：P0 平台实现方式调整

- **日期**：2026-08-27
- **状态**：已采纳
- **背景**：原计划 P0 平台（DeepSeek/火山/智谱/OpenCode）全部新写内置适配器。
- **决策**：Phase 1 发现 onWatch 已内置 DeepSeek / OpenCode Go / Z.ai(cn=智谱)，仅火山引擎 Ark 需新写。
- **理由**：避免重复造轮子，P0 工作量从 4 个适配器缩减为 1 个。
- **影响**：Phase 2 交付物调整为"通用适配器引擎 + 火山引擎 Ark 适配器 + 其余 P0 平台验证"。