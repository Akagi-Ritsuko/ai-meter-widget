# ai-meter-widget

跨平台 AI 用量聚合服务 + 可插拔展示层。统一监控各平台 AI 的配额剩余、余额、token 消耗与费用，支持桌面浮窗与墨水屏展示。

> fork 自 [onWatch](https://github.com/onllm-dev/onWatch)（GPL-3.0）

## 功能特性

- **多平台聚合**：配额剩余 / 余额 / token / 费用，统一指标模型
- **配置驱动通用适配器**：新平台填配置即可接入，零代码
- **本机登录态复用**：读取本机 CLI/浏览器登录态，无需重复登录
- **可插拔展示层**：Web 面板 / 桌面浮窗(Tauri) / 墨水屏(ESP32)
- **跨平台**：Windows / macOS / Linux
- **可移植部署**：单二进制 + 数据目录，拷到别的电脑直接运行

## 架构总览

```
┌────────────────────────────────────────────────┐
│ 展示层（可插拔）                                │
│  Web 面板 / 桌面浮窗(Tauri) / 墨水屏(ESP32)     │
└───────────────┬────────────────────────────────┘
                │ REST API + WebSocket（统一指标模型）
┌───────────────▼────────────────────────────────┐
│ 聚合服务（Go 守护进程，fork onWatch）           │
│  内置适配器 + 通用适配器(配置驱动) + SQLite     │
└────────────────────────────────────────────────┘
```

## 目录结构

```
ai-meter-widget/
├── server/       # 聚合服务（fork onWatch）
├── widget/       # 桌面浮窗（Tauri，二期）
├── firmware/     # ESP32 墨水屏/LED（三期）
└── docs/         # 文档
```

## 快速开始

> 当前为 Phase 1 状态：聚合服务（fork onWatch v2.13.5）已在 Windows 编译运行。

### 构建（Windows）

```powershell
# 前置：Go 1.25+（国内网络需配置代理）
go env -w GOPROXY=https://goproxy.cn,direct

cd server
go build -o onwatch.exe .
```

### 运行

```powershell
# 首次运行需创建数据目录
New-Item -ItemType Directory -Force -Path "$env:USERPROFILE\.onwatch\data"

# debug 模式（前台，日志输出到终端）
.\onwatch.exe --debug --interval 30
```

打开 **http://localhost:9211**，默认凭据 `admin / changeme`（生产环境务必在 `.env` 中修改）。

### 配置平台

编辑 `%USERPROFILE%\.onwatch\.env`，按需填入平台凭证（详见 [docs/requirements.md](docs/requirements.md) 平台支持矩阵）：

```bash
DEEPSEEK_API_KEY=sk-xxx          # DeepSeek 余额
ZAI_API_KEY=xxx                  # Z.ai / 智谱 GLM（ZAI_REGION=cn）
OPENCODE_GO_WORKSPACE_ID=wrk_xxx # OpenCode Go
OPENCODE_GO_AUTH_COOKIE=xxx
ANTHROPIC_TOKEN=xxx              # Claude（可自动探测本机登录态）
CODEX_TOKEN=xxx                  # Codex（可自动探测 ~/.codex/auth.json）
```

> 本机登录态自动探测：Cursor（本地 SQLite）、Claude Code、Codex、Grok、Kimi 等无需手动配置即可识别。

## 文档索引

| 文档 | 说明 |
|---|---|
| [docs/requirements.md](docs/requirements.md) | 需求分解与平台支持矩阵 |
| [docs/architecture.md](docs/architecture.md) | 架构设计与统一指标模型 |
| [docs/roadmap.md](docs/roadmap.md) | 里程碑计划与进展追踪 |
| [docs/decisions.md](docs/decisions.md) | 关键决策记录（ADR） |
| [docs/generic-adapter-guide.md](docs/generic-adapter-guide.md) | 通用适配器零代码接入指南 |

## License

GPL-3.0，fork 自 [onWatch](https://github.com/onllm-dev/onWatch)