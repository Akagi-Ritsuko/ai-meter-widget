<!--
 * @Author: guotao
 * @Date: 2026-09-01
 * @FilePath: \07-Ai-mester\ai-meter-widget\docs\phase3-plan.md
 * @Description: Phase 3 统一指标 API + 展示配置 需求分解与任务计划
 *
 * Copyright (c) 2026 by lzlj, All Rights Reserved.
-->
# Phase 3 需求分解与任务计划（统一指标 API + 展示配置）

> 对应里程碑：[roadmap.md](roadmap.md) 第 4 节「Phase 3 统一指标 API + 展示配置」
> 覆盖需求：FR-2.1~2.4 / FR-6.1 / FR-7.1~7.2（并最终兑现 NFR-4、支撑 Phase 4/5 展示端）
> 状态：第一批（P3-01、P3-02）✅ + 第二批（P3-06、P3-07、P3-09）✅ + 第三批（P3-04、P3-05）✅ 已完成（2026-09-01）；剩余第四批（P3-10、P3-11、P3-03）、第五批（P3-12）未开工
> 最后更新：2026-09-01 | 决策依据：[decisions.md](decisions.md)（实施中新增 ADR 从 ADR-016 起编）

## 1. Phase 3 目标与范围

**目标**：把各平台（内置适配器 + 通用适配器）的指标归一到 architecture.md §2 定义的统一指标模型，通过 REST + WebSocket 对外暴露；实现 90 天历史趋势与重置倒计时；建立展示端配置管理，让桌面浮窗（Phase 4）与墨水屏（Phase 5）可以"零改动聚合服务"接入。

**范围**：

| 需求编号 | 需求 | 在本计划中的落地方式 |
|---|---|---|
| FR-2.1 | 配额剩余（窗口/已用/总量/百分比/重置时间） | 统一指标转换层 + `/api/metrics` 输出（复用各 provider 快照） |
| FR-2.2 | 余额（金额/币种） | 同上（DeepSeek/OpenRouter 等已有余额数据直接映射） |
| FR-2.3 | token 消耗（今日/周期） | 数据可得性评估 + 缺口采集 + 统一输出 |
| FR-2.4 | 费用（今日/周期/币种） | 同上 |
| FR-6.1 | 展示端配置（平台/指标/刷新频率） | 展示端配置模型 + CRUD API + 配置页 UI |
| FR-7.1 | 90 天历史趋势 | history API 扩展 90d + 面板趋势图表 |
| FR-7.2 | 重置倒计时 | 统一输出 `reset_at`，面板倒计时展示 |
| NFR-4 | 可扩展（零代码接入） | 通用适配器平台自动进入统一 API；展示端零改动扩展 |

**明确不做**（本期）：桌面浮窗（Phase 4）、告警阈值推送增强（Phase 4）、ESP32 固件（Phase 5）、多账号聚合报表。

## 2. 现状基线（代码勘察结论，2026-09-01）

任务的拆分基于以下代码现状，避免重复实现：

| 项 | 现状 | 涉及位置 |
|---|---|---|
| 统一指标模型 | ✅ 已定义且与 architecture.md §2 契约一致 | `internal/generic/metrics.go:12-57`（`Metrics`/`QuotaMetric`/`BalanceMetric`/`CostMetric`/`TokensMetric`/`PlatformSnapshot`） |
| 内置 provider → 统一模型转换 | ❌ 不存在（15+ provider 各自输出私有 map 格式） | `internal/web/handlers.go` 各 `buildXxxCurrent()` |
| 通用适配器 → 统一模型 | ✅ 已有（JSONPath 映射直接产出统一模型） | `internal/generic/adapter.go` + `store/generic_store.go` |
| `/api/current` 聚合 | ✅ 存在，17 个 provider key 门控输出（私有格式） | `handlers.go:2285-2370` `currentBoth` |
| `/api/platforms`、`/api/metrics`、`/api/metrics/:platform`、`/api/history/:platform`、`/api/config/*` | ❌ 均不存在 | `internal/web/server.go:81-180`（无挂载） |
| history API | ⚠️ 存在但 range 最大 30d，按 `?provider=` 查询 | `handlers.go:913-932` `parseTimeRange`（1h/6h/24h/7d/30d） |
| 快照历史数据 | ✅ 各 provider `*_snapshots` 表永久保留，数据可回溯到部署首日 | store 层无任何快照裁剪逻辑 |
| 保留策略 | ⚠️ 仅 `api_integration_usage_events` 有 60 天裁剪（`store/api_integrations_store.go:156-161`）；provider 快照无 prune，SQLite 会无限增长 | — |
| WebSocket | ❌ 无服务端实现（gorilla/websocket v1.5.3 已在 go.mod，仅 CDP 客户端使用） | `go.mod:26`、`internal/api/ark_cookie_cdp.go:11` |
| 展示端配置 | ❌ 不存在（仅有 per-provider `display_mode` 与 menubar 托盘选择，均非展示端范畴） | `handlers.go:8266-8284`、`internal/menubar/config.go:49-58` |
| 重置倒计时 | ⚠️ 数据已具备（内置快照 `ResetsAt`、generic `reset_at`），无统一输出与面板倒计时组件 | 各 provider 快照结构 |

