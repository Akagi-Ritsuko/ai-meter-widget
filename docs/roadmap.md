# 里程碑与进展追踪（Roadmap）

> 版本：v5 | 更新日期：2026-09-01
> 决策依据：[decisions.md](decisions.md)（ADR-001 ~ ADR-015）

## 1. 里程碑总览

| Phase | 名称 | 覆盖需求 | 状态 |
|---|---|---|---|
| 1 | 底座跑通 | FR-1.2, FR-5.1, NFR-1/2/3 | ✅ 核心完成（待 Claude/Codex 凭证验证、24h soak） |
| 2 | 通用适配器 + P0 平台 | FR-1.1, FR-1.3, FR-3, FR-4 | 🔄 代码全部完成（P2-10/12 真实验证待用户凭证；智谱接口已验证+解析修复，面板待重启确认） |
| 3 | 统一指标 API + 展示配置 | FR-2, FR-6, FR-7 | 未开始 |
| 4 | 桌面浮窗 + 告警 | FR-5.2, FR-8 | 未开始 |
| 5 | ESP32 墨水屏/LED | FR-5.3 | 未开始 |

## 2. Phase 1 底座跑通

**目标**：fork onWatch 并在 Windows 上编译运行，验证内置适配器与本机登录态探测。

**交付物**：
- fork 仓库 + monorepo 目录结构（server/widget/firmware/docs）
- Windows 编译运行 onWatch
- 内置适配器（Claude/Codex 等）自动探测登录态正常

**验收标准**：
- [x] FR-1.2：Windows 上运行 onwatch 后，localhost:9211 面板可访问（2026-08-27 验证）
- [ ] FR-1.2：配置 Claude/Codex 凭证后，面板显示对应配额数据（本机已自动探测到 Cursor 登录态并采集成功，Claude/Codex 待用户环境验证）
- [x] FR-5.1：Web 面板正常展示各平台用量（Cursor 配额含重置倒计时采集成功）
- [ ] NFR-1：Windows 编译通过，macOS/Linux 交叉编译通过（Windows 已验证）
- [ ] NFR-2：守护进程运行 24h 内存稳定 <60MB（42.4MB WorkingSet 实测，待 24h soak）
- [x] NFR-3：凭证存 .env，日志不打印凭证（Cursor 凭据从本地 SQLite 只读探测）

**状态**：核心完成（2026-08-27 启动）

**已执行内容**：

| 步骤 | 内容                                                                       | 结果         |
| ---- | -------------------------------------------------------------------------- | ------------ |
| 1    | 安装 Go 1.26.7（winget），配置国内代理 `GOPROXY=https://goproxy.cn,direct` | 成功         |
| 2    | fork onWatch v2.13.5 到 `server/`（`git clone --depth 1`）                 | 成功         |
| 3    | 删除 `server/.git`，将嵌套仓库并入父仓库（解决 gitlink 不显示新增的问题）  | 成功         |
| 4    | Windows 编译 `go build -o onwatch.exe .`                                   | 成功（29MB） |
| 5    | 创建数据目录 `~/.onwatch/data`，debug 模式运行                             | 成功         |
| 6    | 验证 Web 面板：localhost:9211 认证后 API 返回 200                          | 成功         |
| 7    | 验证本机登录态探测：Cursor 凭据从本地 SQLite 自动识别                      | 成功         |
| 8    | 验证配额采集：Cursor Auto+Composer / API Usage，含重置倒计时               | 成功         |
| 9    | 内存验证：WorkingSet 42.4MB（NFR-2 <60MB 达标）                            | 成功         |
| 10   | 添加根目录 `.gitignore`（编译产物/凭证/未来 widget/firmware 构建目录）     | 成功         |

**关键发现**：onWatch 已内置 DeepSeek / OpenCode Go / Z.ai(cn=智谱) 适配器，P0 平台仅火山引擎 Ark 需新写。

**剩余待办**（不阻塞 Phase 2）：
- Claude/Codex 凭证验证（取决于本机是否安装对应 CLI）
- macOS/Linux 交叉编译验证
- 24h 内存稳定性测试

## 3. Phase 2 通用适配器 + P0 平台

> 详细需求分解与任务计划见 [phase2-plan.md](phase2-plan.md)；通用适配器使用见 [generic-adapter-guide.md](generic-adapter-guide.md)

**目标**：实现配置驱动的通用适配器引擎，并用 DeepSeek 作为首个落地案例验证；补齐 P0 平台缺口（火山引擎 Ark）。

