# FR-1.1 平台内置适配器（DeepSeek / 火山 Ark / 智谱 / OpenCode）编码实现计划

> 依据：[docs/requirements-fr-1.1-p0-adapters.md](../../docs/requirements-fr-1.1-p0-adapters.md)（D1–D5 交付物）、[docs/phase2-plan.md](../../docs/phase2-plan.md)（P2-09~P2-16）
> 状态：待批准 | 目标目录：`server/`（Go 1.25.7 模块，编译产物 `server/onwatch.exe`）

## 1. 摘要

为 onWatch 实现 FR-1.1 P0 四平台内置适配器：

1. **新建火山方舟 Ark 适配器全栈**（`internal/api` → `internal/agent` → `internal/tracker` → `internal/store` → `internal/web`），接入 AgentManager 与 Web 面板。
2. **验证 DeepSeek / 智谱(Z.ai cn) / OpenCode 三个既有适配器**（mock 回归 + 文档化真实验证步骤，真实验证留待用户提供凭证）。
3. **凭证安全与文档交付物**（D1–D5）。

已确认关键事实（2026-08-27 官方文档调研）：

- 火山方舟个人版 Agent Plan 用量接口：`POST https://ark.cn-beijing.volcengineapi.com/?Action=GetAFPUsage&Version=2024-01-01`，请求体 `{}`，**仅支持 Access Key 鉴权**（`AccessKey`+`SecretKey` 成对，HMAC-SHA256 V4 签名，service=`ark`，region=`cn-beijing`）。
- 响应：`ResponseMetadata{RequestId,...}` + `Result{PlanType, AFPDaily, AFPFiveHour, AFPWeekly, AFPMonthly}`，每窗口 `{Quota, Used, SubscribeTime, ResetTime}`（Quota/Used 官方文档为 string，GetSeatAFPUsage 示例为 number —— 需自定义容错解析；时间戳为 epoch 毫秒）。
- 企业版用 `GetSeatAFPUsage`（需 SeatID 列表）——非 P0 范围，仅记录。

## 2. 现状分析（Phase 1 勘察结论）

- **统一适配器契约**：`agent.AgentManager.RegisterFactory(key, factory)` 注册，`main.go` 1750 行启动列表遍历 + `isPollingEnabled` 开关；agent 均实现 `AgentRunner.Run(ctx)`（立即 poll + `time.NewTicker(interval)`）。
- **既有模式三模板**：
  - 平衡型客户端：`internal/api/deepseek_client.go`（Bearer、30s、64KiB、`redactAPIKey`）。
  - 多窗口配额：`internal/api/opencode_client.go` + `opencode_types.go` + `opencode.go`（四种 Quota 展示顺序 five_hour/weekly/monthly）、`open`；store 双表 `opencode_snapshots`/`opencode_quota_values` + `opencode_reset_cycles`（schema 在 `store/store.go` 792/813 行）。
  - Agent 模板：`internal/agent/zai_agent.go`（client/store/tracker/interval/logger/sm/notifier/pollingCheck；`SetNotifier`/`SetPollingCheck`）。
- **Web 接线（关键发现）**：DeepSeek/OpenCode/Z.ai **均无专属路由**（对比 uniqueminimax 的 `/api/minimax/*`），统一经 handlers.go 的多处 `case "deepseek"|"opencode"` 分发 + `build*Current()` 构造卡片；menubar 卡片直取 `build*Current()`。
- **配置**：`internal/config/config.go` —— env 读取（341/385 行风格）、`HasProvider()` 732 行、`String()` 脱敏 861 行；`.env.example` 需补 Ark 项。
- **面板**：`internal/web/handlers.go` provider 注册表（约 1120 行）、`isProviderConfigured`（1144 行）、`dashboard_tabs.go`（zai/minimax/opencode/deepseek 四分支）、`server.go` 路由无需改动（统一分发）。
- **前置调研产物**：本次对话已完成 Ark 接口调研（见 §1），满足需求 ARK-01 / 交付物 D5。

## 3. 实施改动（Proposed Changes）

### 3.1 新建 Ark 适配器全栈（交付物 D1）

