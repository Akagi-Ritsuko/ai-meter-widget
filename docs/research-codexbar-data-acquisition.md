<!--
 * @Author: guotao
 * @Date: 2026-08-27
 * @LastEditors: guotao
 * @Description: CodexBar 数据采集实现技术调研文档 —— 供 ai-meter-widget 通用适配器 / P0 适配器 / oauth_local 认证设计参考
 * Copyright (c) 2026 by lzlj, All Rights Reserved.
-->

# CodexBar 数据采集实现技术调研文档

> 调研对象：CodexBar（https://github.com/steipete/CodexBar，MIT，macOS 14+ 菜单栏 AI 用量统计应用，内置 69+ provider，另有 Linux/macOS CLI 发行版）
> 调研时间：2026-08-27
> 参考资料：CodexBar 源码提取（30+ Swift 源文件）与官方 docs/ 设计文档
> 关联项目文档：[architecture.md](architecture.md)、[requirements.md](requirements.md)、[phase2-plan.md](phase2-plan.md)、[requirements-fr-1.1-p0-adapters.md](requirements-fr-1.1-p0-adapters.md)

## 1. 调研背景与目标

ai-meter-widget（fork 自 onWatch 的 Go 跨平台 AI 用量聚合守护进程）在 Phase 2 需要实现**通用适配器引擎**与 **P0 平台内置适配器**，其中通用适配器需支持四种认证方式：`api_key` / `bearer` / `cookie` / `oauth_local`（读取本机登录态文件，如 `~/.codex/auth.json`）。

CodexBar 是目前对"读取本机 Codex/OpenAI 登录态 → 调用官方用量 API"最完整的开源参考实现，同时覆盖了浏览器 Cookie 采集、CLI 子进程采集、本地会话日志费用扫描等与本项目 Phase 2/3 直接相关的技术栈。

本调研文档的目标：

1. 系统梳理 CodexBar 在**不同平台**（macOS / Linux / Windows 可类比）上获取、处理、存储数据的完整方法论；
2. 明确各平台的**认证机制、API 端点、数据源路径、平台限制与优化策略**；
3. 为 ai-meter-widget 的通用适配器、P2-02 `oauth_local` 校准、`cookie` 认证跨平台方案提供**可执行的最佳实践**。

## 2. CodexBar 项目概览

### 2.1 项目定位

- 单二进制 macOS 菜单栏应用（Swift 6 + SwiftUI/AppKit/WebKit），以及 `codexbar` CLI（Swift on Linux/macOS，FreeBSD 亦可）。
- 核心价值：**复用已有登录态，不保存密码**——数据源涵盖 OAuth / 设备流 / API Key / 浏览器 Cookie / 本地 CLI / 本地配置与 JSONL 日志。
- 输出一种**统一指标模型**（`UsageSnapshot` / `RateWindow` / `CreditsSnapshot` / 成本快照），并提供 `codexbar serve`（HTTP `/dashboard/v1/snapshot`）与 CLI 两种展示通道。

### 2.2 Provider 采集策略模式（核心架构）

CodexBar 对每个 Provider 使用**策略模式**组织采集路径：

- `ProviderFetchStrategy`（协议）：`isAvailable(context)` → `fetch(context)` → `shouldFallback(on:context:)`
- `ProviderFetchPipeline`：按顺序尝试可用策略，失败时依据 `shouldFallback` 决定是否回退到下一策略。
- `ProviderFetchContext`：携带 sourceMode（`auto` / `pat` / `oauth` / `cli` / `web` 等）、超时、浏览器检测、web 选项等。

以 Codex Provider 为例，其采集策略链为：

```
auto 模式解析 →
  CodexOAuthFetchStrategy        （本地 auth.json → OAuth 用量 HTTP API）
  CodexPATFetchStrategy          （同文件 personal_access_token → PAT 用量 API）
  CodexWebDashboardStrategy      （macOS 专属：浏览器 Cookie → OpenAI Web 仪表盘）
  CodexCLIStrategy / StatusProbe （CLI app-server RPC / PTY /status 文本解析）
  CostUsageFetcher               （本地 rollout JSONL → 费用扫描，独立于配额）
```

**对本项目的启示**：adapter 不必"一张卡只有一个请求"，而是可以像 CodexBar 一样把"配额窗口"、"余额/credits"、"本地 token/费用"拆成多个可独立失败、独立回退的数据源，最后合并成一个统一快照。

## 3. 数据采集总体架构（分层模型）

CodexBar 的数据获取可抽象为六个层次：