**结论**：Phase 3 的核心工作量在「统一转换层 + 新 API 面 + WS 推送 + 展示配置」，历史数据本身已齐备；唯一需要新建的存储逻辑是快照保留策略（可选裁剪），唯一需要新建的采集是 token/费用缺口补齐。

## 3. 任务分解

任务按 6 个工作流（W1~W6）组织，ID 前缀 `P3-`。每个任务包含：**目标 / 范围 / 验收标准 / 优先级**。

优先级定义：**P0** = Phase 3 验收必需；**P1** = 质量与完整性需要；**P2** = 增强项。

### W1 统一指标 API（FR-2.1/2.2 核心）

#### P3-01 内置 provider 统一指标转换层

- **需求映射**：FR-2.1 / FR-2.2 的前置基础
- **目标**：让 15+ 内置 provider 的快照数据归一为 `generic.PlatformSnapshot`（status/metrics.quota/balance）
- **范围**：
  - 新建 `internal/metrics/`（或 `internal/web/unified.go`，开工时定）转换包：每 provider 一个 `Converter`（输入该 provider 最新快照/active cycle，输出统一模型）
  - 状态映射：统一 `ok`/`error`/`auth_failed`/`unconfigured`/`stale`（快照过旧时标 stale，口径与面板 isStale 一致）
  - quota 窗口映射：窗口名（5h/daily/weekly/monthly/cp_* 等）、used/total/percent、`reset_at`（RFC3339）
  - balance 映射：DeepSeek/OpenRouter 等已有余额字段的 provider
  - 通用适配器平台直接复用 `generic.GetPlatformMetrics`，无需转换
  - 注册表模式：`map[providerKey]Converter`，新增 provider 只需注册一个转换器
- **验收标准**：
  - [x] 全部已启用 provider 的统一转换单测覆盖（含 quota 多窗口、余额、无数据/未配置分支）——首批 P0 四家 Ark/Zai/DeepSeek/OpenCode，18 用例全绿；其余 12 家按后续批次补齐
  - [x] 转换层不发起任何网络请求（纯读 store）
  - [x] `go test ./...` 无回归（internal/metrics、web、generic、store 全绿；既有例外单独标注：web 1 例 Windows UserHomeDir、tools/perf-monitor 工具包超时，均与本批无关）
- **优先级**：P0

#### P3-02 统一指标 REST API

- **需求映射**：FR-2.1 / FR-2.2
- **目标**：按 architecture.md §3 API 层契约暴露统一指标
- **范围**：
  - `server.go` 挂载：`GET /api/platforms`（平台列表+状态）、`GET /api/metrics`（全部平台）、`GET /api/metrics/{platform}`（单平台）
  - 响应体 = architecture.md §2 JSON 契约（`platform`/`display_name`/`status`/`updated_at`/`metrics{}`）
  - 鉴权与现有 `/api/*` 中间件一致；`display_mode`（usage/available）继续生效于 percent 语义
  - 输出包含内置 + 通用适配器平台（通用平台走 generic store）