> 注：Phase 1 已确认 DeepSeek / 智谱(GLM, Z.ai cn) / OpenCode Go 已内置在 onWatch 中，P0 平台仅火山引擎 Ark 需新写适配器。

**交付物**：
- 通用适配器：配置模型 + JSONPath 映射引擎 + 4 种认证方式
- 配置页：新增平台表单 + 测试连接/映射预览
- P0 平台内置适配器：火山引擎 Ark（Agent Plan AK/SK + Coding Plan Cookie 双套餐，含 Cookie 自动刷新）

**验收标准**：
- [x] FR-1.3：配置页填写 URL/认证/字段映射后，新平台即可显示数据（零代码）→ **2026-08-31 mock 平台端到端验证通过**
- [x] FR-3.1/3.2/3.3/3.4：四种认证方式均可工作（oauth_local 参考 CodexBar 读取本机登录态）→ **TestApplyAuth 覆盖，含 Windows 路径修复**
- [x] FR-4.1：配置页可新增平台并保存 → **持久化验证通过**
- [x] FR-4.2：JSONPath 字段映射可编辑并保存 → **TestBuildMetrics 覆盖 4 类 source**
- [x] FR-4.3：测试连接展示接口返回与映射结果 → **2026-08-31 验证**
- [ ] FR-1.1：DeepSeek 余额/费用/token 显示正常（配置 DEEPSEEK_API_KEY 验证）→ ⏳ 待用户凭证
- [x] FR-1.1：火山引擎 Ark 配额显示正常 → **代码+34 例单测完成；Coding Plan 真实接口验证通过；Cookie 自动刷新端到端验证通过；面板渲染/设置 UI 完成**（2026-08-31）
- [x] FR-1.1：智谱 GLM 配额显示正常（ZAI_REGION=cn 验证）→ **接口真实验证通过（2026-09-01，Lite 套餐数据正常返回）；CREDIT_LIMIT 解析已修复（ADR-015），面板显示待重启确认**
- [ ] FR-1.1：OpenCode 读取本机 auth.json 显示配额（配置 OPENCODE_GO_* 验证）→ ⏳ 待用户凭证

**状态**：代码全部完成（P2-01~09、13~16 ✅；P2-10/12 真实验证待用户凭证；P2-11 智谱接口已验证+解析修复，面板待重启确认）

**进展记录**：

### 2026-08-28：Ark 适配器（t1-t9）
- Ark 适配器五层（api/store/tracker/agent/web）代码完成，main.go 接线完成，30 例单测全绿，构建通过
- 交付物 D2/D3/D4/D5 文档落地；需求文档 ARK-02/03 回写完成
- 冒烟验收通过：无凭证 `unconfigured`、伪造凭证优雅失败（`auth_failed`）
- 修复 `WithArkBaseURL` 空值覆盖默认端点 bug（ADR-007）

### 2026-08-28：Ark Coding Plan 支持
- 新增 `GetCodingPlanUsage`（控制台 Cookie 鉴权）客户端，agent 双套餐轮询
- 34 例单测全绿，真实接口验证通过（周 19.89%、月 35.24%）
- 窗口以 `cp_` 前缀命名；JWT 过期检测（digest 2 天 / userInfo 30 天）

### 2026-08-31：通用适配器激活（P2-01~16）
- P2-01 接线+路由挂载（server.go mux/basePath 重构 + main.go Agent/Handler 注册）
- P2-02 认证验证 + **修复 Windows 路径冒号解析 bug**（file:path:jsonpath 从右侧找分隔符）
- P2-03 测试补齐（24 例，覆盖率 72%）
- P2-04 端到端零代码验证（mock 平台 balance 12.5 USD 全流程）
- P2-05 配置页入口（设置页 tab）
- P2-06/07/08 配置页联调（持久化/映射/测试连接）
- P2-13/14 凭证安全（env:VAR 引用提示 + 列表脱敏 maskCredential）
- P2-15 零代码接入指南（generic-adapter-guide.md）
- P2-16 验收清单更新

### 2026-08-31：Ark Cookie 自动刷新（CDP）
- JWT 过期检测 + CDP 提取器 + 浏览器 DB 提取器 + Composite 组合（ADR-008/009/010）
- **修复父域过滤 bug**：digest/userInfo/csrfToken 存于 `.volcengine.com`（非 console 子域）
- **修复旧二进制问题**：CDP 修复后需重新构建 onwatch.exe
- 端到端验证：无效 Cookie → CDP 提取 35 个 Cookie → 自动刷新 → 采集成功（weekly 8.61%、monthly 4.30%）
- 新增 `scripts/start-edge-debug.bat`（独立配置目录启动调试 Edge，规避启动加速接管）

