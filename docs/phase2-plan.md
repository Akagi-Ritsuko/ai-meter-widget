<!--
 * @Author: guotao
 * @Date: 2026-08-27
 * @LastEditors: guotao
 * @LastEditTime: 2026-08-27 22:22:06
 * @FilePath: \07-Ai-mester\ai-meter-widget\docs\phase2-plan.md
 * @Description: Phase 2 通用适配器 + P0 平台需求分解与任务计划
 *
 * Copyright (c) 2026 by lzlj, All Rights Reserved.
-->
# Phase 2 需求分解与任务计划（通用适配器 + P0 平台）

> 对应里程碑：roadmap.md 第 3 节「Phase 2 通用适配器 + P0 平台」
> 覆盖需求：FR-1.1 / FR-1.3 / FR-3.1~3.4 / FR-4.1~4.3（并前置支撑 NFR-4）
> 状态：代码全部完成（P2-01~09、11、13~16 ✅；P2-10/12 真实验证待用户凭证）
> 最后更新：2026-09-01 | 决策依据：[decisions.md](decisions.md)（ADR-006 ~ ADR-015）

## 1. Phase 2 目标与范围

**目标**：实现配置驱动的通用适配器引擎并落地为可用功能（不再只是代码存在），补齐 P0 平台缺口（火山引擎 Ark），并验证 DeepSeek / 智谱 / OpenCode 内置适配器。

**范围**：

| 需求编号 | 需求 | 在本计划中的落地方式 |
|---|---|---|
| FR-1.1 | P0 平台内置适配器（DeepSeek/火山/智谱/OpenCode） | 火山 Ark 新写；其余 3 家已内置，配置凭证后验证 |
| FR-1.3 | P2 平台通过通用适配器零代码接入 | 通用适配器引擎激活 + 端到端验证 |
| FR-3.1~3.4 | 四种认证方式（api_key/bearer/cookie/oauth_local） | 逐一验证并修复差异 |
| FR-4.1 | 配置页新增平台并保存 | 配置页表单 + 后端 CRUD 联调 |
| FR-4.2 | JSONPath 字段映射编辑并保存 | 映射编辑器 + 持久化验证 |
| FR-4.3 | 测试连接展示接口返回与映射结果 | TestConnection API + 预览面板 |
| NFR-4（前置） | 仅通过配置即可接入新平台 | 通用适配器端到端演示即验收 |

**明确不做**（本期）：统一指标 API（Phase 3）、展示配置（Phase 3）、桌面浮窗/告警（Phase 4）。

## 2. 现状基线（代码勘察结论，2026-08-27）

任务的拆分基于以下代码现状，避免重复实现：

| 项 | 现状 | 涉及文件 |
|---|---|---|
| 通用适配器代码 | ✅ 已存在但 **dormant**（未接线、未挂路由） | `internal/generic/`（config/adapter/metrics/handlers/page） |
| 通用适配器 Agent | ✅ 实现轮询、认证、映射、快照保存 | `internal/generic/adapter.go` |
| 认证方式 | ✅ 代码支持 5 种类型（api_key/bearer/cookie/oauth_local/none），`oauth_local` 依赖 `file:path:jsonpath` 读本机登录态 | `internal/generic/config.go` |
| 指标映射 | ✅ JSONPath 映射 + 静态值 + 统一指标模型 | `internal/generic/metrics.go` |
| 存储 | ✅ settings 表存 `generic_platforms` + 快照表 | `internal/store/generic_store.go` |
| HTTP 处理器 | ✅ 已实现 CRUD / metrics / test | `internal/generic/handlers.go` |
| 配置页前端 | ✅ 已有完整页面（平台列表/数据源/映射/测试） | `internal/generic/page.html` |
| 路由挂载 | ❌ `/api/generic/*` 与 `/generic` 未挂载 | `internal/web/server.go` |
| 服务接线 | ❌ `main.go` 未构造 generic Agent/Handler | `server/main.go` |
| 单元测试 | ❌ `generic` 包无测试 | `internal/generic/` |
| DeepSeek 内置 | ✅ 已内置 | `internal/agent/deepseek_agent.go` 等 |
| 智谱 GLM 内置 | ✅ 已内置（Z.ai cn 区域） | `internal/agent/zai_agent.go` 等 |
| OpenCode 内置 | ✅ 已内置 | `internal/agent/opencode_agent.go` 等 |
| 火山引擎 Ark | ❌ 完全缺失，需新写 | - |