- **验收标准**：
  - [x] 三个端点认证后返回 200，结构符合契约（curl 冒烟：隔离实例 + BasicAuth，2026-09-01）
  - [x] `/api/metrics/{platform}` 对未知平台返回 404 + 明确错误
  - [x] 至少 1 个内置平台与 1 个通用平台同时在输出中（端到端：zai + generic gen-smoke，旧 DisplayName 被当前配置名覆盖）
  - [x] 单测覆盖 handler 分支（200/404/405/未启用 provider 过滤/awaiting first poll，7 用例）
- **优先级**：P0

#### P3-03 WebSocket 指标推送

- **需求映射**：architecture.md §3（`/ws`）、§6 展示层接入协议；支撑 Phase 4/5
- **目标**：展示端可订阅实时指标推送，替代轮询
- **范围**：
  - `gorilla/websocket` 转 direct 依赖；新建 `internal/web/ws_hub.go`（Hub + upgrader + 客户端管理）
  - `GET /ws` 路由（复用 API 鉴权；token 走 query 参数，WS 无法带 header）
  - 推送时机：轮询写入快照后广播增量（provider key + 统一指标载荷）；推送间隔合并（≥5s 去抖，防风暴）
  - 客户端断开清理、ping/pong 保活
- **验收标准**：
  - [ ] ws 客户端连接后能在轮询周期内收到指标消息（集成测试 + 手工 wscat 验证）
  - [ ] 断开的客户端不再占用资源（连接数不泄漏）
  - [ ] 多客户端并发接收一致
- **优先级**：P1（REST 已满足一期验收，WS 为 Phase 4/5 铺路）

### W2 token/费用统计（FR-2.3/2.4）

#### P3-04 token/费用数据可得性评估与缺口补齐

- **需求映射**：FR-2.3 / FR-2.4
- **目标**：明确每 provider 的 tokens/cost 数据来源；可采的采齐，不可得的明确标注
- **范围**：
  - 逐 provider 盘点：内置适配器现有快照中已含 tokens/cost 的（如 Codex/Anthropic 会话数据、OpenRouter 费用）；官方有用量 API 但未接入的（如 DeepSeek balance 已有、费用接口调研）；通用适配器 tokens/cost source 已支持映射（P2 已验证）
  - 可低成本接入的官方用量接口补采集（走既有 agent 模式，产出调研结论记录到文档）
  - 无法获得的（如智谱 Coding Plan 无费用接口）→ 统一模型中字段置空 + `capabilities` 标注 unavailable
- **验收标准**：
  - [x] 产出《token/费用数据来源矩阵》文档（provider × 数据项 × 来源 × 状态）——[token-cost-data-sources.md](token-cost-data-sources.md)：17 个平台逐一盘点（内置 14 + generic mock），标注已接入/官方有接口未接入/不可得三类与接入路径
  - [x] 至少 2 个平台真实输出 tokens 或 cost（含 1 个内置 + 1 个通用适配器 mock）——openrouter（内置：cost today/month + tokens today/month 真实输出）+ generic mock 平台（tokens/cost 经 JSONPath 映射直接输出）
  - [x] 无数据平台输出中该字段为 null 而非 0（避免误读）——统一模型 `CostMetric`/`TokensMetric` 为指针类型（`omitempty`），无数据平台输出 `null`，`go test ./internal/...` generic/web/api_integrations 全绿验证
- **优先级**：P0

#### P3-05 面板 token/费用统计展示

- **需求映射**：FR-2.3 / FR-2.4
- **目标**：Web 面板显示 token 与费用统计
- **范围**：`dashboard.html`/`app.js` 增加统计区块（按平台或汇总视图，开工时按 UI 现有风格定）；数据来自 `/api/metrics`
- **验收标准**：
  - [x] 有数据的平台显示今日/周期 token 与费用（真实数据验证）——dashboard 新增「Token / Cost Summary」区块（`#unified-stats-section`）：每平台一张卡片显示 Cost today/month、Tokens today/month，tokens 缩写格式（k/M/B）、费用按币种格式化（USD `$`/CNY `¥`/其他后缀）；数据由 `fetchUnifiedStats()` 取自 `/api/metrics`，随 refreshAll/自动刷新/首屏加载刷新
  - [x] 无数据平台不显示 0 占位（隐藏或"暂无数据"）——仅渲染 `metrics.cost || metrics.tokens` 非空的平台卡片；无任何平台有数据时整个区块保持 `hidden`，不显示 0 占位