```
┌─────────────────────────────────────────────────────────────┐
│ L1 本地状态文件解析层                                          │
│    auth.json / config.toml / sessions/*.jsonl / SQLite / CSV  │
├─────────────────────────────────────────────────────────────┤
│ L2 认证层                                                      │
│    OAuth Token 刷新 / PAT / API Key / Cookie / CLI 会话        │
├─────────────────────────────────────────────────────────────┤
│ L3 传输层                                                      │
│    ProviderHTTPClient（重试/退避/重定向守卫） / cookie 传输     │
│    / 子进程传输（stdin/stdout JSON-RPC、PTY） / WebView         │
├─────────────────────────────────────────────────────────────┤
│ L4 数据源层（ProviderFetchStrategy）                           │
│    OAuth 用量 API / PAT API / Web 仪表盘 / CLI RPC /status     │
│    / 余额 API / 本地会话日志 / 浏览器 Cookie 页包              │
├─────────────────────────────────────────────────────────────┤
│ L5 模型层                                                      │
│    UsageSnapshot / RateWindow / CreditDetails /               │
│    CostUsageDailyReport / 项目-会话-模型维度快照              │
├─────────────────────────────────────────────────────────────┤
│ L6 存储与展示层                                                │
│    SQLite（WAL，25k 条 / 256 MiB 上限） / serve HTTP 快照      │
│    / 菜单栏 / WidgetKit / CLI                                 │
└─────────────────────────────────────────────────────────────┘
```

**关键点**：L1~L4 各层均做了**平台相关抽象**（`#if os(macOS)`、可执行文件探测、`ps`/`lsof`/`/proc` 回退），这是跨平台采集可移植性的核心。

## 4. 凭证与认证机制

### 4.1 Codex 凭证文件格式（oauth_local 核心参考）

`~/.codex/auth.json`（或 `$CODEX_HOME/auth.json`）：

```json
{
  "OPENAI_API_KEY": null,
  "tokens": {
    "id_token": "eyJ...",
    "access_token": "eyJ...",
    "refresh_token": "...",
    "account_id": "account-..."
  },
  "last_refresh": "2025-12-28T12:34:56Z"
}
```

要点：

- `tokens.access_token` 即 OAuth 短时令牌（JWT）；`tokens.account_id` 是请求级账号作用域标识；`last_refresh` 是老化兜底时间戳。
- **兼容形态**：顶层 `OPENAI_API_KEY` 非空时，可直存 API Key 使用（API Key 场景）。
- PAT 形态：同文件中的 `personal_access_token` / `personalAccessToken` 字段（`CodexPATCredentials`）。

### 4.2 凭证来源回退链（身份边界原则）

```
native  ~/.codex/auth.json            （默认，最高优先级）
  └─ legacy  ~/.config/codex/auth.json （仅 allowExternalSources 显式开启时考虑）
       └─ OpenCode ~/.local/share/opencode/auth.json（同样需显式开启）
```

- `loadForUsage(env:allowExternalSources:)`：native `CODEX_HOME` 优先；external 源必须显式 opt-in，属隐私边界设计。
- 每种来源都携带 `source`（native/legacy/opencode）、freshness、access token、account scope、refresh metadata，状态区分：`missing / malformed / stale-native / stale-external`。

**对本项目的启示**：`oauth_local` 读取本机登录态时，应同样遵循"默认只读 native 源、外部源需显式开启"的边界原则，避免扫盘行为；`~/.local/share/opencode/auth.json` 是 onWatch 已内置的 OpenCode 数据源，格式可复用同一解析器。

### 4.3 Token 刷新机制（所有权边界）

- **刷新端点**：`https://auth.openai.com/oauth/token`（OAuth2 refresh_token grant），`client_id=app_EMoamEEZ73f0CkXaXp7hrann`，scope=`openid profile email`。
- **关键设计**：CodexBar **不主动刷新** native `auth.json`——刷新与 refresh_token 生命周期归 **Codex CLI 所有**（CLI 拥有刷新端点）。CodexBar 只读 token，发现"过期需刷新"时：
  - native 凭证过期 → `nativeRefreshRequired` → 委托给 Codex CLI 恢复（启动 CLI 触发其自刷）；
  - legacy/OpenCode 凭证过期 → `readOnlySource` → **fail closed**（无安全的写回 handle，绝不写文件）；
  - 刷新错误分类：`expired` / `revoked` / `reused`。
- 过期判定：优先解析 access_token JWT 的 `exp` 声明（5 分钟刷新窗口），缺失/非法时回退到 `last_refresh` 的 8 天老化规则。
- JWT 解析只取 payload（head 3 段非空即可），不验签——真正的认证交给服务端。

**对本项目的启示**：
1. **"谁拥有刷新权"必须显式建模**。ai-meter-widget 作为守护进程，若随意用 refresh_token 刷新并写回共享 auth.json，会与 CLI 客户端产生写竞争（CodexBar 文档明确用"shared-file race"描述此风险）。
2. `oauth_local` 认证的策略应是：**读到可用 access_token 直接用；过期则报 auth 状态（提示用户运行 CLI 或重新登录），而不是越权刷新写文件**。

### 4.4 认证方式 → 采集路径映射（对齐通用适配器四种认证）