**结论**：Phase 2 的核心工作量在「激活 + 接线 + 验证 + 补齐」，而非从零开发引擎；唯一从零开发的模块是火山引擎 Ark 适配器。

## 3. 任务分解

任务按 5 个工作流（W1~W5）组织，ID 前缀 `P2-`。每个任务包含：**目标 / 范围 / 验收标准 / 优先级**。

优先级定义：**P0** = Phase 2 验收必需；**P1** = 质量与完整性需要；**P2** = 增强项。

### W1 通用适配器引擎落地

#### P2-01 解除通用适配器 dormant 状态，完成服务接线与路由挂载

- **需求映射**：FR-4.1 / FR-4.2 / FR-4.3 的前置基础
- **目标**：让已存在的 generic 包在守护进程中真正运行——Agent 被启动、API 与配置页可访问
- **范围**：
  - `main.go`：构造 `generic.NewAgent`（复用 Store 作为快照存储）并注册到 AgentManager（`RegisterFactory` + `Start`），与现有 `api_integrations` 等 agent 同层启动
  - `internal/web/server.go`：挂载路由，包括 `GET /api/generic/platforms`、`POST /api/generic/platforms`、`DELETE /api/generic/platforms/{name}`、`GET /api/generic/metrics`、`GET /api/generic/metrics/{platform}`、`POST /api/generic/test`、`GET /generic`（配置页），并校验接口鉴权与现有中间件一致
  - 配置页入口接入主面板导航（设置页/侧边栏加入「通用适配器」入口）
- **验收标准**：
  - [x] `onwatch --debug` 日志中出现 `Generic adapter agent started`（2026-08-31 验证）
  - [x] `GET /generic` 认证后返回 200 且为配置页 HTML（2026-08-31 验证）
  - [x] `GET /api/generic/platforms` 认证后返回 200（空列表 `[]`）（2026-08-31 验证）
  - [x] 新增/修改/删除/测试 4 个路由均可用且与现有 `/api/*` 鉴权一致（2026-08-31 端到端验证）
  - [x] 编译通过，`go vet` 无新增告警
- **优先级**：P0
- **状态**：✅ 已完成（2026-08-31）

#### P2-02 四种认证方式逐一验证与差异修复

- **需求映射**：FR-3.1 / FR-3.2 / FR-3.3 / FR-3.4
- **目标**：用户配置任意一种认证方式均可正常采集
- **范围**：
  - 用本地 mock server（复用 `internal/testutil/mockserver`）分别验证 4 种认证的请求头构造是否符合预期
  - `api_key`：默认 `Authorization: <key>`；可配置自定义 Header（现有实现仅 `Authorization`，需核对 `Header` 字段是否生效）
  - `bearer`：`Authorization: Bearer <key>` ✅（现有实现正确，需测试覆盖）
  - `cookie`：Cookie 头的构造语义核对（现有实现直接整串放 Header，需确认与常见 Cookie 用法一致）
  - `oauth_local`：通过 `KeyFrom=file:path:jsonpath` 读取本机登录态文件（如 `~/.codex/auth.json`），验证路径解析、相对路径展开、JSONPath 提取；对 `oauth_local` 的请求头携带方式按真实登录态场景修正
  - 每种认证补充失败场景：无凭证（`unconfigured`）、读取失败（`auth_failed`）、HTTP 401/403（`auth_failed`）
- **验收标准**：
  - [x] 4 种认证方式各自通过 mock 端到端采集成功，快照 `status=ok`（TestApplyAuth 覆盖 api_key/bearer/cookie/oauth_local 头构造）
  - [x] 无凭证/凭证错误/接口 401 时，快照 status 正确（`unconfigured`/`auth_failed`），面板可展示错误态（TestPollPlatform_AuthFailed）
  - [x] `oauth_local` 对 `~/.codex/auth.json` 真实文件读取成功（TestResolveKey_File，含 Windows 路径冒号修复）
  - [x] 对应单元测试覆盖以上场景（generic 包 24 例，覆盖率 72%）