#### A. `server/internal/api/ark_types.go`（新建）
- `type ArkQuotaWindow struct { Quota, Used json.Number; SubscribeTime, ResetTime int64 }`（`json.Number` 兼容 string/number 两种官方返回）。
- `type ArkGetAFPUsageResponse struct { ResponseMetadata {...}; Result struct { PlanType string; AFPDaily, AFPFiveHour, AFPWeekly, AFPMonthly ArkQuotaWindow } }`。
- `type ArkWindowSnapshot struct { Name string; Quota, Used float64; Percent float64; ResetsAt, SubscribeAt *time.Time }`（Name ∈ five_hour/daily/weekly/monthly，映射 display 顺序 five_hour=1,daily=2,weekly=3,monthly=4）。
- `type ArkSnapshot struct { CapturedAt time.Time; RawJSON string; PlanType string; Windows []ArkWindowSnapshot }` + `resp.ToSnapshot(now)`：解析 `json.Number`，Quota=0 的窗口仍保留（展示 unconfigured 区分），Percent = Used/Quota*100，epoch ms → `time.UnixMilli`。

#### B. `server/internal/api/ark_client.go`（新建，模板 zai_client.go）
- 类型：`ArkClient{httpClient, accessKey, secretKey, baseURL, region, logger, now func() time.Time}`；options `WithArkBaseURL`、`WithArkTimeout`、`WithArkClock`（测试注入时间，golden 验签）。
- `NewArkClient(accessKey, secretKey string, logger, opts...)`：30s 超时 + 复用 zai/deepseek 同一套 Transport（`MaxIdleConns:1` 等）。
- 错误集：`ErrArkUnauthorized / ErrArkServerError / ErrArkNetworkError / ErrArkInvalidResponse / ErrArkAPIError / ErrArkRateLimited`。
- `FetchUsage(ctx)`：
  1. 固定 Query `Action=GetAFPUsage&Version=2024-01-01`，Body `{}`（`X-Content-Sha256` = hex(sha256(body))）。
  2. **HMAC-SHA256 V4 签名**（实现要点，must-pass）：`CanonicalRequest = POST\n/\n<canonical query>\ncontent-type:application/json\nhost:<host>\nx-content-sha256:<hash>\nx-date:<X-Date>\n\ncontent-type;host;x-content-sha256;x-date\n<hash>`；`StringToSign = "HMAC-SHA256\n<X-Date>\n<date>/<region>/ark/request\nhex(sha256(canonicalRequest))`；签名密钥链 `HMAC(SK, date)→HMAC(,region)→HMAC(,ark)→HMAC(,"request")`；`Authorization: HMAC-SHA256 Credential=<AK>/<date>/<region>/ark/request, SignedHeaders=content-type;host;x-content-sha256;x-date, Signature=<hex>`。
  3. 请求头：`Content-Type: application/json`、`Host`、`X-Date`（`YYYYMMDDTHHMMSSZ` UTC）、`X-Content-Sha256`、`Authorization`；`Accept: application/json`。
  4. 响应读限 1 MiB（需求 §3.2 cap）；HTTP 200 → 解析；`ResponseMetadata.Error` 存在 → 错误码含 `Signature/InvalidAccessKey/AccessDenied/Forbidden` 等判 `ErrArkUnauthorized`，其余 `ErrArkAPIError`（记 code+message）。
  5. 401/403 → `ErrArkUnauthorized`；429 → `ErrArkRateLimited`（读 `Retry-After`）；5xx → `ErrArkServerError`；网络 → `ErrArkNetworkError`；空/超大/解析失败 → `ErrArkInvalidResponse`；body 长度 > cap → 截断报错。
  6. 日志：`redactArkAccessKey`（前4+`***...***`+后3，同 `redactZaiAPIKey` 风格）；不打印 SecretKey、不打印响应体。
- `redactArkAccessKey(key string) string`。

#### C. `server/internal/api/ark_types_test.go` + `ark_client_test.go`（新建）
- `httptest.Server` mock：200 正常（string 型 Quota/Used 与 number 型各一例）→ 解析正确、Authorization 头结构正确（断言 `Credential=AK/<date>/cn-beijing/ark/request`、`SignedHeaders=content-type;host;x-content-sha256;x-date`、Signature 与 `WithArkClock` 固定时刻 golden 一致）；`ResponseMetadata.Error`（SignatureDoesNotMatch）→ `ErrArkUnauthorized`；401/429/500/畸形 JSON/空 body/超 1MiB；断言日志无明文 AK。
- 满足需求 ARK-06（mock 测试覆盖 client 解析）。

