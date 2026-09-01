<!--
 * @Author: guotao
 * @Date: 2026-09-01
 * @Description: Phase 3 第一批实施计划：P3-01 统一指标转换层 + P3-02 统一指标 REST API
 *
 * Copyright (c) 2026 by lzlj, All Rights Reserved.
-->
# Phase 3 第一批实施计划：P3-01 统一转换层 + P3-02 统一指标 REST API

## 一、概要（Summary）

实现 Phase 3 第一批两个任务（对应 [phase3-plan.md](../../docs/phase3-plan.md)）：

1. **P3-01 统一指标转换层**：新建 `internal/metrics` 包，把 **P0 四家**（Ark / Zai / DeepSeek / OpenCode）的最新快照转换为 `generic.PlatformSnapshot` 统一模型（覆盖范围经用户确认，其余 12 家后续批次补齐）。
2. **P3-02 统一指标 REST API**：在 web 层新增 3 个端点——`GET /api/platforms`（平台目录+状态）、`GET /api/metrics`（全部平台统一指标）、`GET /api/metrics/{platform}`（单平台），内置平台与 generic 平台在同一输出中共存。

不涉及：WebSocket（P3-03）、display 过滤（P3-11）、前端 UI、其余 12 家 provider 转换。

## 二、现状分析（代码勘察结论）

| 事实 | 位置 |
|---|---|
| 统一模型契约已定义：`Metrics{Quota []QuotaMetric, Balance, Cost, Tokens}` / `PlatformSnapshot{Platform, DisplayName, Status, Error, UpdatedAt, Metrics}`，状态常量 `ok/error/auth_failed/unconfigured` | [generic/metrics.go:12-65](../../server/internal/generic/metrics.go) |
| 路由全部集中在 `NewServer`，**鉴权为全局中间件**（session → securityHeaders → gzip → csrf 包裹整个 mux），新路由自动受保护 | [web/server.go:81-191](../../server/internal/web/server.go) |
| provider 目录含 16 家的 `Key/Name`（显示名来源） | [web/handlers.go:1110-1129](../../server/internal/web/handlers.go) `providerCatalog()` |
| `isProviderConfigured` 逐 provider 判断配置 | [web/handlers.go:1146](../../server/internal/web/handlers.go) |
| 可见性门控：`providerTelemetryEnabled(visibility, key)`（currentBoth 同款） | [web/handlers.go](../../server/internal/web/handlers.go) |
| generic 平台快照已持久化为统一模型 JSON，可直接复用 | [store/generic_store.go:71-104](../../server/internal/store/generic_store.go) `GetAllGenericSnapshots()`；配置列表 `generic.LoadPlatforms(store)` |
| Ark 查询：`QueryArkLatestPerQuota() ([]store.ArkLatestQuota, error)`（`ORDER BY quota_name ASC`，需按展示顺序重排）；兜底 `QueryLatestArk() (*api.ArkSnapshot, error)`（无数据返回 `nil, nil`） | [store/ark_store.go:381/80](../../server/internal/store/ark_store.go) |
| OpenCode 查询：`QueryOpenCodeLatestPerQuota() ([]store.OpenCodeLatestQuota, error)`；兜底 `QueryLatestOpenCode()` | [store/opencode_store.go:377/78](../../server/internal/store/opencode_store.go) |
| DeepSeek 查询：`QueryLatestDeepSeek() (*api.DeepSeekSnapshot, error)`（`DeepSeekSnapshot{CapturedAt, IsAvailable, Currency, TotalBalance, GrantedBalance, ToppedUpBalance}`，无配额） | [store/deepseek_store.go:53](../../server/internal/store/deepseek_store.go) |
| Zai 查询：`QueryLatestZai() (*api.ZaiSnapshot, error)`（TIME_LIMIT 五小时窗口字段 `TimeLimit/TimeUsage/TimePercentage`；TOKENS_LIMIT 周窗口 `TokensLimit/TokensUsage/TokensPercentage/TokensNextResetTime *time.Time`，ADR-015） | [api/zai_types.go:53](../../server/internal/api/zai_types.go) |
| Ark 展示顺序 helper 已存在：`arkQuotaDisplayOrder`（five_hour=1, daily=2, weekly=3, monthly=4） | [web/ark_handlers.go](../../server/internal/web/ark_handlers.go) |
| JSON 输出 helper：`respondJSON(w, code, v)` | [web/handlers.go](../../server/internal/web/handlers.go) |

## 三、需求拆解

### R1 统一指标转换层（P3-01）