| 通用适配器认证 | CodexBar 对应实现 | 真实请求形态 | 典型端点 |
|---|---|---|---|
| `api_key` | `APITokenFetchStrategy` | `Authorization: Bearer <key>` | `/user/balance`、`/v1/dashboard/billing/credit_grants` |
| `bearer` | OAuth/PAT strategy | `Authorization: Bearer <token>` | `/wham/usage`、`/api/codex/usage` |
| `cookie` | `OpenAIDashboardFetcher` / Cursor / MiniMax cookie 策略 | `Cookie: <header>` + CSRF Origin（POST 时） | `https://chatgpt.com/backend-api/...` |
| `oauth_local` | `CodexOAuthFetchStrategy` | **从本地 auth.json 取 token → 按 bearer 形态发请求** | `https://chatgpt.com/backend-api/wham/usage` |

**核心结论（回应 P2-02 风险）**：`oauth_local` 的真实请求头语义**不是**直接把文件内容当 Cookie 发，而是：

```http
GET https://chatgpt.com/backend-api/wham/usage
Authorization: Bearer <tokens.access_token>
ChatGPT-Account-Id: <tokens.account_id>
User-Agent: CodexBar
Accept: application/json
```

即 `oauth_local` = **"从本地登录态文件解析出 Bearer token + 账号作用域头，再按 bearer 语义调用请求头映射"**。ai-meter-widget 通用适配器应为 `oauth_local` 提供"KeyFrom=file:path:jsonpath 提取值 → 可配置请求头模板（如 `Authorization: Bearer {access_token}` + `ChatGPT-Account-Id: {account_id}`）"的机制。

### 4.5 PAT 认证（CodexPATFetchStrategy）

- 读取同一 `auth.json` 的 `personal_access_token`。
- **managed CODEX_HOME 隔离**：当 CODEX_HOME 指向 managed workspace（fail-closed 占位目录）时跳过 PAT 路径，避免用 stale account 查询错账号。
- PPT 请求身份来自 token 自身（`whoami`），而非 config 里的历史 account id。

### 4.6 凭证文件访问安全与测试隔离

- `CodexCredentialFileAccess`：测试环境变量 `CODEXBAR_TEST_CODEX_FILE_ISOLATION` + FixtureScope 白名单 + **符号链接安全校验**（防止凭证文件被 symlink 指向其他路径）。
- 凭证类日志全部脱敏；CLI 安全性上避免把 token 放 shell history（env 优先于 argv，argv 会被 `ps` 泄露）。
- Keychain 访问可通过设置关闭（`Disable Keychain access`），被关闭后浏览器 Cookie 解密跳过而非请求授权。

## 5. 采集实现明细（按数据源类型）

### 5.1 Codex OAuth 用量 HTTP API（最核心，oauth_local 校准依据）

**端点与 Base URL 解析**：

```
默认 base:  https://chatgpt.com/backend-api/
用量端点1:  {base}/wham/usage                     （chatgpt.com 后端）
用量端点2:  {base}/api/codex/usage                （base 不含 /backend-api 时）
辅助端点:   /wham/rate-limit-reset-credits        （带 OpenAI-Beta: codex-1 + originator 头）
辅助端点:   /accounts/<accountId>/spend-controls/current-user/monthly-usage
```

- Base URL 可被 `~/.codex/config.toml` 的 `chatgpt_base_url` 覆盖；`normalizeChatGPTBaseURL` 对 `https://chatgpt.com` / `https://chat.openai.com` 自动追加 `/backend-api`。
- 按 normalized base 是否含 `/backend-api` 决定走 `wham/usage` 还是 `codex/usage`。

**请求头**：

```http
Authorization: Bearer <access_token>
ChatGPT-Account-Id: <account_id>
User-Agent: CodexBar
Accept: application/json
```

（PAT 路径额外带 `originator: codex-cli` 风格头；rate-limit-reset-credits 带 `OpenAI-Beta: codex-1` + `originator: Codex Desktop`。）

**响应模型（CodexUsageResponse，容错解码）**：

```jsonc
{
  "account_id": "...",
  "plan_type": "pro",                // guest/free/go/plus/pro/free_workspace/team/business/education/quorum/k12/enterprise/edu/unknown
  "rate_limit": {
    "primary_window":   { "used_percent": 15, "reset_at": 1735401600, "limit_window_seconds": 18000 },
    "secondary_window": { "used_percent": 5,  "reset_at": 1735920000, "limit_window_seconds": 604800 },
    "individual_limit": {...}
  },
  "credits": { "has_credits": true, "unlimited": false, "balance": 150.0 },   // balance 兼容 String
  "individual_limit": {...},
  "spend_control": { "individual_limit": {...} },
  "additional_rate_limits": [ { "limit_name": "...", "metered_feature": "...", "rate_limit": {...} } ]
}
```

- 解析策略：字段逐个 `try?` + **snake_case/camelCase 双 key 回退**；数值用 `decodeFlexibleDouble/Int`（Double/Int/String 三态）；`AdditionalRateLimit` 数组用 **Lossy 解码**（单条 malformed 不丢弃兄弟元素）。
- `resolvedIndividualLimit` 优先级：response 根 `individualLimit` → `rateLimit.individualLimit` → `spendControl.individualLimit`。
- 错误分类：401/403 → `unauthorized`（提示重新认证）；其他 → `serverError(code, body)`，并尝试从 `body=` 之后的 JSON 兜底恢复数据。