#### D. `server/internal/store/ark_store.go`（新建，模板 opencode_store / minimax_store）
- Schema（追加到 `store/store.go` migrateSchema，紧随 opencode 表 813 行后）：
  - `ark_snapshots(id INTEGER PRIMARY KEY AUTOINCREMENT, captured_at TEXT NOT NULL, raw_json TEXT, plan_type TEXT)`
  - `ark_quota_values(snapshot_id INTEGER, quota_name TEXT, quota REAL, used REAL, used_percent REAL, resets_at TEXT, subscribe_at TEXT, FOREIGN KEY(snapshot_id) REFERENCES ark_snapshots(id))`
  - `ark_reset_cycles(id INTEGER PRIMARY KEY AUTOINCREMENT, quota_name TEXT, cycle_start TEXT, cycle_end TEXT, reset_at TEXT, peak_utilization REAL, total_delta REAL)`
- 方法（镜像 opencode store 命名）：`InsertArkSnapshot`、`QueryLatestArk`、`QueryArkLatestPerQuota`、`QueryArkRange`、`QueryAllArkQuotaNames`、`QueryArkUtilizationSeries`、cycle CRUD（`CreateArkCycle`/`CloseArkCycle`/`QueryActiveArkCycle`/`QueryArkCycleHistory`/`QueryArkCycleOverview`）、`ArkLatestQuota{Name,Used,Limit,Utilization,ResetsAt,SubscribeAt,CapturedAt}`。
- 单账户模型，无 account_id（与 opencode 一致）。
- `ark_store_test.go`：in-memory sqlite 快照写入/回读/按时间范围/每窗口最新/cycle 生命周期。

#### E. `server/internal/tracker/ark_tracker.go`（新建，模板 opencode_tracker）
- `ArkTracker{store, logger}` + `Process(snapshot *api.ArkSnapshot) error`：对每窗口——检测 reset 边界（`ResetsAt` 跳过 → `CloseArkCycle`+`CreateArkCycle`）、更新 peak/delta、写 utilization 数据点。
- `UsageSummary(quotaName)` → `ArkSummary{CurrentUtil, CompletedCycles, PeakCycle, AvgPerCycle, TotalTracked, ResetsAt, TimeUntilReset, CurrentRate, ProjectedUtil}`（支撑卡片 projected 与 insights）。
- `ark_tracker_test.go`：mock store 场景——窗口内用量上升（无新 cycle）、跨 reset（新 cycle 开启、旧 cycle 关闭）。

#### F. `server/internal/agent/ark_agent.go`（新建，模板 zai_agent.go）
- `ArkAgent{client, store, tracker, interval, logger, sm, notifier, pollingCheck}`；`SetNotifier`/`SetPollingCheck`。
- `Run(ctx)`：立即 poll + `time.NewTicker(interval)`；defer `sm.Close()`。
- `poll()`：pollingCheck 短路 → `client.FetchUsage` → `ToSnapshot(now)` → `store.InsertArkSnapshot` → `tracker.Process`（失败仅记日志）→ `notifier.Check`（每窗口一条 QuotaStatus{Provider:"ark", QuotaKey:<window>, Utilization, Limit}）→ `sm.ReportPoll([]float64{五窗口 Used 值})` → 结构化 Info 日志（每窗口 budget/used/percent/reset）。
- `ark_agent_test.go`：带 mock store 的 poll 单测（可选，与 zai_agent_test 对齐）。

#### G. `server/internal/web/ark_handlers.go`（新建，模板 opencode_handlers.go）
- `SetArkTracker(t *tracker.ArkTracker)`；`arkQuotaDisplayOrder`（five_hour=1,daily=2,weekly=3,monthly=4）与 `arkDisplayNames`（"5-Hour","Daily","Weekly","Monthly"）。
- `buildArkCurrent()`：`QueryArkLatestPerQuota` 排序 → 每窗口 `{name, displayName, utilization, used, limit, format:"percent", status:utilStatus, lastUpdatedAt, ageSeconds, isStale, resetsAt, timeUntilReset, timeUntilResetSeconds, currentRate, projectedUtil}` + `capturedAt` + `planType`；`applyDisplayModeToResponse`。
- Handler 函数（供 handlers.go 分发改用，**不新增路由**）：`currentArk` / `historyArk` / `cyclesArk` / `cycleOverviewArk` / `summaryArk` / `insightsArk` / `loggingHistoryArk`（镜像 opencode 各 handler；改查 Ark store/tracker；display 顺序、quotaNames 前缀 `ark_` 等价物为窗口名）。

### 3.2 接线（既有文件修改）