#### R1.1 转换总则（所有 converter 遵守）

1. **纯函数**：不访问 store/config/网络，输入快照数据、输出 `*generic.Metrics`，便于单测。
2. **时间格式**：所有 `ResetAt` / `UpdatedAt` 输出 RFC3339 UTC 字符串（`time.Format(time.RFC3339)`）；`*time.Time` 为 nil 或零值时输出空串 `""`。
3. **三种窗口范式**（后续 12 家复用的归一策略）：
   - Used-based（直接填 used/total/percent）：ark、opencode
   - Remaining-based（used = total − remaining）：zai（有 usage 直读，无需反推）
   - Percent-only（Used/Total 置 0，仅填 `Percent` + `Unit: "percent"`，**不编造数据**）：本批四家均不涉及，但规则写入 helper 供后续复用
4. **Percent 兜底**：`Total > 0 && Percent == 0` 时按 `Used/Total*100` 计算（镜像 `generic.buildMetrics` 行为）。
5. **Unit 语义**：仅在单位明确时填写（如 opencode 的 `Format` 字段值、deepseek 余额币种放 `Balance.Currency`）；单位未知填 `""`，不猜测。
6. **空值语义**：无数据的指标维度保持零值/nil（`Quota` 为空 slice 而非 nil、`Balance/Cost/Tokens` 无数据为 nil），**不输出假 0**。

#### R1.2 Ark 映射（`ConvertArk(quotas []store.ArkLatestQuota) *generic.Metrics`）

| 统一字段 | 来源 | 说明 |
|---|---|---|
| `Quota[].Window` | `ArkLatestQuota.Name` | `five_hour/daily/weekly/monthly` 原名透传 |
| `Quota[].Used` | `.Used` | |
| `Quota[].Total` | `.Limit` | |
| `Quota[].Percent` | `.Utilization` | 兜底规则 R1.1-4 适用 |
| `Quota[].ResetAt` | `.ResetsAt` | nil → `""` |
| `Quota[].Unit` | `""` | AFP 官方单位未知，不猜测 |
| `Balance/Cost/Tokens` | — | nil（Ark 无此数据） |

**顺序**：按 `five_hour → daily → weekly → monthly` 固定展示顺序排序（镜像 [ark_handlers.go](../../server/internal/web/ark_handlers.go) `arkQuotaDisplayOrder`），未知窗口名排最后。

#### R1.3 Zai 映射（`ConvertZai(snap *api.ZaiSnapshot) *generic.Metrics`）

| 统一字段 | 来源 | 说明 |
|---|---|---|
| 窗口 `"5h"` | `TimeLimit/TimeUsage/TimePercentage` | Used=TimeUsage、Total=TimeLimit、Percent=TimePercentage；**ResetAt=`""`**（ADR-015：动态刷新无固定重置） |
| 窗口 `"weekly"` | `TokensLimit/TokensUsage/TokensPercentage/TokensNextResetTime` | ResetAt=TokensNextResetTime（nil → `""`） |
| `Balance/Cost/Tokens` | — | nil（zai 的 tokens 字段是配额窗口，非用量统计） |

窗口仅在对应 limit > 0 时输出（国内版 CREDIT_LIMIT 可能只有部分窗口，ADR-015）。

#### R1.4 DeepSeek 映射（`ConvertDeepSeek(snap *api.DeepSeekSnapshot) *generic.Metrics`）

| 统一字段 | 来源 |
|---|---|
| `Balance.Amount` | `TotalBalance` |
| `Balance.Currency` | `Currency`（如 CNY/USD） |
| `Quota/Cost/Tokens` | 空 / nil（DeepSeek 纯余额平台） |

#### R1.5 OpenCode 映射（`ConvertOpenCode(quotas []store.OpenCodeLatestQuota) *generic.Metrics`）

| 统一字段 | 来源 | 说明 |
|---|---|---|
| `Quota[].Window` | `.Name` | 原名透传，保持 store 返回顺序 |
| `Quota[].Used/Total/Percent` | `.Used/.Limit/.Utilization` | 兜底规则适用 |
| `Quota[].ResetAt` | `.ResetsAt` | nil → `""` |
| `Quota[].Unit` | `.Format` | 非空才填 |
| `Balance/Cost/Tokens` | — | nil |

#### R1.6 状态判定（web 层组装时应用，不在 converter 内）