**对本项目的启示**：这直接决定了 P2-02 后 `oauth_local` 的**端到端链路**——ai-meter-widget 后续若想支持 Codex 用量卡，只需：读 `~/.codex/auth.json` → 取 access_token + account_id → GET `https://chatgpt.com/backend-api/wham/usage` → 按 `used_percent / reset_at / limit_window_seconds` 映射到统一指标模型（与 onWatch 的 quota window 模型对齐）。

### 5.2 OpenAI Web 仪表盘采集（Cookie 认证参考，macOS 专属）

`OpenAIDashboardFetcher`（`Sources/CodexBarCore/OpenAIWeb/`）：

- 目标页：`https://chatgpt.com/codex/cloud/settings/analytics#usage`。
- 链路：**先 API preflight，再 WebView scrape**：
  1. 从 `WKWebsiteDataStore` 提取 chatgpt.com 域 cookie，拼接 `Cookie: name=value; name=value`；
  2. preflight 调 `https://chatgpt.com/backend-api/wham/usage`、`/subscriptions`、`/me`、`/api/auth/session`；
  3. 需要用量明细时，再加载 WebView（offscreen 1×1px + alpha 0.001 保持 WebKit hydration，防节流），注入 `probeReadiness` / `scrape` JS；
  4. `shouldWaitForProbeReadiness` 状态机（signedInEmail / usageRouteSeenAt / dashboardSignalSeenAt）。
- `findFirstEmail`：BFS 遍历 JSON（≤2000 节点）找 `email` key，用于确认"页面属于正确的账号"。
- **非 macOS 平台直接抛 `noDashboardData("...only supported on macOS.")`**：`#else` 分支 `isAvailable` 恒 false。
- **账号归属校验**（CodexDashboardAuthority）：wrongEmail / 无 scoped email / ambiguous email → 拒绝（fail-closed），防止把别的浏览器账号数据显示到当前账号卡片上。

**对本项目的启示（cookie 认证跨平台）**：
1. WKWebView 系方案 **Windows/Linux 不可用**；`cookie` 认证在跨平台守护进程里应改为两条替代路径：① 浏览器 Cookie 库文件直读（Chrome/Edge 的 `Cookies` SQLite + 解密，Linux 用 `secret-tool`/`kwallet`，Windows 用 DPAPI）；② 用户手动粘贴 Cookie 头。
2. **CSRF 校验**：Cookie 认证且请求为 POST 时，必须带同源 `Origin` 头（Cursor 实测强制要求），否则 403。
3. 多账号场景必须做**账号归属校验**，避免 A 账号 cookie 显示 B 账号数据。

### 5.3 CLI 子进程采集（JSON-RPC 守护进程，CodexRPCClient）

- 启动子进程：`codex -s read-only -a never app-server`，通过 stdin/stdout 走 newline-delimited JSON-RPC：
  `initialize` → `account/read` → `account/rateLimits/read`。
- 超时：连接 8s / 单请求 3s，**超时即杀掉子进程**（不留僵尸进程）。
- 失败时 `recoverUsageFromRPCError` 尝试从错误体 JSON 恢复配额数据。
- **可信启动门**（`CodexCLILaunchGate`）：只信任白名单可执行文件，失败被节流；`TrustedCodexAppServerCache` 缓存可信二进制。

**对本项目的启示**：Go 侧可类比——不直接调用户二进制做 `--help`/`status` 解析，而是启动官方 CLI 的只读 server 子进程走结构化协议；必须用 context 超时 + `os/exec` 进程组杀掉，防止孤儿进程。

### 5.4 CLI PTY /status 文本解析（CodexStatusProbe）

- 在 PTY 中运行 `codex`，发送 `/status`，捕获文本，正则解析：
  - `Credits:\s*([0-9][0-9.,]*)`；
  - 5h limit / Weekly limit 行的百分比与 reset 文案（`parseResetDate` 支持多 datetime 格式 + 未来时间年份抬升）；
  - `Monthly credit limit` → `CodexCreditLimitSnapshot`。
- 解析失败（`parseFailed`）仅重试一次（不同 rows/cols 参数）；检测到"update available" → `updateRequired` 提示用户升级。
- 这是 CodexBar 的**最初实现**，已被 OAuth API 取代为回退路径——文档明确评价"slow and unreliable"。

**对本项目的启示**：CLI 文本解析应只作为最后兜底；优先结构化接口。若必须解析，采用"ANSI 剥离 → 正则 → 多格式日期解析"的顺序，并对解析失败做有限重试。

### 5.5 PAT 用量 API（CodexPATUsageFetcher）

