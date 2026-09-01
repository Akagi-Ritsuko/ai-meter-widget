# 项目工作进度报告

> 版本：v7 | 报告日期：2026-09-01
> 关联：[roadmap.md](roadmap.md) | [decisions.md](decisions.md) | [phase2-plan.md](phase2-plan.md) | [project-tracking.md](project-tracking.md)

## 1. 项目基本信息

| 项目 | 内容 |
|---|---|
| 项目名称 | ai-meter-widget（基于 onWatch v2.13.5 fork） |
| 模块标识 | `github.com/onllm-dev/onwatch/v2`（Go 1.26.7） |
| 当前里程碑 | Phase 3 统一指标 API（第一、二、三批完成） |
| 报告日期 | 2026-09-01 |
| 决策记录 | ADR-001 ~ ADR-015（[decisions.md](decisions.md)） |

## 2. 里程碑总览

| Phase | 状态 | 完成度 | 剩余事项 |
|---|---|---|---|
| 1 底座跑通 | ✅ 核心完成 | 90% | Claude/Codex 凭证验证、24h soak、交叉编译 |
| 2 通用适配器 + P0 | 🔄 代码完成 | 98% | DeepSeek/OpenCode 真实验证（待用户凭证）；智谱已全部验证通过 |
| 3 统一指标 API | 🔄 进行中 | 65% | 第一、二、三批（P3-01/02/04/05/06/07/09）已完成（2026-09-01）；剩余 P3-03、P3-08、P3-10~12（WS、面板图表、展示配置、收尾） |
| 4 桌面浮窗 + 告警 | 未开始 | 0% | - |
| 5 ESP32 墨水屏 | 未开始 | 0% | - |

## 3. 本期完成工作（2026-08-28 ~ 08-31）

### 3.1 火山引擎 Ark 适配器（P2-09，FR-1.1 核心）

**Agent Plan（AK/SK 鉴权）**：五层完整实现（api/store/tracker/agent/web），HMAC-SHA256 V4 签名，34 例单测全绿。

**Coding Plan（Cookie 鉴权）**：`GetCodingPlanUsage` 控制台内部接口客户端，与 Agent Plan 双套餐轮询，**真实接口验证通过**（session 0% / weekly 8.61% / monthly 4.30%）。

**Cookie 自动刷新**（ADR-008/009/010）：
- JWT 过期检测（digest 2 天 / userInfo 30 天，<3 天预警）
- CDP 提取器（浏览器运行时）+ 浏览器 DB 提取器（关闭时）+ Composite 组合
- **端到端验证通过**：无效 Cookie → CDP 提取 35 个 Cookie → 自动刷新 → 采集成功

**面板集成**（ADR-011/012）：
- 前端三件套（容器/渲染/更新）+ fetchCurrent 分支 + ark.svg 图标
- Provider 设置面板（⚙️：Coding Plan Cookie/WebID、Agent Plan AK/SK、Region）
- 占位 Cookie 触发启用模式（`pending-cdp-refresh`，ADR-013）

### 3.2 通用适配器引擎激活（P2-01~08，FR-1.3/3/4）

| 任务 | 成果 |
|---|---|
| P2-01 接线 | server.go 路由挂载（7 条）+ main.go Agent/Handler 注册 + 配置页入口 |
| P2-02 认证验证 | 4 种认证（api_key/bearer/cookie/oauth_local）单测覆盖；**修复 Windows 路径冒号 bug** |
| P2-03 测试 | generic 包 24 例，覆盖率 **72%**（目标 ≥70%） |
| P2-04 端到端 | mock 平台零代码接入全流程验证（配置→测试→保存→轮询→数据） |
| P2-05~08 配置页 | 表单/映射/测试连接联调，持久化重启恢复验证 |

### 3.3 安全与文档（P2-13~16）

- P2-13/14：凭证 `env:VAR` 引用提示、列表 API 脱敏（maskCredential）、日志无凭证
- P2-15：[generic-adapter-guide.md](generic-adapter-guide.md) 零代码接入指南
- P2-16：验收清单更新（[phase2-plan.md](phase2-plan.md)）

### 3.4 智谱 Coding Plan 真实接口验证 + CREDIT_LIMIT 解析修复（2026-09-01）