| 条件 | Status | Metrics | Error | UpdatedAt |
|---|---|---|---|---|
| 未配置（`!isProviderConfigured`）或不可见（telemetry 禁用） → **不输出该平台** | — | — | — | — |
| 已配置 + 查询出错 | `error` | nil | 错误信息 | `""` |
| 已配置 + 无快照 | `error` | nil | `"awaiting first poll"` | `""` |
| 已配置 + 有快照 | `ok` | 转换结果 | — | `CapturedAt` |

说明：契约状态枚举仅 4 种（[generic/metrics.go:53](../../server/internal/generic/metrics.go)），phase3-plan 提及的 `stale` 状态**本批不实现**（`updated_at` 已足够客户端判断新鲜度，stale 留待 P3-11 展示配置时按需引入）。

#### R1.7 generic 平台直通

- 遍历 `generic.LoadPlatforms(h.store)` 配置；`GetAllGenericSnapshots()` 命中 → 直接使用存量 `PlatformSnapshot`；未命中（配置了但从未轮询）→ 补 `PlatformSnapshot{Status: unconfigured}`。
- generic 平台不做任何转换（配置驱动时已是统一模型）。

### R2 统一指标 REST API（P3-02）

#### R2.1 `GET /api/platforms` — 平台目录

```json
[
  {"platform": "ark", "display_name": "Volcano Ark", "status": "ok"},
  {"platform": "zai", "display_name": "Z.ai", "status": "ok"},
  {"platform": "deepseek", "display_name": "DeepSeek", "status": "unconfigured"},
  {"platform": "opencode", "display_name": "OpenCode Go", "status": "error"},
  {"platform": "my-zhipu", "display_name": "My Zhipu", "status": "ok"}
]
```

- 输出规则：内置平台 = 目录 16 家中「telemetry 启用 且（已配置 或 库中有快照）」；generic 平台全部列出（含 unconfigured）。
- `display_name`：内置取 `providerCatalog().Name`；generic 取配置 `DisplayName`。
- 状态判定同 R1.6；generic 未轮询 → `unconfigured`。

#### R2.2 `GET /api/metrics` — 全部平台统一指标

- 返回 `[]generic.PlatformSnapshot` 数组（内置 + generic 混合，与 R2.1 同一输出集合）。
- 组装过程任何单平台查询失败不影响整体响应（该平台置 `error` 状态，HTTP 仍 200）。

#### R2.3 `GET /api/metrics/{platform}` — 单平台

- 路径解析：`TrimPrefix(basePath+"/api/metrics/")`（generic handlers 同款手工解析，不用 go 1.22 path pattern，保持风格一致）。
- 平台在输出集合中 → `200` 单个 `PlatformSnapshot`；不在 → `404 {"error":"platform not found"}`。

#### R2.4 通用约束

- 非 GET 请求 → `405`（三个端点一致）。
- 鉴权：自动继承全局 session 中间件，**无需额外处理**。
- 输出统一走 `respondJSON`。

## 四、变更清单

| # | 文件 | 动作 | 内容 |
|---|---|---|---|
| 1 | `server/internal/metrics/unified.go` | 新建 | 包 `metrics`；helper（`rfc3339(*time.Time) string`、percent 兜底、percent-only 窗口 helper、ark 排序）；4 个导出转换函数 `ConvertArk/ConvertZai/ConvertDeepSeek/ConvertOpenCode`（签名见 R1.2–R1.5）。依赖仅 `internal/generic`、`internal/store`、`internal/api`、`time`、`sort`——无循环依赖（store → generic 已存在，web → metrics 单向） |
| 2 | `server/internal/metrics/unified_test.go` | 新建 | 见 §七-1 测试清单（约 14 个用例） |
| 3 | `server/internal/web/unified_handlers.go` | 新建 | `platformSummary` 类型；`(h *Handler) Platforms / UnifiedMetrics / UnifiedPlatformMetrics`；内部组装函数 `collectUnifiedSnapshots() ([]generic.PlatformSnapshot, map[string]string /*displayNames*/)`：4 家内置（可见性门控 + configured/hasData 门控 + store 查询 + converter 调用 + R1.6 状态）+ generic 直通（R1.7）；display 名映射从 `providerCatalog()` 构建。Ark/OpenCode 无 PerQuota 数据时走 `QueryLatestArk/QueryLatestOpenCode` 兜底（把快照内窗口映射成切片后仍调用同一 converter） |
| 4 | `server/internal/web/unified_handlers_test.go` | 新建 | httptest 覆盖：200/404/405、unconfigured 剔除、error 平台（无快照）、generic unconfigured、数据正确性（用临时 sqlite store 夹具，沿用 web 包既有测试模式） |
| 5 | `server/internal/web/server.go` | 修改 | 在 `/api/current` 注册行之后追加 3 行：`mux.HandleFunc(p("/api/platforms"), handler.Platforms)`、`mux.HandleFunc(p("/api/metrics"), handler.UnifiedMetrics)`、`mux.HandleFunc(p("/api/metrics/"), handler.UnifiedPlatformMetrics)` |

