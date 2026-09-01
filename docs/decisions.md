# 决策记录（ADR）

记录项目关键决策及其背景，便于追溯"为什么这么做"。

> 版本：v5 | 更新日期：2026-09-01
> 变更摘要：新增 ADR-016（展示端配置字段语义对齐）；v4 新增 ADR-015（智谱国内版 CREDIT_LIMIT 解析修复）；v3 新增 ADR-006 ~ ADR-014（火山引擎 Coding Plan、Cookie 自动刷新、Ark 前端渲染、Insights 样式统一）

## 目录

- [ADR-001](#adr-001fork-onwatch-作为聚合服务底座) ~ [ADR-005](#adr-005p0-平台实现方式调整)：底座与 Phase 1 决策（2026-08-27）
- [ADR-006](#adr-006火山引擎-coding-plan-采用控制台-cookie-鉴权) ~ [ADR-010](#adr-010cookie-自动刷新采用-cdp--浏览器-db-组合提取)：Ark 适配器核心决策（2026-08-28）
- [ADR-011](#adr-011ark-前端渲染补全) ~ [ADR-014](#adr-014insights-卡片样式统一)：面板集成与样式决策（2026-08-31）
- [ADR-015](#adr-015智谱国内版-credit_limit-解析修复)：智谱 Coding Plan 适配修复（2026-09-01）
- [ADR-016](#adr-016展示端配置字段语义与-onwatch-前端消费字段对齐)：Phase 3 展示端配置设计约定（2026-09-01）

---

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

---

## ADR-006：火山引擎 Coding Plan 采用控制台 Cookie 鉴权

- **日期**：2026-08-28
- **状态**：已采纳
- **背景**：用户实际订阅为 Coding Plan（个人版），其查询接口 `GetCodingPlanUsage` 为控制台内部接口，仅支持浏览器 Cookie 鉴权（无 OpenAPI）。
- **决策**：新增 `ArkCodingPlanClient`（Cookie + x-csrf-token 鉴权），与 Agent Plan（AK/SK）并存，agent 双套餐轮询；Coding Plan 窗口以 `cp_` 前缀命名（cp_session/cp_weekly/cp_monthly）。
- **理由**：Coding Plan 是用户实际使用的套餐；百分比制（Percent+Cap）与 Agent Plan 的 used/limit 制通过统一 ArkSnapshot 归一化。
- **依据**：真实接口验证（2026-08-28）+ [ArkCodingPlanUsage](https://github.com/xiaokaiyyy/ArkCodingPlanUsage) 参考实现。
- **详情**：见 [ark-interface-research.md](ark-interface-research.md) §7。

## ADR-007：withArkBaseURL 空值保护

- **日期**：2026-08-28
- **状态**：已采纳
- **背景**：冒烟测试发现 `WithArkBaseURL("")` 把默认端点清空，导致请求发到 `?Action=GetAFPUsage` 无主机地址。
- **决策**：`WithArkBaseURL` 仅在传入非空值时覆盖默认值。
- **理由**：config 中 `ARK_BASE_URL` 为可选字段，空值传入是常态而非异常。

## ADR-008：Cookie 过期预警与自动刷新双机制

- **日期**：2026-08-31
- **状态**：已采纳
- **背景**：Coding Plan Cookie 中 `digest` JWT 仅 2 天有效，`userInfo` 30 天，手动刷新不可持续。
- **决策**：两层机制——①过期预警：解析 JWT exp，剩余 <3 天时日志告警；②自动刷新：Cookie 快过期（<1 天）或 401 时触发提取器刷新。
- **理由**：预警保证可见性，自动刷新减少手动操作；实测无法绕过"30 天内至少访问一次控制台"的前提（无官方 refresh token 接口）。

## ADR-009：Cookie 提取器接口化（CDP + 浏览器 DB 组合）

- **日期**：2026-08-31
- **状态**：已采纳
- **背景**：Windows 上浏览器运行时 Cookie 文件被独占锁定（实测 robocopy /B 也无法绕过），浏览器 DB 提取只在浏览器关闭时可用；用户选择 CDP 方案实现真正自动。
- **决策**：定义 `CookieExtractor` 接口，实现 `CDPCookieExtractor`（浏览器运行时，需 `--remote-debugging-port`）与 `BrowserCookieExtractor`（DPAPI+SQLite，浏览器关闭时），由 `CompositeCookieExtractor` 按 CDP→DB 顺序组合。
- **理由**：组合覆盖两种浏览器状态；接口化便于后续扩展（如 macOS keychain）。
- **代价**：CDP 需要用户用调试脚本启动浏览器（独立配置目录避免启动加速接管）。

## ADR-010：Cookie 自动刷新采用 CDP + 浏览器 DB 组合提取

- **日期**：2026-08-31
- **状态**：已采纳（ADR-009 的实现落地）
- **背景**：同 ADR-009。
- **决策**：main.go 中 `SetCookieExtractor(NewCompositeCookieExtractor(CDP, BrowserDB))`；提取触发条件为 `lastAuthFailed || Cookie 解析失败 || 剩余 <1 天`。
- **端到端验证**（2026-08-31）：无效 Cookie 启动 → CDP 提取 35 个 Cookie → 自动刷新 → 采集成功（weekly 8.61%、monthly 4.30%）。
- **关键坑**：digest/userInfo/csrfToken 存储在**父域 `.volcengine.com`**，CDP 过滤条件必须匹配 `volcengine.com`（最初只匹配 `console.volcengine.com` 导致提取缺关键 Cookie）；x-web-id 实测非必需。

## ADR-011：Ark 前端渲染补全

- **日期**：2026-08-31
- **状态**：已采纳
- **背景**：Ark 适配器后端完成但面板无数据——前端缺失三件套：`quota-grid-ark` 容器、`renderArkQuotaCards/updateArkCard` 渲染函数、`fetchCurrent` 的 ark 分支，导致 Ark 标签页空白、last-updated 不更新。
- **决策**：镜像 OpenCode 的渲染模式补全三件套 + 新增 `ark.svg` 图标；`isProviderConfigured("ark")` 同时检查 AK/SK 与 ConsoleCookie（任一即配置）。
- **教训**：新增内置 provider 时，后端五层（api/store/tracker/agent/web handlers）之外，必须同步检查前端四件（容器/渲染/更新/fetch 分支）与图标。
- **影响文件**：dashboard.html / app.js / handlers.go / icons/ark.svg

## ADR-012：Ark Provider 设置面板接入 UI

- **日期**：2026-08-31
- **状态**：已采纳
- **背景**：Provider Controls 中 Volcano Ark 只有 Telemetry/Dashboard 开关，无凭证配置入口（其他 provider 如 Copilot/Z.ai 有齿轮按钮）。
- **决策**：前端 `providerSettingsConfig` 注册 ark（Coding Plan Cookie/WebID、Agent Plan AK/SK、Region 五字段）；后端 `ApplyProviderSettingsFromDB` 扩展读取 console_cookie/console_web_id/console_csrf_token。
- **理由**：与现有 provider 设置机制一致，用户可在面板配置凭证，无需手改 .env。

## ADR-013：Cookie 占位值触发启用模式

- **日期**：2026-08-31
- **状态**：已采纳
- **背景**：用户不想手动复制长 Cookie；CDP 激活时可以全自动。
- **决策**：`.env` 配置 `ARK_CONSOLE_COOKIE=pending-cdp-refresh` 占位值触发 provider 启用，运行时 CDP 自动提取真实 Cookie 替换。
- **理由**：把"配置凭证"简化为"启动调试浏览器"，配合 ADR-010 实现全自动。
- **前提**：CDP 调试 Edge 需运行（`scripts/start-edge-debug.bat`）。

## ADR-014：Insights 卡片样式统一

- **日期**：2026-08-31
- **状态**：已采纳
- **背景**：Usage Insights 的 insight-card 与配额卡片 quota-card 视觉不一致（小圆角/无边框阴影/紧凑内边距）。
- **决策**：`.insight-card` 基础样式对齐 `.quota-card`（radius-lg / 22px padding / shadow-card / border-default；hover 同升起效果）；severity 着色保留（mix 到 surface-card）。
- **理由**：统一视觉语言，降低维护成本。

## ADR-015：智谱国内版 CREDIT_LIMIT 解析修复

- **日期**：2026-09-01
- **状态**：已采纳
- **背景**：用户配置 `ZAI_REGION=cn`（国内版 open.bigmodel.cn）后，面板 Z.ai 卡片用量全 0 且日志无错误。真实接口验证发现国内版返回 `CREDIT_LIMIT` 类型 limit（`{unit, number, usage, currentValue, remaining, percentage, nextResetTime}`），而 `ToSnapshot` 仅处理国际版（z.ai）的 `TIME_LIMIT`/`TOKENS_LIMIT`，两个 limit 均被跳过。
- **决策**：`ToSnapshot` 新增 `CREDIT_LIMIT` 分支——`number=5`（5 小时窗口，动态刷新无固定重置）映射到 Time 字段；`number=1`（周窗口，带 `nextResetTime`）映射到 Tokens 字段以保留重置倒计时。新增真实响应样本单测 `TestZaiQuotaResponse_ToSnapshot_DomesticCreditLimit`。
- **理由**：国内版与国际版响应结构同 wrapper（`code/msg/data/success`）但 limit 类型不同；复用现有快照字段避免 DB 迁移，周窗口映射到 Tokens 字段可复用 `TokensNextResetTime` 倒计时能力。
- **验证数据**：Lite 套餐，5 小时窗口 0/2000（0%），周窗口 2005/2000（100%，已超额）。
- **影响文件**：`internal/api/zai_types.go`、`internal/api/zai_types_test.go`
- **待办**：重启 onwatch 后确认面板显示。

---

## ADR-016：展示端配置字段语义与 onWatch 前端消费字段对齐

- **日期**：2026-09-01
- **状态**：已采纳（Phase 3 开工前设计约定）
- **背景**：Phase 3 需新增「展示端配置」（FR-6.1，每个展示端声明显示哪些平台/指标/刷新频率）。存在两种做法：发明一套新的配置字段体系，或与 onWatch 前端已消费的字段语义对齐。
- **决策**：
  1. **字段语义对齐**——展示端配置的可选项 = 前端已在消费的字段：平台集合（provider key 枚举，与 `/api/current` 输出 key 及 generic 平台一致）、指标类型（quota/balance/cost/tokens，即统一模型四类）、配额窗口勾选（对齐 menubar `SelectedQuotas` 的 per-window 选择语义）、`display_mode`（usage/available，复用既有语义）、刷新频率。
  2. **UI 可新做**——仅约定字段语义，不强制复用现有设置组件（齿轮面板等），交互可按展示端管理场景重新设计。
  3. **首个消费方为 Web 面板自举**——展示端配置完成后，第一个接入验证方是 Web 面板自身（默认展示端 `web-dashboard`），随后才是 Phase 4 Tauri 浮窗、Phase 5 墨水屏。
- **理由**：字段语义对齐保证展示端配置与统一指标模型、前端渲染逻辑天然兼容，避免出现"配置了但前端没有对应渲染"的悬空字段；Web 自举以最小改动闭环验证端到端链路，不阻塞浮窗/墨水屏接入。
- **影响**：[phase3-plan.md](phase3-plan.md) P3-10/P3-11 范围已按此约定细化；实施时不再另行讨论配置字段体系。

---

## 附：技术债与已知问题

| # | 问题 | 状态 | 说明 |
|---|---|---|---|
| 1 | `extra_coverage_test.go` Windows 编译失败（引用仅 `!windows` 定义的 `getCredentialsFilePath`） | 既有，未修 | 测试时临时移开该文件；修复方向：补 Windows 实现或改用 `USERPROFILE` |
| 2 | web/agent 包部分测试 Windows 失败（`os.UserHomeDir()` 不受 HOME env 影响） | 既有，未修 | Q8；与本项目改动无关 |
| 3 | config 包测试沙箱环境失败（TempDir 指向 C:\Windows 被拒） | 环境限制 | 真实环境正常 |
| 4 | CDP 依赖调试浏览器运行 | 设计约束 | 用户需保持 start-edge-debug.bat 启动的 Edge 运行；后续可探索后台无头模式 |