- `whoami`：`https://auth.openai.com/api/accounts/v1/user-auth-credential/whoami` → `chatgpt_account_id` / `chatgpt_plan_type` / `email`。
- 用量：复用 `CodexOAuthUsageFetcher.chatGPTUsageURL(env:)` + `Authorization: Bearer <pat>` + `ChatGPT-Account-Id: <whoami.account_id>`。
- **PAT 身份来自 token 自身 whoami，而非历史配置**（防 managed workspace fail-close 后查错账号）。

### 5.6 API Key 余额采集（P0 平台参考）

**OpenAI API Key**（`OpenAIAPICreditBalanceFetcher`）：

- `GET https://api.openai.com/v1/dashboard/billing/credit_grants`，`Authorization: Bearer <apiKey>`。
- 响应：`total_granted` / `total_used` / `total_available` / `grants.data[]`（grant_amount/used_amount/expires_at）。
- 错误分类：401 → 需 Organization Admin API key；403 → 需 legacy/user API key。
- 映射：`usedPercent = used/granted`；`nextGrantExpiry → resetsAt`；ProviderCostSnapshot(period="API credits")。

**DeepSeek**（`DeepSeekUsageFetcher`，与 onWatch 现有适配器同理念）：

- 余额：`GET https://api.deepseek.com/user/balance`（Bearer apiKey）→ `balance_infos[]`（currency / total_balance / granted_balance / topped_up_balance）。
- 平台 token 详细用量：`GET https://platform.deepseek.com/api/v0/usage/amount?month=&year=` 与 `/usage/cost`（Bearer platformToken），`TaskGroup` 并行。
- 错误信封 `code` / `biz_code`（40002/40003 = 认证错误）；余额 String/Double 容错解码。
- 映射：余额 ≤0 → usedPercent 100%。

**对本项目的启示**：这两例是"API Key 认证 + 余额/配额 → 统一 RateWindow/成本快照"的标准范式，与 onWatch 现有 `deepseek_client.go`/`zai_client.go` 结构一致；P0 适配器（火山 Ark 新写）可直接沿用。

### 5.7 浏览器 Cookie 导入（BrowserCookieImportOrder）

- `cookieImportCandidates(using:)` 过滤：Keychain 禁用 + `detection.isCookieSourceAvailable` + `BrowserCookieAccessGate.shouldAttempt`。
- `lazyCookieImportCandidates`：lazy 序列，"第一个成功即停"。
- `usesKeychainForCookieDecryption`：**Chrome/Edge/Brave/Arc 系必须 Keychain**（Safe Storage 解密）；Safari/Firefox 系不需要。
- 平台差异：真实实现仅 macOS（`SweetCookieKit`）；Linux/Windows 由其他途径（直接读 cookie 文件）。

### 5.8 本地会话日志费用扫描（token/费用统计，Phase 3 参考）

**Codex rollout**（`CostUsageScanner` + `CodexRolloutFirstLineParser`）：

- 路径：`$CODEX_HOME/sessions/yyyy/MM/dd/rollout-*.jsonl`，近 2 天；读首行元数据提取 sessionID / cwd / title / model。
- 增量解析：mtime+size 变化检测、`parsedBytes` 续扫、`pricingKey` 缓存失效。

**Pi/omp 家族**（`PiSessionCostScanner`）：

- 根目录 `~/.pi/agent/sessions`、`~/.omp/agent/sessions`；文件名正则 `^(\d{4}-\d{2}-\d{2})T(\d{2})-(\d{2})-(\d{2})-(\d{3})Z_`。
- JSONL 行：`session`（id）、`model_change`（切换当前模型）、`message`（仅 assistant 取 usage）。
- usage 字段容错：`input`/`inputTokens`/`input_tokens`/`promptTokens`/`prompt_tokens`；`cacheRead*`/`cache_write*`；`output*`/`completion_tokens`；total 取"显式 total 与派生 total 的 max"。
- 去重：`seenEntriesBySessionID`（防多文件重复计入同一 session）。

**Claude**（`ClaudeSessionProjectMapper.transcripts`）：`~/.claude/projects/<cwd>/*.jsonl`。

**Cursor**（token 费用，双轨）：

- 在线：`POST /api/dashboard/get-filtered-usage-events`（Cookie + Origin 头），分页 pageSize=1000 / maxPages=200（上限 20 万事件），`totalUsageEventsCount` 权威核对 + **页边界重复行去重**（无稳定事件 ID）。
- 本地：`CursorLocalCSVReader` 读 tokscale 兼容的 `~/.config/tokscale/cursor-cache/usage*.csv`（列布局自适应 hasKind 两种格式；`totalTokens` 权威列优先）。
- 成本口径：`tokenUsage.totalCents` 为 API 列表价；`chargedCents` 为 Cursor 套餐真实扣费（任一事件缺失则整窗口合计返回 nil，防止公布不完整小计）。

**定价**：Models.dev 定价管道（`ModelsDevPricingCache` + 自定义定价指纹），费用单位 `costNanos`（1e9 标度），无定价 token 用 `unknownCostTokens` 标记；`costProvenance` 记录数据来源。

