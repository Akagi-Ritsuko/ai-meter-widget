# onWatch 项目工作进度报告

## 1. 项目基本信息

| 项目 | 内容 |
|---|---|
| 项目名称 | onWatch（AI 配额跟踪守护进程） |
| 模块标识 | `github.com/onllm-dev/onwatch/v2`（Go 1.25.7） |
| 发行版本 | v2.13.5 |
| 本期目标 | FR-1.1 平台内置适配器（火山方舟 Ark）P0 编码实现 |
| 负责人 | 待确认（当前执行人：AI 编码助手 + 开发者评审） |
| 报告日期 | 2026-08-27 |
| 关联需求 | [requirements-fr-1.1-p0-adapters.md](requirements-fr-1.1-p0-adapters.md) |
| 实施计划 | [fr-1.1-adapters-implementation-plan.md](../.trae/documents/fr-1.1-adapters-implementation-plan.md) |

---

## 2. 已完成工作内容

### 2.1 前置阶段（Phase 1 / Phase 2，已完成）

| 里程碑 | 成果 | 日期 |
|---|---|---|
| Phase 1 运行 | 在新机器完成项目运行环境搭建与启动（Go 位于本机 `go/bin/go.exe`，非 PATH） | 2026-08-27 |
| Phase 2 需求拆分 | 全部需求项拆分为可执行子任务，含目标/范围/验收标准/优先级，结构化文档落地 | 2026-08-27 |
| FR-1.1 需求文档 | 含功能总览、功能需求、技术规格、验证标准、交付物、依赖约束、验收标准 7 部分 | 2026-08-27 |
| 编码实施计划 | 9 阶段任务（t1–t9）写入 [fr-1.1-adapters-implementation-plan.md](../.trae/documents/fr-1.1-adapters-implementation-plan.md) 并经评审批准 | 2026-08-27 |

### 2.2 Ark 适配器编码实施（t1–t5 主体完成）

| # | 模块层 | 主要成果 | 测试 | 状态 |
|---|---|---|---|---|
| t1 | `internal/api` | 新建 [ark_types.go](../server/internal/api/ark_types.go)（`json.Number` 容错解析五窗口、`ToSnapshot` 归一化）、[ark_client.go](../server/internal/api/ark_client.go)（HMAC-SHA256 V4 签名全流程、错误集、AK 日志脱敏、1 MiB 限读、30s 超时） | [ark_types_test.go](../server/internal/api/ark_types_test.go) + [ark_client_test.go](../server/internal/api/ark_client_test.go) 共 16 用例，含 V4 签名 golden 校验 | ✅ 全绿 |
| t2 | `internal/store` | [store.go](../server/internal/store/store.go) `migrateSchema()` 追加 ark 三表 + 四索引；新建 [ark_store.go](../server/internal/store/ark_store.go)（快照双表、`ark_reset_cycles`、周期查询/概览/序列）；修复三处 `ORDER BY quota_name` → `ORDER BY id`（保持 five_hour→daily→weekly→monthly 展示顺序） | [ark_store_test.go](../server/internal/store/ark_store_test.go) 4 用例 | ✅ 全绿 |
| t3 | `internal/tracker` | 新建 [ark_tracker.go](../server/internal/tracker/ark_tracker.go)（镜像 opencode 重置检测：90min 容差 + ResetsAt 变化检测 + 漂移变量互斥）+ `UsageSummary` | [ark_tracker_test.go](../server/internal/tracker/ark_tracker_test.go) 6 用例 | ✅ 全绿 |
| t4 | `internal/agent` | 新建 [ark_agent.go](../server/internal/agent/ark_agent.go)（`poll` 含 nil client 保护、pollingCheck 短路、`SetNotifier`/`SetPollingCheck` 链、结构化日志） | [ark_agent_test.go](../server/internal/agent/ark_agent_test.go) 5 用例 | ✅ 全绿 |
| t5 | `internal/web`（接线） | 新建 [ark_handlers.go](../server/internal/web/ark_handlers.go)（8 个 handler：current/history/cycles/cycleOverview/summary/insights/loggingHistory + 类型 + helper）；[handlers.go](../server/internal/web/handlers.go) 完成 9 类改动（`arkTracker` 字段、providerCatalog 注册、`isProviderConfigured`、`providerEnumFields`、`provSettings["ark"]`、七处 switch 分发、三处聚合点）；[config.go](../server/internal/config/config.go) 提前完成 Ark 字段/env 读取/`HasProvider`/`String()` 脱敏；[menubar.go](../server/internal/web/menubar.go) ark 卡片、[dashboard_tabs.go](../server/internal/web/dashboard_tabs.go) `case "ark"`、[static/app.js](../server/internal/web/static/app.js) `usePercent` 列表加 `ark` | `go build ./internal/web/` 编译通过 | ⏳ 编译通过，测试待全绿 |