- **优先级**：P0

### W3 历史趋势 90 天（FR-7.1）

#### P3-06 history API 扩展与统一

- **需求映射**：FR-7.1
- **目标**：支持 90 天窗口查询，路由对齐架构契约
- **范围**：
  - `parseTimeRange` 增加 `"90d"`（保留既有 1h~30d 兼容）
  - 新增 `GET /api/history/{platform}`（统一出口，内部按 provider 分发到既有 range 查询 + downsample；`/api/history` 旧路由保持兼容）
  - downsample 策略复核：90 天窗口下取点密度（复用 `downsampleStep`，保证 maxChartPoints 上限）
- **验收标准**：
  - [x] `?range=90d` 返回 90 天数据且点数 ≤ 上限（构造跨 90 天测试数据验证，unified_history 测试覆盖）
  - [x] 旧 `/api/history?provider=x` 行为不变（兼容性单测；新增统一路由 `GET /api/history/{platform}`）
  - [x] 全部 provider 的 90d 查询无 SQL 性能问题（`captured_at` 索引 + downsample + limit 双保险；统一路由复用既有 range 查询路径）
- **优先级**：P0

#### P3-07 快照保留策略（90 天趋势保障 + 容量控制）

- **需求映射**：FR-7.1、NFR-2（低资源）
- **目标**：确保 90 天可回溯，同时 SQLite 不无限膨胀
- **范围**：
  - 新增统一 prune：`DELETE FROM {provider}_snapshots WHERE captured_at < ?`，默认保留 **100 天**（90d 需求 + buffer），env `ONWATCH_SNAPSHOT_RETENTION` 可覆盖，0 = 禁用（维持现状）
  - 执行时机：复用 api_integrations 每小时 prune job 模式（`agent/api_integrations_ingest_agent.go:259-276`），独立轻量 goroutine
  - reset_cycles 表暂不裁剪（行数量级小）
  - 文档记录容量估算（按现有轮询频率估算 90 天行数与库体积）
- **验收标准**：
  - [x] 注入过期测试数据后 prune 执行正确删除、保留期内数据不动（store 单测：16 表清单化覆盖 + ark 子表联动 + 边界保留）
  - [x] `ONWATCH_SNAPSHOT_RETENTION=0` 时行为与现状一致（agent 单测：仅日志 disabled，不执行删除）
  - [x] 全量 provider 快照表覆盖（清单化：`snapshotPruneTables` 16 张 `*_snapshots` 表逐一注入校验，漏表视为未完成）
- **优先级**：P1

#### P3-08 面板 90 天趋势图表

- **需求映射**：FR-7.1
- **目标**：面板可查看历史用量趋势
- **范围**：面板历史视图增加 90d 档位（复用现有图表组件与 range 切换 UI）；数据走 `/api/history/{platform}?range=90d`
- **验收标准**：
  - [ ] 面板选择 90 天后图表渲染正常（用真实积累数据或注入数据验证）
  - [ ] 切换 range 不卡顿（downsample 生效）
- **优先级**：P0

### W4 重置倒计时（FR-7.2）

#### P3-09 重置倒计时统一输出与面板展示

- **需求映射**：FR-7.2、FR-2.1（reset_at 字段）
- **目标**：每个配额窗口的重置时间统一可读
- **范围**：
  - 统一模型 `reset_at` 全 provider 对齐（内置快照 `ResetsAt`、generic `reset_at`、无重置概念的余额/费用为 null）
  - 面板配额卡片倒计时组件统一（已有 provider 如 Cursor/Ark 有各自倒计时实现，抽公共渲染：`xh ym zs` / `xd` 粒度）