注意：`/api/metrics`（精确）与 `/api/metrics/`（前缀）同时注册，Go ServeMux 精确优先，无冲突；与 Prometheus `/metrics`、`/api/generic/metrics` 均不冲突。

## 五、假设与决策（已与用户确认 + 实施决策）

| # | 决策 | 来源 |
|---|---|---|
| D1 | P3-01 首批仅覆盖 Ark/Zai/DeepSeek/OpenCode + generic，其余 12 家后续批次 | 用户确认 |
| D2 | 多账号 provider 预留「每账号一条」形态（platform key 形如 `codex:2`）；本批四家均为单账号，注册表设计不阻塞该形态 | 用户确认 |
| D3 | Percent-only 窗口：Used/Total 置 0、仅填 Percent、Unit="percent"，不编造数据 | 用户确认 |
| D4 | 状态仅用契约 4 种枚举；`stale` 延后至 P3-11；「已配置无数据」= `error` + `"awaiting first poll"` | 实施决策 |
| D5 | 平台输出集合 = telemetry 启用 且（已配置 或 有历史快照）；generic 平台全列 | 实施决策（与 currentBoth 门控兼容，目录更完整） |
| D6 | 转换层放 `internal/metrics` 新包（纯函数），组装/状态/鉴权留在 web 层（复用 `isProviderConfigured`、detect 系列，不复制逻辑） | 实施决策 |
| D7 | 路由用既有「普通路径 + TrimPrefix」风格，不引入 go 1.22 pattern | 实施决策 |
| D8 | zai 窗口命名 `"5h"`/`"weekly"`；ark/opencode 窗口名透传 | 实施决策 |
| D9 | 时间统一 RFC3339 UTC | 实施决策 |

## 六、明确不做（本批）

其余 12 家 provider 转换（anthropic/codex/copilot/synthetic/antigravity/minimax/openrouter/moonshot/gemini/cursor/grok/kimi）、WebSocket 推送（P3-03）、display 配置过滤（P3-10/11）、前端消费改造、`stale` 状态、多账号展开、token/费用采集（P3-04）。

## 七、验证步骤

1. **转换层单测**（`go test ./internal/metrics/`，Go 路径 `D:\learning_code\07-Ai-mester\go\bin\go.exe`）：
   - ConvertArk：全 4 窗口顺序断言（five_hour→daily→weekly→monthly）、部分窗口缺失、nil ResetsAt、percent 兜底、空输入返回空 Metrics
   - ConvertZai：双窗口、仅国内版单窗口（limit=0 窗口不输出）、weekly ResetAt、5h ResetAt 为空
   - ConvertDeepSeek：余额+币种透传、Quota 为空 slice
   - ConvertOpenCode：字段映射、Format 透传为 Unit、store 顺序保持
   - percent-only helper：Used/Total=0 且 Unit=percent
2. **web 层单测**（`go test ./internal/web/ -run Unified`）：三端点 200/404/405、unconfigured 剔除、无快照平台 error 状态、generic unconfigured 补位、输出字段完整
3. **构建**：`go build ./...`（server 目录）
4. **全量回归**：`go test ./...`（既有 Windows 例外项单独标注，延续 P2 口径）
5. **curl 冒烟**（重启 onwatch 后）：
   - `GET /api/platforms`：zai（已配置且有数据）→ `ok`；未配置 provider 不出现或目录合理
   - `GET /api/metrics/zai`：数值与面板 `/api/current?provider=zai` 卡片一致（交叉验证）
   - `GET /api/metrics/nonexistent`：404
   - generic 平台（如有配置）出现在输出中

## 八、执行任务清单

| # | 任务 | 交付 |
|---|---|---|
| t1 | `internal/metrics/unified.go` + `unified_test.go`（R1 全部） | 4 个 converter 全绿 |
| t2 | `internal/web/unified_handlers.go`（R2 组装 + 状态判定 + generic 直通） | 编译通过 |
| t3 | `server.go` 挂 3 条路由 | 编译通过 |
| t4 | `unified_handlers_test.go` | web 层用例全绿 |
| t5 | 构建 + 全量回归 + curl 冒烟 + phase3-plan.md 勾选 P3-01/P3-02 状态 | 验证记录 |