#### H. `server/main.go`（复刻 deepseek 全套位点）
1. 客户端（约 983 行后）：`var arkClient *api.ArkClient; if cfg.HasProvider("ark") { arkClient = api.NewArkClient(cfg.ArkAccessKey, cfg.ArkSecretKey, logger, api.WithArkBaseURL(cfg.ArkBaseURL)); logger.Info("Ark API client configured") }`。
2. Tracker（约 1206 行后）：`var arkTr *tracker.ArkTracker; if cfg.HasProvider("ark") { arkTr = tracker.NewArkTracker(db, logger) }`。
3. Agent（约 1276 行后）：`arkSm := agent.NewSessionManager(db, "ark", idleTimeout, logger); arkAg = agent.NewArkAgent(arkClient, db, arkTr, cfg.PollInterval, logger, arkSm)`（仅当 `arkClient != nil`）。
4. `SetNotifier` 链（1377 行 opencode 后）：`arkAg.SetNotifier(notifier)`。
5. `SetPollingCheck` 链（1540 行 opencode 后）：`arkAg.SetPollingCheck(func() bool { return isPollingEnabled("ark") })`。
6. `handler.SetArkTracker(arkTr)`（1642 行 `SetDeepSeekTracker` 附近）。
7. `agentMgr.RegisterFactory("ark", func() (agent.AgentRunner, error) { return arkAg, nil })`（1704 行 opencode 后）。
8. 启动列表（1750 行）加入 `"ark"`。

#### I. `server/internal/config/config.go`
- 结构体字段：`ArkAccessKey`（`ARK_ACCESS_KEY`）、`ArkSecretKey`（`ARK_SECRET_KEY`）、`ArkRegion`（`ARK_REGION`，默认 `cn-beijing`）、`ArkBaseURL`（`ARK_BASE_URL`，默认 `https://ark.cn-beijing.volcengineapi.com/`，供测试/代理覆盖）。
- env 读取（385 行 DeepSeekAPIKey 附近）。
- `HasProvider` 增加 `case "ark": return c.ArkAccessKey != "" && c.ArkSecretKey != ""`（752 行附近）。
- providerConfigured 汇总（798/813 行附近）与 `String()` 脱敏（861 行附近，用现有 `redactAPIKey`）。

#### J. `server/internal/web/handlers.go`
- provider 注册表（1120 行 near deepseek）：`{Key: "ark", Name: "Volcano Ark", Description: "Volcano Engine Ark AFP quota tracking"}`。
- `isProviderConfigured` 增加 `case "ark": return h.config.ArkAccessKey != "" && h.config.ArkSecretKey != ""`（1183 行附近）。
- providerSettings（1444 行 `"opencode": {...}` 旁）加 ark 默认块；`provSettings["ark"]`（1550 行附近）。
- **逐处镜像 deepseek/opencode 分发**（`case "deepseek"|"opencode"` 出现的所有 switch 与 `HasProvider(...)` 聚合点，共 2 类路径）：新增 `case "ark"` → 对应 `h.currentArk/historyArk/...`；聚合点加 `if h.config.HasProvider("ark") && providerTelemetryEnabled(visibility, "ark") { response["ark"] = h.buildArkCurrent()/... }`。执行方式：grep `"opencode"` 于 handlers.go，凡 deepseek/opencode 成对出现即补 ark 对。
- `dashboard_tabs.go`（43/47 行 opencode/deepseek 旁）加 `case "ark"`。

#### K. 杂项
- `server/.env.example`：追加 `ARK_ACCESS_KEY=`、`ARK_SECRET_KEY=`、`ARK_REGION=cn-beijing`、`ARK_BASE_URL=`（注释说明获取位置与安全提示）。
- `server/internal/web/menubar.go` 及 menubar companion：grep `opencode` 加入 ark 卡片/汇总分支（若 opencode 缺席则仅保持统一端点）。

### 3.3 既有适配器验证（DSK-01~03 / ZHP-01~03 / OPC-01~03，mock 为主）

- **回归确认**（不改行为）：`go test ./...` 全绿即证明 deepseek/zai/opencode 现有 mock 用例无回归；新增各一个"凭证缺失 → unconfigured"场景断言（若既有测试已覆盖则跳过）。
- **真实验证留待用户凭证**（写入 D2/D4 文档 + 验收清单注明"用户提供凭证后执行"，不作跳过失败处理）。
- `ZAI_REGION=cn` 路径：仅文档化（`ZAI_BASE_URL` 需指向智谱开放平台 cn 域名的用法说明），代码零改动（config 已支持 ZaiRegion）。

### 3.4 文档交付物回写