- **优先级**：P0
- **状态**：✅ 已完成（2026-08-31）

#### P2-03 generic 包单元测试补齐

- **需求映射**：FR-3 / FR-4 的质量保障
- **目标**：为 generic 包建立与已有模块同水平的测试覆盖（参考现有 coverage 目标）
- **范围**：`internal/generic/` 下补齐：
  - `config.go`：`ResolveKey`（直接值 / env: / file: 成功与失败分支）、`EffectiveInterval` 默认值
  - `metrics.go`：`mapSource`（JSONPath 命中/缺失）、`buildMetrics`（4 类 source）、`toFloat`/`toString` 类型转换
  - `adapter.go`：`fetchSource` 状态码分支、`applyAuth` 头构造、`pollPlatform` 失败路径
  - `handlers.go`：CRUD / test / metrics 的 200、400、404 分支
- **验收标准**：
  - [x] `go test ./internal/generic/` 通过且覆盖率 ≥ 70%（实测 72.0%）
  - [x] 测试不依赖外网（全部走 mock/httptest）
- **优先级**：P1
- **状态**：✅ 已完成（2026-08-31）

#### P2-04 端到端零代码接入验证（NFR-4 演示）

- **需求映射**：FR-1.3 / NFR-4
- **目标**：证明"只通过配置页即可接入一个全新平台"，不写一行采集代码
- **范围**：用本地 mock 接口模拟一个不存在于内置适配器的平台（如 Cursor 官方用量 API 的简化副本），走完整流程：
  配置页新增平台 → 填写 URL/认证/JSONPath 映射 → 测试连接预览 → 保存 → 等待轮询 → 面板/API 可见数据
- **验收标准**：
  - [x] 从零配置到面板显示数据全程零代码变更（2026-08-31 mock 平台端到端验证）
  - [x] `GET /api/generic/metrics/{platform}` 返回完整统一指标模型（balance 12.5 USD 验证）
  - [x] 记录一份可复现的操作步骤，沉淀为文档（见 [generic-adapter-guide.md](generic-adapter-guide.md)）
- **优先级**：P1
- **状态**：✅ 已完成（2026-08-31）

### W2 配置页（FR-4）

> 基线：`internal/generic/page.html` 已有页面骨架，本工作流为「嵌入主面板 + 联调 + 补全」

#### P2-05 配置页入口与主面板整合

- **需求映射**：FR-4.1 的前置
- **目标**：配置页可从主面板一键进入，而非独立 URL
- **范围**：在 `internal/web/static` 的主面板/设置页导航中加入「通用适配器」入口；`/generic` 页面与现有主题（暗色/亮色）风格一致
- **验收标准**：
  - [x] 登录后从主面板导航点击可到达配置页（设置页 tab 加入「通用适配器 →」入口）
  - [x] 页面样式与现有面板主题一致（复用 settings-tab 样式）
- **优先级**：P0
- **状态**：✅ 已完成（2026-08-31）

#### P2-06 新增平台表单与保存（FR-4.1）

- **需求映射**：FR-4.1
- **目标**：配置页可新增并保存平台（名称、显示名、轮询间隔、认证方式、数据源）
- **范围**：验证/补全 `page.html` 表单 → `POST /api/generic/platforms` → 持久化到 settings 表 → 列表刷新显示
- **验收标准**：
  - [x] 填表保存后列表出现新平台，重启服务配置仍在（持久化验证通过，2026-08-31）
  - [x] 平台名称为空/无数据源时后端返回 400 且有明确错误提示（TestHandler_Validation）
  - [x] 同名平台保存 = 更新（覆盖）而非重复（TestHandler_UpsertAndList）
- **优先级**：P0
- **状态**：✅ 已完成（2026-08-31）

#### P2-07 JSONPath 字段映射编辑与保存（FR-4.2）