- **真实接口验证**：配置 `ZAI_REGION=cn` 后直接请求 `https://open.bigmodel.cn/api/monitor/usage/quota/limit`，返回 HTTP 200、`success: true`，数据完整（Lite 套餐：5 小时窗口 0/2000、周窗口 2005/2000 已超额）。
- **发现根因**：国内版返回 `CREDIT_LIMIT` 类型 limit，而 `ToSnapshot` 仅处理国际版（z.ai）的 `TIME_LIMIT`/`TOKENS_LIMIT`，导致面板用量全 0 且日志无错误。
- **修复**：`zai_types.go` 新增 `CREDIT_LIMIT` 分支（`number=5` → Time 字段；`number=1` → Tokens 字段保留重置倒计时）+ 真实样本单测（ADR-015）。
- **测试**：`go test ./internal/api/ -run "TestZai"` 全绿，无回归。
- **面板验证**：重启 onwatch 后智谱面板配额显示正常（2026-09-01），P2-11 全部完成。

### 3.5 Phase 3 启动：第一批完成（2026-09-01）

**P3-01 统一指标转换层**：
- 新建 `internal/metrics/unified.go`：Ark/Zai/DeepSeek/OpenCode 四家快照 → `generic.PlatformSnapshot` 统一模型（剩余 12 家后续批次补齐，ADR 后续编号）
- 转换规则：窗口范式（used-based / remaining-based / percent-only）、Percent 兜底、RFC3339 UTC 时间、空值不输出假 0
- 单测 18 例全绿（`internal/metrics` + `web` + `generic` + `store` 无回归）

**P3-02 统一指标 REST API**：
- 新建 `internal/web/unified_handlers.go`：`collectUnifiedSnapshots()` 组装（内置 4 家 + generic 直通），可见性/配置门控与 currentBoth 一致
- `server.go` 挂载 3 条路由：`GET /api/platforms`、`GET /api/metrics`、`GET /api/metrics/{platform}`
- 验收：curl 冒烟通过（隔离实例 + BasicAuth）；`/api/metrics/{platform}` 未知平台 404；单平台 + 通用平台端到端共存（zai + gen-smoke）；handler 单测 7 用例（200/404/405/门控过滤/awaiting first poll）

实施计划与决策记录：[phase3-batch1-unified-metrics-plan.md](../.trae/documents/phase3-batch1-unified-metrics-plan.md)（D1~D9）。

### 3.6 Phase 3 第二批完成（2026-09-01）

**P3-06 history API 扩展与统一**：
- `parseTimeRange` 增加 `"90d"` 档位（既有 1h~30d 兼容不变）
- 新增统一路由 `GET /api/history/{platform}`（`unified_handlers.go`：`UnifiedHistory` + `parseUnifiedHistoryPlatform`，兼容 basePath 部署；`/api/history` 旧路由保持兼容）
- downsample 复用 `downsampleStep`，90d 窗口点数 ≤ maxChartPoints 上限；统一单测 9 用例（unified_history_test.go）

**P3-07 快照保留策略**：
- `store/snapshot_retention.go`：统一 prune 16 张 `*_snapshots` 表（`snapshotPruneTables` 清单化覆盖，含 ark 子表联动），UTC cutoff + RFC3339Nano 比对
- `agent/snapshot_retention_agent.go`：独立 goroutine（复用 AgentManager 模式），每小时 ticker；默认保留 100 天，env `ONWATCH_SNAPSHOT_RETENTION` 可覆盖，0 = 禁用
- 测试：store 3 例（清单化覆盖校验/删除正确性/边界保留）+ agent 2 例（prune 生效/禁用行为）

**P3-09 重置倒计时**：
- 后端：`QuotaMetric.ResetAt` 无 omitempty 恒输出（第一批已覆盖，空值显式输出，前端按空值隐藏）
- 前端：抽公共 helper `applyResetCountdown`/`countdownSecondsFrom`（data-reset-at 时间戳客户端重算模式，免递减累计漂移）；`startCountdowns` 双分支（data-reset-at 重算优先，legacy 递减回退）
- 重构 grok/kimi/ark 三家卡片接入（原重复实现删除）；无重置时间窗口隐藏倒计时区域并清理 State 注册；主 provider 卡片（服务端秒数递减）行为不变

**回归**：`go build ./...` 通过；核心包（store/metrics/generic/api_integrations/menubar/notify/testutil/tracker）全绿；既有平台/环境性失败不变（详见 project-tracking.md §1.1）。

### 3.7 Phase 3 第三批完成（2026-09-01）