- `docs/requirements-fr-1.1-p0-adapters.md`：
  - §1.1 表中"endpoint TBD"→ `GetAFPUsage`（四窗口）。
  - §3.2 表格 Ark 列：Env vars `ARK_ACCESS_KEY`/`ARK_SECRET_KEY`（可选 `ARK_REGION`/`ARK_BASE_URL`）、Endpoint `https://ark.cn-beijing.volcengineapi.com/?Action=GetAFPUsage&Version=2024-01-01`、Method POST、Auth **HMAC-SHA256 V4（AK/SK）**、cap 1 MiB、超时 30 s。
  - ARK-02：路由措辞改为"接入统一分发端点（current/history/cycles/summary/insights/logging-history），不新增专属路由，与 DeepSeek/OpenCode 一致"。
  - ARK-03：env var → `ARK_ACCESS_KEY` + `ARK_SECRET_KEY`。
- `docs/ark-interface-research.md`（新建，D5）：GetAFPUsage 端点、V4 签名步骤、响应 schema、AK/SK 获取方式（IAM 密钥管理页）、限流与 GetSeatAFPUsage 对照、`GetInferenceUsage`（ApiKeyID 查询，2026-08-12 后新 Key）备注。
- `docs/credential-guide.md`（新建，D3）：四平台 env 变量清单、获取位置、脱敏/安全注意事项。
- `docs/verification-fr-1.1.md`（新建，D2）：TC-01~12 结果表框架 + 真实验证操作步骤（用户凭证）。
- `docs/test-report-fr-1.1.md`（新建，D4）：执行矩阵、`go test ./...` 结果、内存/时延抽样、开放项（未执行的真实验证列出原因）。
- `docs/phase2-plan.md` / `docs/roadmap.md`：P2-09~P2-16 状态与 D1–D5 链接勾选。

## 4. 决策与假设（Assumptions & Decisions）

| # | 决策 | 依据 |
|---|---|---|
| D1 | 凭证 env：`ARK_ACCESS_KEY` + `ARK_SECRET_KEY`（**用户已确认**） | GetAFPUsage 仅支持 AK/SK 鉴权 |
| D2 | 路由：**统一分发，不新增 `/api/ark/*`**（用户采纳推荐项） | 与 deepseek/opencode 一致，代码量最小；同步修订 ARK-02 措辞 |
| D3 | 接入深度：**完整对齐 OpenCode 模式**（快照双表 + 周期统计 + summary/insights/logging-history） | 需求 ARK-02"完整 adapter stack" |
| D4 | 启用规则：两 key 均非空即视为配置（不引入 `ARK_ENABLED`） | 与 DEEPSEEK 惯例一致；缺失 → `unconfigured`，agent 不启动 |
| D5 | GetAFPUsage 仅覆盖个人版 Agent Plan；企业版 GetSeatAFPUsage 不在 P0 | 需求 ARK-01 调研结论（D5 文档记录） |
| D6 | Quota/Used 用 `json.Number` 容错解析（官方 string 与 number 两版返回） | 官方文档与 API Explorer 示例不一致 |
| D7 | 真实验证（TC-01~05 有效凭证）**不在本机执行**——沙箱无火山 AK/SK 及三家付费凭证；改为 mock 单测全覆盖 + 文档化操作步骤，验收清单标注待办 | 环境约束 |
| D8 | schema 迁移采用 `CREATE TABLE IF NOT EXISTS` 追加至 `store.go` migrateSchema（opencode 表后） | 既有迁移幂等模式 |
| D9 | 单账户模型（无 account_id） | 与 opencode 一致；Ark 个人版单套餐 |

## 5. 验证步骤

1. `cd server && go build -o onwatch.exe .` 编译通过。
2. `go test ./...` 全绿（新增 ark 全套 client/tracker/store/agent 单测 + 既有回归）。
3. 无凭证启动 `.\onwatch.exe`：日志无"Ark agent started"（未配置），面板 ark 卡片 `unconfigured`，无错误刷屏（E2E-1）。
4. 伪造 `ARK_ACCESS_KEY`/`ARK_SECRET_KEY` 启动：agent 启动、轮询走 mock/通配失败返回 `error`（网络 401 由 mock 测试覆盖），面板不崩、无明文 key 日志（TC-12 部分）。
5. `/api/providers`、`/api/current`（provider=ark 空数据兜底）与 dashboard 渲染无回归。
6. 文档回写完成后核对 §7 验收标准 1~5 项勾选记录（真实验证项标注待用户凭证）。

## 6. 实施顺序（Todo 划分解引用）

1. Ark api 层（types + client + 单测）→ 2. store（schema + 方法 + 单测）→ 3. tracker（+单测）→ 4. agent（+单测）→ 5. web handlers + handlers.go 分发 + menubar + tabs → 6. main.go/config/env 接线 → 7. 构建与全量测试 → 8. 验证文档/需求回写 → 9. 启动冒烟验收。