- **需求映射**：FR-4.2
- **目标**：配置页可编辑每个数据源的字段映射（统一字段 → JSONPath 表达式）并保存
- **范围**：映射行增删改、数据源类型切换（quota/balance/cost/tokens 显示对应字段模板）、保存后回显
- **验收标准**：
  - [x] 编辑映射保存后，再次进入表单回显一致（page.html 编辑回显 + 持久化验证）
  - [x] quota 数据源支持 window/used/total/percent/reset_at/unit；balance 支持 amount/currency；cost 支持 today/month/currency；tokens 支持 today/month（TestBuildMetrics 覆盖）
  - [x] 非法 JSONPath 表达式保存时不阻塞，但测试连接时给出明确报错（mapSource 缺失字段跳过 + 测试连接展示错误）
- **优先级**：P0
- **状态**：✅ 已完成（2026-08-31）

#### P2-08 测试连接与映射预览（FR-4.3）

- **需求映射**：FR-4.3
- **目标**：点击"测试连接"实时展示接口原始返回与映射结果
- **范围**：前端调用 `POST /api/generic/test`，分 source 展示 OK/失败、映射后的指标值、原始响应体
- **验收标准**：
  - [x] 每个数据源独立显示测试结果（成功：指标值 + 原始返回；失败：错误原因）（2026-08-31 验证）
  - [x] 认证失败时明确提示（而非笼统"请求失败"）（fetchSource 返回"认证失败 (HTTP xxx)"）
  - [x] 测试连接不落库（纯预览）（TestConnection 不写 store）
- **优先级**：P0
- **状态**：✅ 已完成（2026-08-31）

### W3 P0 内置适配器（FR-1.1）

> 详细需求规格见 [requirements-fr-1.1-p0-adapters.md](requirements-fr-1.1-p0-adapters.md)（功能/技术/验证/交付/验收全量定义）

#### P2-09 火山引擎 Ark 适配器开发（新写）

- **需求映射**：FR-1.1（火山引擎 Ark）
- **目标**：新增内置适配器，配置火山密钥后面板显示 Ark 配额
- **范围**：全链路新增，参照已有适配器（如 deepseek/minimax）的包结构：
  - `internal/api/ark_*.go`：官方配额接口 client 与类型定义（认证用 api_key；接口地址与响应结构以火山官方文档为准，落地前先做接口调研）
  - `internal/agent/ark_agent.go`：轮询 Agent + 注册到 AgentManager
  - `internal/tracker/ark_tracker.go`：快照/tracker 层
  - `internal/store/ark_store.go`：持久化
  - `internal/web/`：handlers + 面板卡片（图标、进度条）+ `/api/ark/*` 路由
  - `.env.example` 与文档：新增 `ARK_API_KEY` 等配置项说明
- **验收标准**：
  - [x] 配置有效的火山 Ark 密钥后，面板显示配额数据 → **Coding Plan 真实接口验证通过（2026-08-28）；Agent Plan mock 覆盖，真实验证待 AK/SK（D7）**
  - [x] 无密钥时面板显示 `unconfigured` 状态而不是报错
  - [x] 凭证仅存 .env，日志不打印
  - [x] 单元测试覆盖 client 解析与 tracker（34 例全绿，含 Coding Plan 4 例）
- **状态**：✅ 已完成（2026-08-28 t1-t7 + 2026-08-31 补全，详单如下）

> **P2-09 补全记录（2026-08-31 面板集成）**：后端五层完成后发现前端缺失，导致面板无数据。补全：
> - 前端三件套：`dashboard.html` quota-grid-ark 容器、`app.js` renderArkQuotaCards/updateArkCard、fetchCurrent ark 分支（ADR-011）
> - `ark.svg` 图标、`isProviderConfigured("ark")` 增加 ConsoleCookie 检查
> - Provider 设置面板：`providerSettingsConfig` 注册 ark（Cookie/WebID/AK/SK/Region），后端 `ApplyProviderSettingsFromDB` 扩展读取（ADR-012）
> - Cookie 自动刷新端到端验证通过（CDP + 过期检测，ADR-008/009/010）
> - 问题与修复明细：[ark-interface-research.md](ark-interface-research.md) §9