### 2.3 过程中解决的关键问题

| 问题 | 解决方案 |
|---|---|
| `ArkSnapshot` 缺 `ID` 字段导致 store 编译失败 | [ark_types.go](../server/internal/api/ark_types.go) 补 `ID int64` |
| `QueryLatestArk` 窗口顺序错误（字母序 daily 排前） | [ark_store.go](../server/internal/store/ark_store.go) 三处 `ORDER BY quota_name` → `ORDER BY id` |
| 官方文档与 API Explorer 返回 Quota/Used 类型不一致（string vs number） | [ark_types.go](../server/internal/api/ark_types.go) 全链路 `json.Number` 容错解析 |
| api 包既有 Windows 编译失败（`getCredentialsFilePath` 仅 `!windows` 定义，[extra_coverage_test.go](../server/internal/api/extra_coverage_test.go) 引用） | 临时移开测试文件跑测试后移回（既有环境约束，非本任务引入） |
| Ark 窗口有固定展示顺序而 opencode 模板用字母序 | 决策采用 `ORDER BY id` 保持插入顺序 |

---

## 3. 进行中工作状态

| 维度 | 状态 |
|---|---|
| 总体进度 | **约 55%**（t1–t4 完成、t5 代码完成待测试全绿、t6 config 部分提前完成） |
| 当前阶段 | t5 Ark web 层（接线收尾） |
| 已完成成果 | 五层代码全落地；新增测试 **31 个**（api 16 / store 4 / tracker 6 / agent 5）全部通过；`internal/web` 包编译通过 |
| 已投入资源 | 本机 Go 工具链（`go/bin/go.exe`）、项目既有测试基础设施（`t.Context()`、`slog.HandlerOptions{Level: slog.LevelDebug}`、mock HTTP server） |
| 预计完成时间 | 剩余 t6→t7→t8→t9 四阶段（接线 → 构建/全量测试 → 文档交付物 → 冒烟验收），完成后即达 FR-1.1 验收标准；具体日期待 t6 接线推进后评估 |
| 面临的挑战 | ① 真实验证不可行——沙箱无火山 AK/SK（决策 D7，mock 全覆盖替代）；② Windows 跨平台既有测试问题影响 t7 全量测试通过率；③ 前端 `app.js` 改动面大，需回归面板渲染 |

---

## 4. 待完成问题清单