### 2026-08-31：Ark 面板集成补全
- **修复面板无数据**：补全前端三件套（quota-grid-ark 容器 / renderArkQuotaCards+updateArkCard / fetchCurrent ark 分支）+ ark.svg 图标（ADR-011）
- **修复 isProviderConfigured("ark")**：增加 ConsoleCookie 检查（原只查 AK/SK）
- Ark Provider 设置面板（齿轮按钮：Coding Plan Cookie/WebID、Agent Plan AK/SK、Region）（ADR-012）
- `.env` 占位 Cookie 触发启用模式（`pending-cdp-refresh`）（ADR-013）
- Insights 卡片样式统一（.insight-card 对齐 .quota-card）（ADR-014）

### 2026-09-01：智谱 Coding Plan 真实接口验证 + CREDIT_LIMIT 解析修复
- **真实接口验证**：`ZAI_REGION=cn` 请求 `https://open.bigmodel.cn/api/monitor/usage/quota/limit` 返回 HTTP 200（Lite 套餐：5h 0/2000、周 2005/2000 已超额）
- **发现根因**：国内版返回 `CREDIT_LIMIT` 类型，解析器只认 `TIME_LIMIT`/`TOKENS_LIMIT` → 面板全 0
- **修复**：`zai_types.go` 新增 `CREDIT_LIMIT` 分支 + 真实样本单测（ADR-015），api 包 Zai 套件全绿
- **待办**：重启 onwatch 确认面板显示（沙箱限制需手动重启）

## 4. Phase 3 统一指标 API + 展示配置

**目标**：对外暴露统一指标 API，支持 token/费用统计与历史趋势，实现展示端配置管理。

**交付物**：
- 统一指标模型 API（REST + WebSocket）
- token/费用统计（官方用量 API + 通用适配器 source）
- 90 天历史趋势存储与查询
- 展示端配置管理

**验收标准**：
- [ ] FR-2.1/2.2：配额与余额通过统一 API 输出
- [ ] FR-2.3/2.4：token 与费用统计显示正常
- [ ] FR-6.1：配置页可管理多个展示端配置（平台/指标/刷新频率）
- [ ] FR-7.1：面板显示 90 天历史趋势图表
- [ ] FR-7.2：面板显示各配额窗口重置倒计时
- [ ] NFR-4：仅通过配置即可接入新平台（端到端验证）

**状态**：未开始

## 5. Phase 4 桌面浮窗 + 告警

**目标**：Tauri 透明置顶小窗展示用量，支持低配额/余额不足告警。

**交付物**：
- Tauri 桌面浮窗（消费聚合 API，按展示配置渲染）
- 告警通知（配额 ≥90% 或余额低于阈值）

**验收标准**：
- [ ] FR-5.2：透明置顶小窗显示配置的平台用量
- [ ] FR-8.1：配额 ≥90% 或余额低于阈值时弹出桌面通知
- [ ] NFR-5：浮窗与聚合服务分离部署，可独立更新

**状态**：未开始

## 6. Phase 5 ESP32 墨水屏/LED

**目标**：ESP32 墨水屏/LED 展示用量。

**交付物**：
- ESP32 固件（轮询聚合 API）
- 展示配置（`display: epaper`）

**验收标准**：
- [ ] FR-5.3：墨水屏显示配置的平台用量
- [ ] 固件断网/服务不可用时显示降级状态

**状态**：未开始

## 7. 进度追踪表

| Phase | 状态 | 开始日期 | 完成日期 | 备注 |
|---|---|---|---|---|
| 1 底座跑通 | 核心完成 | 2026-08-27 | - | 待 Claude/Codex 凭证验证、24h soak、交叉编译 |
| 2 通用适配器 + P0 | 代码完成 | 2026-08-28 | - | P2-10/12 真实验证待用户凭证；P2-11 智谱接口已验证+解析修复，面板待重启确认 |
| 3 统一指标 API | 未开始 | - | - | |
| 4 桌面浮窗 + 告警 | 未开始 | - | - | |
| 5 ESP32 墨水屏 | 未开始 | - | - | |

> 更新规则：Phase 开工时更新"开始日期"，验收标准全部勾选后更新"完成日期"与状态。