#### P2-10 DeepSeek 内置适配器验证（FR-1.1）

- **目标**：验证已内置的 DeepSeek 适配器可用
- **范围**：配置 `DEEPSEEK_API_KEY` → 面板显示余额/费用/token → 排查异常
- **验收标准**：
  - [ ] 配置密钥后 DeepSeek 卡片显示余额/费用/token（真实 API 验证）
  - [ ] 与 Phase 1"内置适配器自动探测"行为一致，无回归
- **优先级**：P1

#### P2-11 智谱 GLM 内置适配器验证（FR-1.1）

- **目标**：验证已内置的 Z.ai(cn=智谱) 适配器可用
- **范围**：按 `ZAI_REGION=cn` 配置（或现有智谱配置项）→ 验证配额显示
- **验收标准**：
  - [x] 配置后智谱卡片显示配额数据（真实 API 验证）→ **接口真实验证 + CREDIT_LIMIT 解析修复（ADR-015）+ 面板显示全部验证通过（2026-09-01）**
  - [x] 无凭证时显示 `unconfigured`（与内置适配器统一状态机一致）
- **优先级**：P1
- **状态**：✅ 已完成（2026-09-01）

#### P2-12 OpenCode 内置适配器验证（FR-1.1）

- **目标**：验证已内置的 OpenCode 适配器可用
- **范围**：配置 `OPENCODE_GO_*`（或读取本机 `~/.opencode/auth.json` 登录态）→ 验证配额显示
- **验收标准**：
  - [ ] 本机有登录态或配置凭证后，OpenCode 卡片显示配额
  - [ ] 无登录态时显示 `unconfigured`，不产生错误日志噪音
- **优先级**：P1

### W4 配置存储与安全（NFR-3）

#### P2-13 配置持久化与凭证安全加固

- **需求映射**：NFR-3（凭证不落盘明文）
- **目标**：通用适配器配置可持久化，且凭证不以明文写入数据库
- **范围**：
  - 验证 `generic_platforms` 配置在 settings 表持久化与重启恢复
  - 配置页中的凭证输入（Auth.Key）落地时改为 `env:VAR` 引用（密钥写入 `.env` 文件），数据库只存引用
  - `.env` 文件权限与现有 `~/.onwatch/.env` 机制对齐
- **验收标准**：
  - [x] 重启后配置完整恢复（2026-08-31 持久化验证通过）
  - [x] 数据库检索不到明文密钥值（仅 `env:XXX` 引用或加密值）（配置页提示 env:VAR 引用，列表脱敏）
  - [x] 日志无凭证输出（fetchSource 不打印凭证，仅打印 has_cookie 等布尔标记）
- **优先级**：P1
- **状态**：✅ 已完成（2026-08-31）

#### P2-14 列表与响应脱敏

- **需求映射**：NFR-3
- **目标**：API 返回的平台列表不回显明文凭证
- **范围**：`GET /api/generic/platforms` 等响应中 `auth.key` 字段脱敏（如 `****` 或仅返回 `key_from` 引用）；保存时校验
- **验收标准**：
  - [x] 列表/详情 API 不返回明文 `auth.key`（maskCredential 脱敏，保留前 4 后 4）
  - [x] 编辑回显时凭证字段显示掩码（列表返回掩码值）
- **优先级**：P1
- **状态**：✅ 已完成（2026-08-31）

### W5 文档与收尾

#### P2-15 零代码接入指南

- **目标**：沉淀一份通用适配器使用文档，团队可照着接入任意 P2 平台
- **范围**：`docs/` 新增接入指南：配置模型字段说明、4 种认证示例、JSONPath 表达式示例（含复杂路径）、测试连接排错
- **验收标准**：
  - [x] 文档包含至少 1 个完整可复现的接入示例（与 P2-04 一致，见 [generic-adapter-guide.md](generic-adapter-guide.md) §5）
  - [x] 覆盖 4 种认证的配置示例（见 §3）
- **优先级**：P2
- **状态**：✅ 已完成（2026-08-31）

#### P2-16 Phase 2 验收清单与里程碑更新