**P3-04 token/费用数据可得性评估与缺口补齐**：
- 产出 [token-cost-data-sources.md](token-cost-data-sources.md)：17 个平台（内置 14 + generic mock）逐一盘点 tokens/cost 数据来源（已接入/官方有接口未接入/不可得）与接入路径
- **openrouter 内置接入**（P3-04 唯一代码改动）：`unified_handlers.go` openrouter 转换分支补充 cost（today/month）与 tokens（today/month）真实输出，快照结构已有 `DailyCost`/`TokensUsed` 字段，零新采集
- **generic 平台天然支持**：JSONPath 映射直接产出 cost/tokens（P2 已验证），mock 平台真实输出
- **null 语义**：统一模型 `CostMetric`/`TokensMetric` 指针类型（`omitempty`），无数据平台输出 `null` 而非 0

**P3-05 面板 token/费用统计展示**：
- `dashboard.html`：新增「Token / Cost Summary」区块（`#unified-stats-section`，insights-panel 风格容器，初始 `hidden`）
- `app.js`：新增 `fetchUnifiedStats()`（取 `/api/metrics`）+ `renderUnifiedStats()`（仅渲染有 cost/tokens 数据的平台卡片）+ `formatUnifiedTokens()`（k/M/B 缩写）/`formatUnifiedCost()`（USD `$`/CNY `¥`/其他币种后缀）；接入 refreshAll/自动刷新/首屏加载三处刷新链路
- `style.css`：新增 `.unified-stats-grid`/`.unified-stat-card` 等卡片样式（对齐 insights-panel 视觉体系）
- **无 0 占位**：无数据平台不渲染卡片；全部无数据时整区保持隐藏

**收尾与回归**：
- 修复 internal/api Windows 测试构建失败：`extra_coverage_test.go` 从 TEMP 恢复后引用 unix-only 的 `getCredentialsFilePath` 导致 4 处 `undefined`；将 4 个 `TestGetCredentialsFilePath_*` 测试迁移至新建 `extra_coverage_unix_test.go`（`//go:build !windows`）
- 回归：`node --check`（JS 语法）✅；`go build ./...` ✅；web dashboard 模板测试 21 例全绿；generic/api_integrations/testutil 全绿；既有环境性失败不变（agent/config/service/update/mockserver），api 包新增 19 例 + web 1 例环境性失败（HOME/USERPROFILE 语义 + 沙箱写拦截，系恢复文件首次在 Windows 编译暴露，详见 project-tracking.md §1.1）

## 4. 问题处理记录