**扫描预算**（`DirectoryMetadataScanBudget`）：maxEntryCount / maxDepth / timeLimit 三纬上限，防止扫盘失控。

**对本项目的启示**：Phase 3 "token/费用统计" 应采纳：① 结构化日志（JSONL）优先于非结构化；② 增量扫描（mtime+size+parsedBytes）；③ 目录扫描预算；④ 会话去重；⑤ "权威计费字段缺失时拒绝发布不完整合计"的诚实降级。

### 5.9 本地项目/会话维度用量索引（CodexLocalProjectUsageIndexer）

- `loadSnapshot` 编排：`CostUsageScanner.loadDailyReportCancellable` → `CodexThreadCatalogReader`（会话标题/模型目录）→ `sidecar` 同步 → `buildSnapshotFromCostCache`。
- 输出多维视图：projects / sessions / modelBreakdowns / daily / modelsAnalytics（current vs previous 周期对比）。
- 项目归属：从 cwd 解析 project identity，`chatsProjectId` 兜底；缓存指纹 `stableScopeSignature`（scope identifier + timeZone）保证缓存按范围失效。

## 6. 平台差异分析（macOS / Linux / Windows 视角）

### 6.1 进程枚举

| 能力 | macOS（Darwin 原生） | Linux/其他（回退） |
|---|---|---|
| 全部 PID | `proc_listallpids`（buffer +32 余量动态扩） | `ps -axo pid=,ppid=,lstart=,command=` |
| 可执行路径 | `proc_pidpath` | `ps` command 列 |
| 父进程/启动时间 | `proc_pidinfo(PROC_PIDTBSDINFO)` | `ppid`/`lstart` 列 |
| 完整命令行（含参数） | `sysctl([CTL_KERN, KERN_PROCARGS2, pid])` + `parseProcArgs2` | **不读** `/proc/<pid>/environ`（隐私：会捕获无关秘密）；仅用 argv 白名单匹配后再请求 |
| CWD | `proc_pidinfo(PROC_PIDVNODEPATHINFO)` → `pvi_cdir.vip_path` | `lsof -a -d cwd -Fn -p <pids>` 或 `/proc/<pid>/cwd` 符号链接 |
| 监听端口 | `proc_pidinfo(PROC_PIDLISTFDS)` + `PROC_PIDFDSOCKETINFO`（TCP LISTEN，`insi_lport` bigEndian） | 不适用（未实现） |

**隐私原则**：绝不执行 `ps eww`、绝不读 `/proc/<pid>/environ`（CodexBar 明确写入设计文档）。

### 6.2 浏览器 Cookie 与系统钥匙串差异

| 浏览器类别 | 是否需要 Keychain/系统凭据解密 | 说明 |
|---|---|---|
| Chrome / Edge / Brave / Arc | **需要**（Safe Storage） | macOS Keychain；Windows 对应 DPAPI；Linux 为 `gnome-keyring`/`kwallet` |
| Safari / Firefox | 不需要 | 明文/独立存储 |
| 非 macOS | Cookie 导入实现为空壳 | 需替代方案（文件直读或手动粘贴） |

### 6.3 WebKit / WKWebView 平台限制

- Web 仪表盘类采集（需要 JS 渲染 + 登录态）**仅 macOS**（WKWebView）。
- Linux CLI / Windows 版 CodexBar 社区移植（Win-CodexBar / codexbar-waybar 等）均基于 CLI/serve 输出，**不含 WebView 采集**。

### 6.4 路径与配置约定

- Codex：`~/.codex/`（或 `CODEX_HOME`），config.toml 里 `chatgpt_base_url` / `model_provider` / `requires_openai_auth`（CodexBar 用轻量 TOML 解析器判断后端是 OpenAI 还是 Amazon Bedrock，决定配额是否可用）。
- Cross-platform home：`FileManager.homeDirectoryForCurrentUser` 等价于 Go 的 `os.UserHomeDir`；XDG 约定（`~/.config`、`~/.local/share`）必须在 Linux 上优先于 macOS 惯例。

### 6.5 可执行文件探测

- `BinaryLocator.resolveCodexBinary(env:loginPATH:)`：先按 `which`/PATH 解析，再校验 `isExecutableFile`；身份来自 **login shell PATH 缓存**（`LoginShellPathCache`），因为 CLI 往往按 `~/.local/bin`、`bun`、Homebrew 等非默认路径安装。

**对本项目的启示（Windows 优先）**：
1. Windows 没有 `/proc`/`ps`/`lsof` 等价物；需要"检测某个 CLI 是否在运行/定位其可执行文件"时，应使用 Windows 专属（WMI `Win32_Process` / `tasklist` / `Get-Process` API 层或 Go `gopsutil`）并用 build tag 隔离，与 Linux 回退同时实现。
2. onWatch 已有 `platform_windows.go` 之类 OS 隔离模式，新适配器复用该模式即可。

## 7. 容错与健壮性设计

