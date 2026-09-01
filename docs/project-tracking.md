<!--
 * @Author: guotao
 * @Date: 2026-09-01
 * @LastEditors: guotao
 * @LastEditTime: 2026-09-01
 * @FilePath: \ai-meter-widget\docs\project-tracking.md
 * @Description: 项目标准化跟踪文档（变更记录 / 里程碑规划 / 需求待完成项）
 *
 * Copyright (c) 2026 by lzlj, All Rights Reserved.
-->
# 项目跟踪文档（Project Tracking）

> 版本：v1 | 更新日期：2026-09-01 | 更新人：guotao
> 用途：统一记录项目**变更记录 / 里程碑规划 / 需求待完成项**，作为团队查阅与跟踪项目进展的标准化入口。
> 关联文档：[roadmap.md](roadmap.md)（里程碑详情）| [project-status.md](project-status.md)（工作进度）| [decisions.md](decisions.md)（决策记录）| [phase2-plan.md](phase2-plan.md)（任务计划）| [requirements.md](requirements.md)（需求定义）

---

## 1. 变更记录（Change Log）

> 记录文档/代码/功能的所有版本更新。版本号递增规则：功能或修复落地 → 版本号 +1；仅文档修订 → 修订号 +1（如 v4.1）。

| 版本 | 日期 | 更新人 | 变更内容摘要 | 变更原因 |
|---|---|---|---|---|
| v1 | 2026-08-27 | guotao | Phase 1 底座跑通：fork onWatch v2.13.5、Windows 编译运行、内置适配器与本机登录态探测 | 项目启动，建立 monorepo 结构与可运行底座 |
| v2 | 2026-08-28 | guotao | 火山引擎 Ark 适配器五层开发完成（api/store/tracker/agent/web）+ Coding Plan 支持（控制台 Cookie 鉴权） | 补齐 P0 平台缺口（FR-1.1），用户实际订阅为 Coding Plan |
| v3 | 2026-08-31 | guotao | 通用适配器引擎激活（P2-01~08）+ Ark 面板集成（前端三件套/设置面板）+ Cookie 自动刷新（CDP+浏览器 DB 组合） | Phase 2 代码全部完成；Coding Plan Cookie 2 天过期需自动刷新 |
| v4 | 2026-09-01 | guotao | 智谱 Coding Plan 真实接口验证 + **CREDIT_LIMIT 解析修复**（zai_types.go 新增 CREDIT_LIMIT 分支 + 单测） | 国内版智谱（open.bigmodel.cn）返回 `CREDIT_LIMIT` 类型，原解析器只认 `TIME_LIMIT`/`TOKENS_LIMIT`，导致面板用量全 0 |

### 1.1 本次变更明细（v4，2026-09-01）