- **验收标准**：
  - [x] `/api/metrics` 输出中所有 quota 窗口带 `reset_at`（或明确 null）——后端由第一批覆盖：`QuotaMetric.ResetAt` 无 omitempty 恒输出，无重置概念输出空串/null，前端按空值隐藏
  - [x] 面板各平台卡片显示倒计时且随时间递减（抽公共 helper `applyResetCountdown`：data-reset-at 时间戳客户端重算模式，grok/kimi/ark 三家重构接入；主 provider 卡片沿用服务端秒数递减）
  - [x] 无重置时间窗口不显示倒计时区域（resetsAt 为空时隐藏倒计时元素并清理 State 注册）
- **优先级**：P0

### W5 展示端配置（FR-6.1）

#### P3-10 展示端配置模型与 CRUD

- **需求映射**：FR-6.1
- **目标**：每个展示端（web 面板/浮窗/墨水屏）一份配置：显示哪些平台、哪些指标、刷新频率
- **范围**（字段语义约定见 [decisions.md](decisions.md) ADR-016）：
  - 配置模型：`{id, name, platforms[], metrics[], quota_windows{}, display_mode, refresh_seconds}`，存 settings 表新 key `display_configs`（JSON，与 `generic_platforms` 同模式）
  - **字段语义与 onWatch 前端消费字段对齐**：
    - `platforms[]`：provider key 枚举（与 `/api/current` 输出 key 一致，含 generic 平台）
    - `metrics[]`：统一模型四类 `quota/balance/cost/tokens`
    - `quota_windows{}`：按平台的配额窗口勾选（对齐 menubar `SelectedQuotas` 的 per-window 选择语义）
    - `display_mode`：`usage`/`available`（复用既有语义）
    - `refresh_seconds`：刷新频率
  - API：`GET/POST/PUT/DELETE /api/config/displays`（鉴权一致、校验、默认配置 `web-dashboard`）
- **验收标准**：
  - [ ] CRUD 全部可用且持久化、重启恢复（复用 P2-06 持久化验证手法）
  - [ ] 非法配置（空 platforms/refresh 越界）返回 400
  - [ ] 删除默认配置有保护或可重建
  - [ ] 配置可选项与前端消费字段一一对应（无悬空字段）
- **优先级**：P0

#### P3-11 统一 API 按展示配置过滤 + 配置页 UI

- **需求映射**：FR-6.1、NFR-4
- **目标**：展示端凭配置取数：`GET /api/metrics?display={id}` 只返回配置的平台/指标；配置页可视化管理
- **范围**：
  - `/api/metrics` 支持 `display` 参数过滤（平台集合 + 指标裁剪 + 窗口勾选过滤）
  - 配置页 UI **可新做**（不强制复用现有设置组件，ADR-016 仅约束字段语义），按展示端管理场景设计：列表/新增/编辑平台与指标勾选/刷新频率
  - **首个消费方为 Web 面板自举**：默认展示端 `web-dashboard` 由主面板消费，随后浮窗（Phase 4）/墨水屏（Phase 5）接入
  - WS 推送同样按订阅时声明的 display 过滤（P3-03 联动）
- **验收标准**：
  - [ ] `?display=x` 输出与配置一致（平台过滤 + 指标字段裁剪验证）
  - [ ] 主面板按 `web-dashboard` 配置过滤显示（自举端到端验证）
  - [ ] 配置页增删改展示端并即时生效（端到端）
  - [ ] 新增展示端零改动聚合服务（NFR-4 演示：配置一份 epaper 预留配置，API 可取数）
- **优先级**：P0

### W6 测试与收尾

#### P3-12 测试补齐 + 文档 + 验收更新

- **需求映射**：Phase 3 全部验收项
- **目标**：质量保障与文档沉淀
- **范围**：
  - 转换层/API/WS/prune/展示配置单测补齐（目标：新代码覆盖率 ≥ 70%，对齐 P2-03 口径）
  - 文档：统一指标 API 使用指南（`docs/unified-metrics-api.md`：端点/契约/WS 订阅/展示配置示例）；更新 architecture.md 偏差（如有）
  - roadmap.md Phase 3 验收项核验勾选、进度追踪表更新；实施中新决策记录 ADR（统一 API 鉴权口径、retention 默认值、WS 消息协议等）
- **验收标准**：
  - [ ] `go test ./...` 全绿（既有 Windows 例外项单独标注）
  - [ ] 新增代码覆盖率 ≥ 70%
  - [ ] API 指南包含 1 个完整可复现的展示端接入示例（与 P3-11 一致）
  - [ ] roadmap.md Phase 3 验收项全部勾选或标注原因