### 7.1 容错解码（JSON 字段）

- snake_case/camelCase 双 key 回退；
- 数值三态（Double/Int/String，`decodeFlexibleDouble/Int`）；
- 数组 Lossy 解码（单元素坏不丢兄弟）；
- 封面字段问题不致命——字段缺失（`.omitted`）与字段非法（`.invalid`）区分记录，允许"部分已知"累计。

### 7.2 HTTP 重试与重定向

- `ProviderHTTPRetryPolicy`：408/429/500/502/503/504 + URL 错误码；只对幂等方法重试；尊重 `Retry-After`；指数退避。
- `ProviderHTTPRedirectGuardDelegate`：**仅允许 HTTPS→HTTPS 同源重定向**（防 cookie/token 被重定向泄露到其他域）。

### 7.3 分页与去重

- Cursor 事件页：`maxPages` 硬上限防死循环；`totalUsageEventsCount` 权威总数核对；页边界精确重复行去除；**最后满页不能证明完成**（要求出现空/短页）。
- 本地会话：`seenEntriesBySessionID` 去重。

### 7.4 资源上限

- 响应体大小上限（README 示例：dashboard 页 2 MiB、API 响应 64 KiB）；
- 目录扫描预算（maxEntryCount/maxDepth/timeLimit）；
- 进程枚举预算（最新 64 个 agent 进程、128 条 rollout 元数据、64 条 Claude transcript 候选）；
- SQLite 保留上限 25k 条 / 256 MiB（WAL 模式）。

### 7.5 错误分类与状态机

- 网络错误 / 认证错误（401/403）/ 服务端错误（5xx）/ 解析错误 / 超时，各有明确的"是否可回退"策略；
- 状态枚举（以 DeepSeek 为例）：`notRequested / available / webSessionRequired / profileSelectionRequired / unavailable`——**哪个环节缺什么，明确告知用户**；
- `stale-while-revalidate`：旧数据保留展示（带 updatedAt），新数据后台刷新。

### 7.6 配额真实性

- `isSyntheticPlaceholder`：当服务端未提供真值而占位 0% 时，标记为合成数据，防止 UI 渲染"幻影 0% 窗口"。
- `unknownCostTokens` / `costProvenance`：费用数据标注已知/未知、数据来源，杜绝把估算当账单。

## 8. 统一指标模型

```swift
UsageSnapshot { primary/secondary/tertiary: RateWindow, updatedAt, identity }
RateWindow  { usedPercent, windowMinutes, resetsAt, resetDescription, isSyntheticPlaceholder }
CreditDetails { hasCredits, unlimited, balance }
CostUsageDailyReport { date, input/output/cacheRead/cacheWrite tokens, costUSD, modelBreakdowns }
CodexLocalProjectUsageSnapshot { total, projects, sessions, modelBreakdowns, daily }
```

> 与 onWatch 现有 snapshot（quota windows used/limit/percent/reset + balance）可直接对齐：`used_percent`→`usedPercent`，`reset_at`（epoch 秒）→`resetsAt`，`limit_window_seconds`→`windowMinutes`。

## 9. 对 ai-meter-widget 的映射与实施建议

### 9.1 P2-02：`oauth_local` 真实场景校准（直接结论）

- **请求头语义**：不是"把 auth.json 当 Cookie"，而是**提取 `tokens.access_token` 作 `Authorization: Bearer`，提取 `tokens.account_id` 作 `ChatGPT-Account-Id` 头**。
- **通用适配器修改建议**：`oauth_local` 配置增加"请求头模板"能力，例如：

```jsonc
{
  "auth": { "type": "oauth_local" },
  "key_from": { "type": "file", "path": "~/.codex/auth.json", "jsonpath": "$.tokens.access_token" },
  "headers_from": [
    { "name": "Authorization", "template": "Bearer {tokens.access_token}" },
    { "name": "ChatGPT-Account-Id", "value_from": { "jsonpath": "$.tokens.account_id" } }
  ]
}
```

- **刷新边界**：access_token 过期时，只上报 `auth_failed`（提示运行 `codex` 重新认证），**禁止守护进程用 refresh_token 写回共享 auth.json**。
- 若 ai-meter-widget 后续提供 Codex 内置卡，可直接照抄 §5.1 端点与人头构造。

### 9.2 `cookie` 认证跨平台方案

- V1（立即）：支持 `cookie` 认证时走"手动 cookie 头 + Origin 同源校验"，覆盖 chatgpt.com / cursor.com / opencode.ai 类站点。
- V2（Phase 3+）：Windows 用 DPAPI 解密 Chrome/Edge `Cookies` SQLite；Linux 用 `secret-tool`/`kwallet`。参考 §6.2 浏览器类别差异（Safari/Firefox 免解密）。
- 必须做账号归属校验（cookie 与期望账号 email 一致），否则 UI 显示错误数据。

### 9.3 `api_key` / `bearer` 认证参考