| 问题 | 根因 | 对策 | 结果 |
|---|---|---|---|
| Ark 请求发到无主机地址 | `WithArkBaseURL("")` 清空默认值 | 空值不覆盖（ADR-007） | ✅ 修复 |
| `file:path:jsonpath` Windows 解析失败 | `SplitN(":")` 被 `C:\` 盘符干扰 | 从右侧取最后冒号分隔 | ✅ 修复 + 单测 |
| CDP 提取缺 digest/userInfo/csrfToken | 关键 Cookie 存父域 `.volcengine.com`，过滤只匹配 console 子域 | 过滤改匹配 `volcengine.com`（ADR-010） | ✅ 修复 + 端到端验证 |
| CDP 修复后 onwatch 仍失败 | 旧二进制未含修复 | 重新构建 | ✅ 解决 |
| 面板 Ark 标签页无数据、last-updated 不更新 | 前端缺容器/渲染/fetch 分支 | 补全三件套 + 图标（ADR-011） | ✅ 修复 |
| Provider Controls 无 Ark 设置入口 | `providerSettingsConfig` 未注册 | 前后端补设置（ADR-012） | ✅ 修复 |
| start-edge-debug.bat 闪退/不生效 | ①UTF-8 中文被 cmd 按 GBK 解析 ②Edge 启动加速接管 ③Edge 路径带空格 | 纯 ASCII + 独立配置目录 + 注册表定位 | ✅ 修复 |
| Insights 卡片样式与配额卡不一致 | 独立样式定义 | 基础样式对齐 .quota-card（ADR-014） | ✅ 修复 |
| 智谱面板用量全 0（日志无错误） | 国内版返回 `CREDIT_LIMIT` 类型，解析器只认 `TIME_LIMIT`/`TOKENS_LIMIT` | `ToSnapshot` 新增 `CREDIT_LIMIT` 分支（ADR-015） | ✅ 修复 + 单测；面板验证通过（2026-09-01） |

完整明细：[ark-interface-research.md](ark-interface-research.md) §9。

## 5. 测试与验证状态

| 套件 | 结果 |
|---|---|
| Ark 适配器（api/store/tracker/agent） | ✅ 34 例全绿 |
| generic 包 | ✅ 24 例全绿，覆盖率 72% |
| store / tracker / config / agent | ✅ 全绿 |
| web 包 | ⚠️ 1 例既有失败（Windows Q8，非本项目引入） |
| api 包全量 | ⚠️ `extra_coverage_test.go` Windows 构建失败（既有，临时移开跑） |
| 真实接口验证 | ✅ Ark Coding Plan；✅ 智谱（2026-09-01，CREDIT_LIMIT 已修复 + 面板验证通过）；⏳ DeepSeek/OpenCode 待凭证 |
| Phase 3 第一批（metrics 转换层 + web 统一 handler） | ✅ 25 例全绿（18 转换 + 7 handler），全量回归无新增失败 |
| Phase 3 第二批（unified history + snapshot retention） | ✅ web 统一历史 9 例 + store prune 3 例 + agent retention 2 例全绿；核心包全量回归无新增失败（既有平台/环境性例外不变） |
| Phase 3 第三批（token/费用矩阵 + 面板统计展示） | ✅ generic/web/api_integrations 全绿；web dashboard 模板 21 例全绿；JS 语法校验通过；api 包恢复文件暴露 19 例环境性失败（非本批引入，已单独标注） |

## 6. 待办清单

| 优先级 | 项 | 责任 | 说明 |
|---|---|---|---|
| P1 | DeepSeek 真实验证（P2-10） | 用户提供 `DEEPSEEK_API_KEY` | 配置后面板验证 |
| P1 | OpenCode 真实验证（P2-12） | 用户提供 `OPENCODE_GO_*` | 同上 |
| P1 | Phase 3 第四批：展示配置 + WS（P3-10、P3-11、P3-03） | 执行人 | 第一、二、三批已完成（2026-09-01） |
| P2 | Phase 1 收尾（24h soak、交叉编译、Claude/Codex 验证） | 执行人 | 不阻塞 Phase 3 |
| P2 | CDP 无头模式探索 | 执行人 | 减少"调试 Edge 需运行"约束 |

## 7. 变更文件清单（2026-08-28 ~ 08-31）

**后端（Go）**：
- `internal/api/`：ark_types/ark_client/ark_codingplan_client/ark_cookie_expiry/ark_cookie_windows/ark_cookie_other/ark_cookie_composite（+测试）
- `internal/generic/`：config（Windows 路径修复）/adapter/metrics/handlers（脱敏）/page（+generic_test.go 24 例）
- `internal/store/`：ark_store（+测试）、generic_store、store.go（ark/generic 表）
- `internal/tracker/`、`internal/agent/`：ark_tracker/ark_agent（+测试）
- `internal/web/`：ark_handlers、handlers.go（ark 设置/分发/配置检查）、server.go（generic 路由）、dashboard.html（ark 容器）、settings.html（入口）、static/app.js（ark 渲染+设置）、static/style.css（insight 样式）、static/icons/ark.svg
- `internal/config/config.go`：Ark 全部字段（AK/SK/Region/BaseURL/Cookie/WebID/CSRF/CDP URL）
- `main.go`：ark + generic 接线
- `.env.example`：Ark 全部变量

**脚本与配置**：
- `scripts/start-edge-debug.bat`（新建）
- `~/.onwatch/.env`（用户环境，含 Ark 占位 Cookie）

**文档**：
- 更新：roadmap（v4）、decisions（v3，ADR-006~014）、phase2-plan、ark-interface-research（v3）、credential-guide（v3）、requirements-fr-1.1、README
- 新建：generic-adapter-guide.md、verification-fr-1.1.md、test-report-fr-1.1.md、credential-guide.md、ark-interface-research.md

## 8. 变更文件清单（2026-09-01，智谱 CREDIT_LIMIT 修复）

**后端（Go）**：
- `internal/api/zai_types.go`：`ToSnapshot` 新增 `CREDIT_LIMIT` 分支（`number=5` → Time 字段；`number=1` → Tokens 字段保留重置倒计时）
- `internal/api/zai_types_test.go`：新增 `TestZaiQuotaResponse_ToSnapshot_DomesticCreditLimit`（真实响应样本）

**构建产物**：
- `server/onwatch.exe`：重新构建（含 CREDIT_LIMIT 修复）

**文档**：
- 新建：project-tracking.md（标准化跟踪文档：变更记录/里程碑规划/需求待完成项）
- 更新：decisions（v4，ADR-015）、credential-guide（v4）、verification-fr-1.1（TC-02 智谱 ✅）、test-report-fr-1.1（v2，§7）、project-status（v4）、roadmap（v5）

## 9. 变更文件清单（2026-09-01，Phase 3 第一批）

**后端（Go，新建）**：
- `server/internal/metrics/unified.go`：ConvertArk/ConvertZai/ConvertDeepSeek/ConvertOpenCode + helper（rfc3339/percent 兜底/percent-only/ark 排序）
- `server/internal/metrics/unified_test.go`：18 用例
- `server/internal/web/unified_handlers.go`：Platforms/UnifiedMetrics/UnifiedPlatformMetrics + collectUnifiedSnapshots
- `server/internal/web/unified_handlers_test.go`：7 用例

**后端（Go，修改）**：
- `server/internal/web/server.go`：挂载 `/api/platforms`、`/api/metrics`、`/api/metrics/` 三条路由

**文档**：
- 新建：phase3-plan.md、.trae/documents/phase3-batch1-unified-metrics-plan.md
- 更新：project-status（v5）、project-tracking（v5）

## 10. 变更文件清单（2026-09-01，Phase 3 第二批）

**后端（Go，新建）**：
- `server/internal/store/snapshot_retention.go`：`snapshotPruneTables` 16 表清单 + `PruneSnapshotsOlderThan`（UTC cutoff，逐表删除）
- `server/internal/store/snapshot_retention_test.go`：3 用例（清单覆盖校验/删除正确性含 ark 子表联动/边界保留）
- `server/internal/agent/snapshot_retention_agent.go`：SnapshotRetentionAgent（每小时 ticker，retention ≤ 0 禁用）
- `server/internal/agent/snapshot_retention_agent_test.go`：2 用例（prune 生效/禁用）
- `server/internal/web/unified_history_test.go`：9 用例（90d 档位/点数上限/旧路由兼容/统一路由分发）

**后端（Go，修改）**：
- `server/internal/web/handlers.go`：`parseTimeRange` 增加 `"90d"` 档位
- `server/internal/web/unified_handlers.go`：`UnifiedHistory` 统一历史端点（`GET /api/history/{platform}`，按 provider 分发既有 range 查询）
- `server/internal/web/server.go`：挂载统一历史路由（`/api/history` 旧路由保持）
- `server/internal/config/config.go`：快照保留天数配置（env `ONWATCH_SNAPSHOT_RETENTION`，默认 100）
- `server/main.go`：SnapshotRetentionAgent 注册接线

**前端（JS，修改）**：
- `server/internal/web/static/app.js`：新增 `applyResetCountdown`/`countdownSecondsFrom` 公共 helper；`startCountdowns` 支持 data-reset-at 客户端重算分支；grok/kimi/ark 卡片倒计时重构接入（删除重复实现）

**文档**：
- 更新：phase3-plan.md（第二批验收项勾选）、project-status（v6，本版）、project-tracking（v6）

## 11. 变更文件清单（2026-09-01，Phase 3 第三批）

**后端（Go，修改）**：
- `server/internal/web/unified_handlers.go`：openrouter 统一转换分支补充 cost（today/month）+ tokens（today/month）输出（P3-04）

**后端（Go，新建/修改，测试修复）**：
- `server/internal/api/extra_coverage_unix_test.go`（新建，`//go:build !windows`）：从 extra_coverage_test.go 迁入 4 个 `TestGetCredentialsFilePath_*` 测试（引用 unix-only 函数，修复 Windows 测试构建失败）
- `server/internal/api/extra_coverage_test.go`（修改）：移除上述 4 个测试

**前端（HTML/CSS/JS，修改）**：
- `server/internal/web/templates/dashboard.html`：新增「Token / Cost Summary」统一统计区块（`#unified-stats-section`，初始 hidden）
- `server/internal/web/static/style.css`：新增 `.unified-stats-grid`/`.unified-stat-card` 系列卡片样式
- `server/internal/web/static/app.js`：新增 `fetchUnifiedStats`/`renderUnifiedStats`/`formatUnifiedTokens`/`formatUnifiedCost`；refreshAll/自动刷新/首屏加载三处接入

**文档**：
- 新建：token-cost-data-sources.md（《token/费用数据来源矩阵》，17 平台盘点）
- 更新：phase3-plan.md（第三批验收项勾选）、project-status（v7，本版）、project-tracking（v7）