- **优先级**：P1

## 4. 依赖关系与执行顺序

```
W1 转换层（P3-01）
   ├─▶ W1 REST API（P3-02）            ← 依赖统一模型
   │     ├─▶ W2 token/费用（P3-04, P3-05）   ← 在统一输出上补齐字段
   │     ├─▶ W3 历史（P3-06, P3-08）          ← 路由/图表扩展
   │     ├─▶ W4 倒计时（P3-09）               ← reset_at 对齐 + 面板
   │     └─▶ W5 展示配置（P3-10 → P3-11）     ← 过滤统一输出
   ├─▶ W1 WS（P3-03）                  ← 依赖统一模型与广播时机
   └─▶ W3 prune（P3-07）               ← 独立，可任意时机并行
        └─▶ W6 收尾（P3-12）            ← 最后
```

建议的落地批次：

| 批次 | 任务 | 说明 |
|---|---|---|
| 第一批 | P3-01, P3-02 | 转换层 + REST 骨架（本期价值核心） |
| 第二批 | P3-06, P3-07, P3-09 | 历史 90d + 保留策略 + 倒计时（可并行） |
| 第三批 | P3-04, P3-05 | token/费用（含官方接口调研） |
| 第四批 | P3-10, P3-11, P3-03 | 展示配置 + WS 推送 |
| 第五批 | P3-12 | 测试、文档、验收收尾 |

## 5. 需求 → 任务验收矩阵

| 需求 | 验收要点 | 对应任务 |
|---|---|---|
| FR-2.1 配额剩余 | 统一 API 输出窗口/已用/总量/百分比/重置时间 | P3-01, P3-02, P3-09 |
| FR-2.2 余额 | 金额/币种统一输出 | P3-01, P3-02 |
| FR-2.3 token 消耗 | 今日/周期统计显示 | P3-04, P3-05 |
| FR-2.4 费用 | 今日/周期/币种统计显示 | P3-04, P3-05 |
| FR-6.1 展示端配置 | 配置页管理多个展示端 | P3-10, P3-11 |
| FR-7.1 90 天趋势 | 面板历史图表 + 数据保障 | P3-06, P3-07, P3-08 |
| FR-7.2 重置倒计时 | 各窗口重置时间显示 | P3-09 |
| NFR-4 可扩展 | 通用平台自动进统一 API；新增展示端零改动 | P3-02, P3-11 |

## 6. 风险与开放问题

| 风险/问题 | 影响 | 应对 |
|---|---|---|
| token/费用数据各平台可得性差异大（部分平台无公开用量接口） | FR-2.3/2.4 无法全覆盖 | P3-04 先出数据来源矩阵，明确"可采/不可采"边界；不可采输出 null + capabilities 标注，验收口径按矩阵 |
| 统一转换层覆盖 15+ provider，工作量大且各快照结构异构 | P3-01 工期风险 | 注册表模式逐个转换、逐个单测；第一批先覆盖 P0 四家（Ark/智谱/DeepSeek/OpenCode）+ 通用适配器，其余分批补齐 |
| WS 鉴权（浏览器 WS 不能带自定义 header） | 安全口径 | token 走 query 参数 + 一次性短期票据（实施时定，记 ADR） |
| 90 天查询在无索引大表上的性能 | 查询变慢 | 确认 `captured_at` 索引存在（迁移时已建）；downsample + limit 双保险；P3-06 验收含性能验证 |
| prune 误删（时区/时钟回拨） | 数据丢失 | cutoff 用 UTC 比对 + 保留 buffer 100 天 > 90 天需求；retention=0 可禁用 |
| 展示配置与既有 `display_mode`/menubar 选择概念混淆 | 用户困惑 | 已解决（ADR-016）：`display_mode` 直接复用既有语义，窗口勾选对齐 `SelectedQuotas`，配置字段即前端消费字段，无新概念 |
| 既有 web 包 1 例 Windows 测试失败（Q8） | 全绿口径 | 延续 P2 口径：单独标注，不阻塞 |