- P0 新写火山 Ark 时，直接参照 §5.6 的 DeepSeek/OpenAI 范式：Bearer 认证 + 官方 quota 端点 + 字段容错解码 + 401/403→`auth_failed`、429 尊重 `Retry-After`、5xx→`error` 保留 last-good。
- 火山 Ark 接口调研（P2-09 D5）建议先按本章节框架产出：端点、认证、响应 schema、限速策略、错误映射。

### 9.4 通用适配器设计原则（本项目可直接落地）

1. **策略链而非单请求**：`api_key` 与本地扫描可并存（如 Codex 配额 + 本地 rollout 费用），各自独立失败；
2. **容错解码引擎**：把 snake_case/camelCase 双 key、数值三态、Lossy 数组解析做成 JSONPath 映射引擎的内置能力（FR-4.2）；
3. **请求头模板**：`bearer`/`oauth_local` 都应有 header template + 变量注入；
4. **错误状态机**：`ok / error / auth_failed / unconfigured`（对齐 requirements-fr-1.1 §STD-03），未配置不发请求；
5. **凭证只读**：日志脱敏、不写 DB 明文、外部源 opt-in（对齐 ADR 与 NFR-3）；
6. **扫描预算**：任何目录枚举都带 entry/depth/time 上限。

### 9.5 平台适配建议（Windows 优先）

| 能力 | Windows 方案建议 |
|---|---|
| 进程/CLI 探测 | `gopsutil` / WMI，build tag 隔离；只读 argv 白名单 |
| CLI 子进程调用 | `exec.CommandContext` + 超时 + Kill 进程组（防僵尸） |
| Cookie 解密 | DPAPI（Chrome/Edge）；Firefox `cookies.sqlite` 无需解密 |
| 文件路径 | `filepath`/`os.UserConfigDir`，Linux 遵 XDG（现有代码已示范） |
| 服务 | onWatch 已跑通 Windows；仅新增平台层的差异化实现 |

### 9.6 最佳实践清单（速查）

- [ ] oauth_local = 本地文件解析 token → Bearer 请求头 + 可选账号作用域头；不越权刷新共享文件
- [ ] 谁拥有凭证写回权必须显式建模（CLI 拥有刷新权，守护进程只读）
- [ ] 多来源回退链 + 外部源显式 opt-in（隐私边界）
- [ ] 容错解码：双 key / 数值多态 / 单元素 Lossy
- [ ] HTTP：Retry-After 尊重、指数退避、仅 HTTPS 同源重定向、响应体上限
- [ ] POST+cookie 必须同源 Origin 头（CSRF）
- [ ] Cookie 采集做账号 email 归属校验
- [ ] 本地扫描全部带预算（entry/depth/time）
- [ ] 权威计费字段缺失 → 拒绝公布不完整合计（诚实降级）
- [ ] 占位数据标记 isSyntheticPlaceholder，防幻影 0%
- [ ] 错误分类驱动回退策略，而不是一律重试
- [ ] 凭证日志脱敏、DB 不落明文、env 优先于 argv

## 10. 参考资料索引

- CodexBar 源码分析文件（本调研原始素材，位于本地 temp 目录）：
  - `Sources__CodexBarCore__Providers__Codex__CodexOAuth__CodexOAuthCredentials.swift`（凭证格式/来源链）
  - `Sources__CodexBarCore__Providers__Codex__CodexOAuth__CodexTokenRefresher.swift`（刷新端点/错误分类）
  - `codex_oauth_usage_fetcher.swift`（OAuth 用量 API 完整实现）
  - `Sources__CodexBarCore__Providers__Codex__CodexPAT__`（PAT 采集）
  - `Sources__CodexBarCore__OpenAIWeb__OpenAIDashboardFetcher.swift`（Web/Cookie 采集）
  - `Sources__CodexBarCore__Providers__Cursor__`（Cursor 事件/CSV/Sand Usage）
  - `Sources__CodexBarCore__Providers__DeepSeek__DeepSeekUsageFetcher.swift`（余额+平台用量双轨）
  - `Sources__CodexBarCore__Providers__OpenAI__OpenAIAPICreditBalanceFetcher.swift`（credit_grants）
  - `Sources__CodexBarCore__CostUsageFetcher.swift` / `PiSessionCostScanner.swift` / `CodexLocalProjectUsageIndexer.swift`（本地费用扫描）
  - `Sources__CodexBarCore__DarwinProcessEnumerator.swift` / `LocalAgentSessionScanner.swift`（平台差异）
  - `Sources__CodexBarCore__ProviderHTTPClient.swift`（重试/重定向）
- CodexBar 官方文档（同名提取）：dashboard-api / codex-oauth / agent-sessions-design / predictive-refresh-policy / configuration / cli
- 本项目关联文档：[phase2-plan.md](phase2-plan.md)（P2-02）、[requirements-fr-1.1-p0-adapters.md](requirements-fr-1.1-p0-adapters.md)、[requirements.md](requirements.md)、[decisions.md](decisions.md)、[roadmap.md](roadmap.md)