| 优先级 | # | 问题描述 | 影响范围 | 建议解决方案 | 责任人 | 计划解决 |
|---|---|---|---|---|---|---|
| P0 | Q1 | main.go 未接线：Ark client/tracker/agent 的初始化、`SetNotifier`/`SetPollingCheck` 链、`SetArkTracker`、`RegisterFactory("ark")`、启动列表（计划 §H 共 8 处） | Ark 面板无数据来源，agent 不轮询 | 复刻 deepseek/opencode 全套位点接线 | 执行人 | t6 |
| P0 | Q2 | `server/.env.example` 未追加 `ARK_ACCESS_KEY` / `ARK_SECRET_KEY` / `ARK_REGION=cn-beijing` / `ARK_BASE_URL` | 用户无法按示例配置凭证 | 追加变量行 + 获取位置与安全注释 | 执行人 | t6 |
| P0 | Q3 | 构建与全量测试未执行（`go build -o onwatch.exe .` + `go test ./...`） | 无法确认全仓无回归 | 构建；`extra_coverage_test.go` 移开手法跑测试并记录既有失败 | 执行人 | t7 |
| P1 | Q4 | 文档交付物缺失：`ark-interface-research.md`（D5）、`credential-guide.md`（D3）、`verification-fr-1.1.md`（D2）、`test-report-fr-1.1.md`（D4） | 验收/交接缺依据 | 计划 §3.4 逐项落地 | 执行人 | t8 |
| P1 | Q5 | 需求文档回写未做：ARK-02 路由措辞（统一分发）、ARK-03 凭证 env、§3.2 表格端点/鉴权字段；phase2-plan/roadmap 勾选 | 文档与实现不一致 | 按计划 §3.4 回写 | 执行人 | t8 |
| P1 | Q6 | 启动冒烟验收未做（E2E-1：无凭证 `unconfigured` + 伪造凭证 `error` 路径） | 运行态行为未验证 | t9 按验证步骤 3/4 执行 | 执行人 | t9 |
| P2 | Q7 | 真实验证待用户凭证：TC-01~05 需有效火山 AK/SK | 线上接口行为未实跑 | 交付后用户提供凭证，按 D2 文档步骤执行，不作为跳过失败 | 用户 | 凭证提供后 |
| P2 | Q8 | web 包既有测试失败：`TestHandlerTryAutoDetectAdditionalCoverage/anthropic_success_from_credentials_file`（Windows 下 `os.UserHomeDir()` 不受 `HOME` env 影响） | 全量测试非全绿（非本任务引入） | 记录为既有跨平台问题；后续可改用 `USERPROFILE` env 或平台分支断言 | 执行人 | 跟进（不阻塞本需求） |

---

## 5. 风险评估与应对措施

| 风险 | 等级 | 影响 | 应对措施 |
|---|---|---|---|
| 无真实 AK/SK，线上 GetAFPUsage 行为未验证 | 中 | 接口字段/schema 假设可能存在偏差 | 已用 `json.Number` 覆盖 string/number 双版本返回；mock 单测 31 例全覆盖；文档标注 D7 待用户凭证补验 |
| Windows 跨平台既有测试问题（`getCredentialsFilePath`、Anthropic credentials 检测） | 低 | t7 `go test ./...` 非全绿，可能误判为本需求回归 | 已确认 Q8 与 Ark 无关（改前即失败）；t7 用隔离文件手法跑全量并单独记录既有失败清单 |
| 前端 `app.js` 修改波及渲染逻辑 | 低 | 面板/周期表渲染回归 | 改动仅 1 行（`usePercent` 列表加 `ark`）；t9 冒烟验收覆盖 dashboard 渲染 |
| schema 迁移破坏既有库 | 低 | 升级后数据异常 | `CREATE TABLE IF NOT EXISTS` 幂等追加（镜像 opencode 表模式），含索引；t2 store 测试覆盖 |
| 计划时间跨度内 API 变更（官方 Quota 类型 string/number 不一致） | 低 | 解析失败丢窗口 | `json.Number` 兜底 + `ToSnapshot` 零值容错（缺失窗口保留零配额） |
| 多轮会话上下文压缩导致步骤丢失 | 低 | 中途断点 | 采用 TodoWrite 九任务清单 + 本文档固化进度，可随时续接 |

---

> 备注：本项目状态由本次会话工具调用记录整理，进度百分比为按 TodoWrite 九项任务（t1–t9）完成度的估算口径，非精确度量。