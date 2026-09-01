<!--
 * @Author: guotao
 * @Date: 2026-09-01
 * @FilePath: \07-Ai-mester\ai-meter-widget\docs\token-cost-data-sources.md
 * @Description: token/费用数据来源矩阵（Phase 3 P3-04 产出物）
 *
 * Copyright (c) 2026 by lzlj, All Rights Reserved.
-->
# token/费用数据来源矩阵（P3-04）

> 关联：[phase3-plan.md](phase3-plan.md) P3-04 | 契约：[architecture.md](architecture.md) §2（`cost`/`tokens` 对象）
> 勘察日期：2026-09-01 | 状态口径：✅ 可直接映射 / ⚠️ 部分可得 / ❌ 无数据源（需新增采集或接入遥测）

## 1. 口径说明

- **统一模型**：`cost = {today, month, currency}`、`tokens = {today, month}`（[architecture.md](architecture.md) §2）。无数据的维度输出 `null`（JSON 指针为空），**不输出假 0**，避免误读。
- **capabilities 标注决策**：契约中未定义 `capabilities` 字段；为避免契约漂移且前端暂无消费方，本矩阵文档即为「不可得 → unavailable」的标注载体，统一 API 侧仅以字段置 null 表达。后续如展示端需要机器可读能力位，再评估新增字段（新 ADR）。
- **易混淆点**：智谱 `TOKENS_LIMIT`/`tokensLimit`、kimi `/usages` 计数是**配额窗口**（quota 维度），不是 token 消耗统计；codex/anthropic 的「会话」是配额利用率级，不存在 `sessionTokens`（全库 grep 零匹配）。
- **今日/月度语义**：以聚合服务本地时区的自然日/自然月为准，取自各数据源在快照/遥测入库时携带的当日（或账期）累计值。

## 2. 矩阵（provider × 数据项 × 来源 × 状态）

### 2.1 跨 provider 遥测（唯一完整 tokens+cost 来源）

| 通道 | 数据项 | 来源 | 覆盖 provider | 状态 |
|---|---|---|---|---|
| API Integrations 遥测 | tokens（prompt/completion/total，支持今日/任意区间） | `api_integration_usage_events` 表（外部脚本 JSONL 自报，5s tail 摄取） | anthropic/openai/mistral/openrouter/gemini | ✅ 已持久化，聚合输出见 `/api/integrations/current` |
| API Integrations 遥测 | cost（`cost_usd`，支持今日/任意区间） | 同上 | 同上 | ✅ 同上 |

### 2.2 内置 provider 逐家盘点（16 家）

| provider | tokens | cost | 结论 |
|---|---|---|---|
| openrouter | ❌ 平台无 token 统计（遥测通道可覆盖） | ✅ `OpenRouterSnapshot.UsageDaily/UsageMonthly`（美元，`GET /api/v1/auth/key`） | **cost 本批接入**（Today=usage_daily、Month=usage_monthly、USD）；tokens 走遥测 |
| cursor | ❌ | ⚠️ `CursorPlanUsage.TotalSpend`（美分，账期累计） | 遥测通道可覆盖 tokens；cost 为账期 spend 无「今日」维度，后续批次评估映射为月度 |
| deepseek | ❌ | ⚠️ 仅余额（`/user/balance`），无消费流水接口接入 | 余额走 balance 维度；cost 需新增采集 |
| moonshot | ❌ | ⚠️ 仅余额三字段 | 同上 |
| codex | ❌（sessions 表为配额利用率级） | ⚠️ 仅 `CreditsBalance`（余额非消费） | 需新增采集；遥测通道归属 openai 需确认 |
| anthropic | ❌（statusline 桥接仅利用率） | ❌ | 需新增采集，或引导接入遥测脚本 |
| zai | ❌（`TOKENS_LIMIT` 为预算配额，勿误用） | ❌（Coding Plan 无费用接口） | 需新增采集 / 不可得 |
| kimi | ⚠️ `/usages` 返回原始计数，单位未确认 | ❌ | 勘察单位后评估 |
| grok | ⚠️ `GrokLocalSessionSummary.TotalTokens`（本地会话级，informational，非今日聚合） | ❌ | 不可靠，暂不映射 |
| antigravity | ❌ | ⚠️ PromptCredits/MonthlyCredits 为额度非消费 | 需新增采集 |
| minimax | ❌ | ❌ | 无数据源 |
| gemini | ❌（遥测通道可覆盖） | ❌ | 走遥测 |
| copilot | ❌ | ❌（entitlement 非消费） | 无数据源 |
| synthetic | ❌（请求次数配额） | ❌ | 无数据源 |
| ark | ❌ | ❌ | 无数据源（官方无费用接口） |
| opencode | ❌ | ❌ | 无数据源 |

### 2.3 通用适配器（generic）

| 数据项 | 状态 | 说明 |
|---|---|---|
| tokens / cost | ✅ | source 映射原生支持（`cost.today/cost.month/cost.currency`、`tokens.today/tokens.month` JSONPath 映射或静态值），P2 已验证；P3-04 以 mock 平台端到端验证统一 API 直通输出 |

## 3. 本批落地（P3-04 验收）

| 验收项 | 落地 |
|---|---|
| ≥2 平台真实输出 tokens 或 cost（1 内置 + 1 generic mock） | openrouter → cost（真实美元数据）；generic mock 平台 → cost/tokens 直通（单测端到端） |
| 无数据平台输出 null 而非 0 | `Metrics.Cost/Tokens` 为指针，无数据即 null（第一批四家 ark/zai/deepseek/opencode 均保持 null） |
| 数据来源矩阵文档 | 本文档 |

## 4. 后续方向（不阻塞验收）

- cursor/deepseek/moonshot 的账期 spend 映射（无「今日」维度的标注口径）。
- kimi `/usages` 单位勘察。
- 引导用户接入 API Integrations 遥测脚本，把 anthropic/gemini 等的 token 级数据带进统一 API。
- 如需机器可读能力位（capabilities），走新 ADR 评估契约扩展。