| 项 | 内容 |
|---|---|
| 变更类型 | Bug 修复（智谱国内版用量解析） |
| 触发问题 | 面板 Z.ai 卡片用量全 0，日志无错误（`success: true` 正常解析） |
| 根因 | 国内版接口返回 `CREDIT_LIMIT` 类型 limit，`ToSnapshot` 仅处理 `TIME_LIMIT`/`TOKENS_LIMIT`，两个 limit 均被跳过 |
| 修复 | [zai_types.go](file:///d:/workSpace/program/02-PuppeteerTools/07-AiMeter/ai-meter-widget/server/internal/api/zai_types.go) `ToSnapshot` 新增 `CREDIT_LIMIT` 分支：`number=5`（5 小时窗口）→ Time 字段；`number=1`（周窗口）→ Tokens 字段（保留 `nextResetTime` 倒计时） |
| 测试 | [zai_types_test.go](file:///d:/workSpace/program/02-PuppeteerTools/07-AiMeter/ai-meter-widget/server/internal/api/zai_types_test.go) 新增 `TestZaiQuotaResponse_ToSnapshot_DomesticCreditLimit`（真实响应样本），api 包 Zai 套件全绿 |
| 验证数据 | 真实接口返回：Lite 套餐，5 小时窗口 0/2000（0%），周窗口 2005/2000（100%，已超额） |
| 待办 | 重启 onwatch 后确认面板显示（沙箱限制无法自动重启，需手动执行） |

---

## 2. 里程碑规划（Milestone Plan）

> 各阶段详细验收标准见 [roadmap.md](roadmap.md)。

| 阶段 | 名称 | 计划时间 | 预期目标 | 交付成果 | 状态 |
|---|---|---|---|---|---|
| Phase 1 | 底座跑通 | 2026-08-27 ~ 2026-08-31 | fork onWatch 并在 Windows 编译运行，验证内置适配器与本机登录态探测 | monorepo 结构、onwatch.exe、Web 面板（localhost:9211）、内置适配器探测 | ✅ 核心完成（90%） |
| Phase 2 | 通用适配器 + P0 平台 | 2026-08-28 ~ 2026-09-01 | 配置驱动通用适配器引擎 + 补齐 P0 平台（火山 Ark 新写；DeepSeek/智谱/OpenCode 验证） | 通用适配器引擎、Ark 适配器（Agent Plan + Coding Plan + Cookie 自动刷新）、配置页、智谱 CREDIT_LIMIT 修复 | 🔄 代码完成（95%），真实验证待凭证 |
| Phase 3 | 统一指标 API + 展示配置 | 未排期 | 统一指标模型 API（REST+WS）、token/费用统计、90 天历史趋势、展示端配置管理 | 统一指标 API、历史趋势图表、展示配置管理 | ⬜ 未开始 |
| Phase 4 | 桌面浮窗 + 告警 | 未排期 | Tauri 透明置顶小窗展示用量，支持低配额/余额不足告警 | 桌面浮窗、告警通知（配额 ≥90% / 余额低于阈值） | ⬜ 未开始 |
| Phase 5 | ESP32 墨水屏/LED | 未排期 | ESP32 墨水屏/LED 展示用量 | 固件（轮询聚合 API）、展示配置（`display: epaper`） | ⬜ 未开始 |

### 2.1 当前阶段关键节点（Phase 2）

| 节点 | 时间 | 目标 | 状态 |
|---|---|---|---|
| P2-01~08 通用适配器激活 | 2026-08-31 | 引擎接线、4 种认证、配置页、端到端零代码接入 | ✅ 完成 |
| P2-09 火山 Ark 适配器 | 2026-08-28 ~ 08-31 | 五层实现 + Coding Plan + Cookie 自动刷新 + 面板集成 | ✅ 完成（真实接口验证通过） |
| P2-10 DeepSeek 验证 | 待用户凭证 | 配置 `DEEPSEEK_API_KEY` 后面板显示余额 | ⏳ 待凭证 |
| P2-11 智谱 GLM 验证 | 2026-09-01 | 配置 `ZAI_API_KEY` + `ZAI_REGION=cn` 后面板显示配额 | 🔄 接口已验证 + 解析已修复，待重启确认 |
| P2-12 OpenCode 验证 | 待用户凭证 | 配置 `OPENCODE_GO_*` 后面板显示配额 | ⏳ 待凭证 |
| P2-13~16 安全与文档 | 2026-08-31 | 凭证安全、脱敏、接入指南、验收清单 | ✅ 完成 |

---

## 3. 需求待完成项（Pending Requirements）

> 优先级定义：**P0** = 当前阶段验收必需；**P1** = 质量与完整性需要；**P2** = 增强项/收尾项。

| # | 需求描述 | 优先级 | 负责人 | 计划完成时间 | 当前状态 |
|---|---|---|---|---|---|
| 1 | 重启 onwatch 确认智谱 CREDIT_LIMIT 修复生效（面板 Z.ai 卡片显示 5h/周窗口用量） | P1 | guotao | 2026-09-01 | 🔄 代码已修复+单测通过，待重启验证 |
| 2 | DeepSeek 真实验证（P2-10）：配置 `DEEPSEEK_API_KEY` 后面板显示余额/费用/token | P1 | 用户提供凭证 | 待凭证 | ⏳ 待用户凭证 |
| 3 | 智谱 GLM 真实验证（P2-11）：配置 `ZAI_API_KEY` + `ZAI_REGION=cn` 后面板显示配额 | P1 | 用户提供凭证 | 待凭证 | 🔄 接口已验证（Lite 套餐数据正常返回），面板待重启确认 |
| 4 | OpenCode 真实验证（P2-12）：配置 `OPENCODE_GO_*` 后面板显示配额 | P1 | 用户提供凭证 | 待凭证 | ⏳ 待用户凭证 |
| 5 | Phase 1 收尾：24h 内存稳定性 soak（NFR-2）、macOS/Linux 交叉编译（NFR-1）、Claude/Codex 凭证验证 | P2 | 执行人 | 不阻塞 Phase 3 | ⏳ 待执行 |
| 6 | CDP 无头模式探索：减少"调试 Edge 需运行"约束 | P2 | 执行人 | 后续 | ⏳ 待探索 |
| 7 | Phase 3 统一指标 API + 展示配置（FR-2/FR-6/FR-7） | - | - | 下一里程碑 | ⬜ 未开始 |
| 8 | Phase 4 桌面浮窗 + 告警（FR-5.2/FR-8） | - | - | 后续里程碑 | ⬜ 未开始 |
| 9 | Phase 5 ESP32 墨水屏/LED（FR-5.3） | - | - | 后续里程碑 | ⬜ 未开始 |
| 10 | 技术债：`extra_coverage_test.go` Windows 构建失败（引用仅 `!windows` 定义的 `getCredentialsFilePath`） | P2 | - | 后续 | ⚠️ 既有（测试时临时移开该文件） |
| 11 | 技术债：web/agent 包既有 Windows 测试失败（Q8 等） | P2 | - | 后续 | ⚠️ 既有（与本项目改动无关） |
| 12 | 技术债：config 包测试沙箱环境失败（TempDir 指向 C:\Windows 被拒） | P2 | - | 后续 | ⚠️ 环境限制（真实环境正常） |

---

## 4. 使用说明

1. **变更落地**：任何代码/功能变更完成后，在 §1 变更记录追加一行（版本 +1），并在 §1.1 记录根因与修复明细。
2. **里程碑推进**：阶段开工/完成时更新 §2 状态与关键节点表。
3. **需求跟踪**：新增需求在 §3 追加条目；完成时更新状态为 ✅ 并注明完成日期。
4. **一致性**：本文档为跟踪入口，详细内容以关联文档为准；更新本文档时同步检查关联文档状态。