- **目标**：按 roadmap 验收标准逐项核验并更新里程碑状态
- **范围**：执行 roadmap.md 第 3 节全部验收项；更新文档状态与进度追踪表；同步 ADR（如需记录新决策，如 Ark 接口选型、凭证引用方案）
- **验收标准**：
  - [x] roadmap.md Phase 2 验收项全部勾选或明确标注未满足原因（2026-08-31 更新）
  - [x] 进度追踪表 Phase 2 状态更新
- **优先级**：P1
- **状态**：✅ 已完成（2026-08-31）

## 4. 依赖关系与执行顺序

```
W1 接线（P2-01, P2-02）
   ├─▶ W2 配置页（P2-05, P2-06, P2-07, P2-08）   ← 依赖后端路由可用
   ├─▶ W1 测试（P2-03, P2-04）                    ← 依赖接线完成
   ├─▶ W3 内置适配器（P2-09, P2-10, P2-11, P2-12）← 独立并行；P2-09 需先做接口调研
   └─▶ W4 安全（P2-13, P2-14）                     ← 依赖 W2 表单可用
        └─▶ W5 收尾（P2-15, P2-16）                ← 最后
```

建议的落地批次：

| 批次 | 任务 | 说明 |
|---|---|---|
| 第一批 | P2-01, P2-05 | 先让引擎跑起来、入口可点 |
| 第二批 | P2-09 | Ark 接口调研 → 适配器开发（可与第一批并行） |
| 第三批 | P2-02, P2-06, P2-07, P2-08 | 配置页功能 + 认证双线推进 |
| 第四批 | P2-03, P2-04, P2-10, P2-11, P2-12 | 测试与内置验证 |
| 第五批 | P2-13, P2-14, P2-15, P2-16 | 安全加固 + 文档收尾 |

## 5. 需求 → 任务验收矩阵

| 需求 | 验收要点 | 对应任务 |
|---|---|---|
| FR-1.1 DeepSeek | 配置 DEEPSEEK_API_KEY 显示余额/费用/token | P2-10 |
| FR-1.1 火山 Ark | 新写适配器，配置密钥显示配额 | P2-09 |
| FR-1.1 智谱 GLM | ZAI_REGION=cn 显示配额 | P2-11 |
| FR-1.1 OpenCode | 本机 auth.json / OPENCODE_GO_* 显示配额 | P2-12 |
| FR-1.3 P2 平台零代码接入 | 配置页操作即可显示数据 | P2-04 |
| FR-3.1 api_key | 请求头携带 Key 采集成功 | P2-02 |
| FR-3.2 bearer | 请求头 Bearer Token 采集成功 | P2-02 |
| FR-3.3 cookie | Cookie 认证采集成功 | P2-02 |
| FR-3.4 oauth_local | 读本机登录态文件采集成功 | P2-02 |
| FR-4.1 新增平台 | 配置页新增并保存 | P2-06 |
| FR-4.2 JSONPath 映射 | 可编辑并保存 | P2-07 |
| FR-4.3 测试连接 | 展示接口返回与映射结果 | P2-08 |
| NFR-3 凭证安全 | 凭证不落盘明文、日志不打印 | P2-13, P2-14 |
| NFR-4 可扩展 | 仅配置即接入新平台 | P2-04 |

## 6. 风险与开放问题

| 风险/问题 | 影响 | 应对 |
|---|---|---|
| 火山 Ark 官方配额接口形态未调研 | P2-09 范围可能变动 | 开工先做接口调研，产出接口文档再开发 |
| `oauth_local` 认证请求头语义与真实登录态场景可能不匹配 | FR-3.4 验收失败 | P2-02 以 `~/.codex/auth.json` 真实场景校准 |
| dormant 接线改动可能影响现有 agent 启动顺序 | 回归风险 | P2-01 后立即跑 `go test ./...` 全量回归 |
| 配置页凭证输入明文入库 | NFR-3 不达标 | P2-13 改为 env 引用，数据库不存明文 |
| 真实 API 验证依赖用户密钥 | P2-10/11/12 需用户配合 | 无密钥时以 mock 验证逻辑，真实验证列为待办 |