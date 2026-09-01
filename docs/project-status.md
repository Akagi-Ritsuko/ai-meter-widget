# 项目工作进度报告

> 版本：v4 | 报告日期：2026-09-01
> 关联：[roadmap.md](roadmap.md) | [decisions.md](decisions.md) | [phase2-plan.md](phase2-plan.md) | [project-tracking.md](project-tracking.md)

## 1. 项目基本信息

| 项目 | 内容 |
|---|---|
| 项目名称 | ai-meter-widget（基于 onWatch v2.13.5 fork） |
| 模块标识 | `github.com/onllm-dev/onwatch/v2`（Go 1.26.7） |
| 当前里程碑 | Phase 2 通用适配器 + P0 平台（代码完成） |
| 报告日期 | 2026-09-01 |
| 决策记录 | ADR-001 ~ ADR-015（[decisions.md](decisions.md)） |

## 2. 里程碑总览

| Phase | 状态 | 完成度 | 剩余事项 |
|---|---|---|---|
| 1 底座跑通 | ✅ 核心完成 | 90% | Claude/Codex 凭证验证、24h soak、交叉编译 |
| 2 通用适配器 + P0 | 🔄 代码完成 | 96% | DeepSeek/OpenCode 真实验证（待用户凭证）；智谱面板显示待重启确认 |
| 3 统一指标 API | 未开始 | 0% | - |
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
- **待办**：重启 onwatch 确认面板显示（沙箱限制无法自动重启）。

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
| 智谱面板用量全 0（日志无错误） | 国内版返回 `CREDIT_LIMIT` 类型，解析器只认 `TIME_LIMIT`/`TOKENS_LIMIT` | `ToSnapshot` 新增 `CREDIT_LIMIT` 分支（ADR-015） | ✅ 修复 + 单测；面板待重启确认 |

完整明细：[ark-interface-research.md](ark-interface-research.md) §9。

## 5. 测试与验证状态

| 套件 | 结果 |
|---|---|
| Ark 适配器（api/store/tracker/agent） | ✅ 34 例全绿 |
| generic 包 | ✅ 24 例全绿，覆盖率 72% |
| store / tracker / config / agent | ✅ 全绿 |
| web 包 | ⚠️ 1 例既有失败（Windows Q8，非本项目引入） |
| api 包全量 | ⚠️ `extra_coverage_test.go` Windows 构建失败（既有，临时移开跑） |
| 真实接口验证 | ✅ Ark Coding Plan；✅ 智谱（2026-09-01，CREDIT_LIMIT 已修复）；⏳ DeepSeek/OpenCode 待凭证 |

## 6. 待办清单

| 优先级 | 项 | 责任 | 说明 |
|---|---|---|---|
| P1 | 重启 onwatch 确认智谱面板显示（CREDIT_LIMIT 修复生效） | guotao | 新二进制已构建，沙箱限制需手动重启 |
| P1 | DeepSeek 真实验证（P2-10） | 用户提供 `DEEPSEEK_API_KEY` | 配置后面板验证 |
| P1 | 智谱 GLM 面板确认（P2-11） | 用户提供 `ZAI_API_KEY` + `ZAI_REGION=cn` | 接口已验证，面板待重启确认 |
| P1 | OpenCode 真实验证（P2-12） | 用户提供 `OPENCODE_GO_*` | 同上 |
| P2 | Phase 1 收尾（24h soak、交叉编译、Claude/Codex 验证） | 执行人 | 不阻塞 Phase 3 |
| P2 | CDP 无头模式探索 | 执行人 | 减少"调试 Edge 需运行"约束 |
| - | Phase 3 统一指标 API | - | 下一里程碑 |

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