# 架构设计（Architecture）

## 1. 架构总览

```
┌──────────────────────────────────────────────────────────┐
│ 展示层（可插拔，各自独立消费 API）                        │
│  ├─ Web 面板（内置，onWatch 已有）                       │
│  ├─ 桌面浮窗（Tauri 透明置顶小窗，二期）                 │
│  └─ ESP32 墨水屏/LED（三期，HTTP 轮询）                  │
└───────────────┬──────────────────────────────────────────┘
                │ REST API + WebSocket（统一指标模型）
┌───────────────▼──────────────────────────────────────────┐
│ 聚合服务（Go 守护进程，fork onWatch）                    │
│  ├─ 内置适配器：Claude/Codex/Copilot/MiniMax/Z.ai/...    │
│  │   （继承 onWatch + 新增 P0 平台）                     │
│  ├─ 通用适配器：配置驱动，零代码接入新平台               │
│  │   URL + 认证方式 + JSONPath 字段映射 → 统一指标模型   │
│  ├─ SQLite：当前值 + 90 天历史趋势 + 重置倒计时          │
│  └─ 配置管理 API + 配置页（新增/测试/启停平台）          │
└──────────────────────────────────────────────────────────┘
```

核心原则：**采集/聚合层与展示层彻底解耦**。所有展示端只认统一指标模型，聚合服务负责把各家接口归一化。

## 2. 统一指标模型

所有平台归一化后的输出契约，展示层与适配器都依赖它。

```json
{
  "platform": "my_platform",
  "display_name": "My Platform",
  "status": "ok",
  "updated_at": "2026-08-27T10:00:00Z",
  "metrics": {
    "quota": [
      {
        "window": "5h",
        "used": 75,
        "total": 100,
        "percent": 75,
        "reset_at": "2026-08-27T12:00:00Z",
        "unit": "requests"
      }
    ],
    "balance": { "amount": 12.5, "currency": "USD" },
    "cost": { "today": 1.2, "month": 30.5, "currency": "USD" },
    "tokens": { "today": 123456, "month": 2345678 }
  }
}
```

字段说明：

| 字段 | 类型 | 说明 |
|---|---|---|
| `quota[]` | 数组 | 配额窗口列表（如 5h 窗口、7 天窗口），每项含窗口名/已用/总量/百分比/重置时间 |
| `balance` | 对象 | 账户余额与币种 |
| `cost` | 对象 | 今日/周期费用与币种 |
| `tokens` | 对象 | 今日/周期 token 消耗 |
| `status` | 枚举 | `ok` / `error` / `auth_failed` / `unconfigured` |

## 3. 模块职责

### 聚合服务（fork onWatch）

- 守护进程，常驻后台，按各平台配置的间隔轮询采集
- 提供 Web 面板（localhost:9211）与 REST/WebSocket API
- SQLite 存储当前值与历史趋势

### 内置适配器

- 代码实现的平台采集逻辑（认证 + 请求 + 解析）
- 继承 onWatch 已有 8 家（Claude/Codex/Copilot/MiniMax/Z.ai/Gemini CLI/Antigravity/Synthetic）
- 新增 P0 平台（DeepSeek/火山/智谱/OpenCode），参考 openusage、token-health 等轮子的实现

### 通用适配器（核心新增）

配置驱动的通用 HTTP 采集器，支持：

- **认证方式**：`api_key` / `bearer` / `cookie` / `oauth_local`（读取本机登录态文件）
- **多数据源**：一个平台可配置多个 source（如 quota 源 + balance 源 + cost 源）
- **字段映射**：JSONPath 表达式将接口响应字段映射到统一指标模型
- **轮询间隔**：每个 source 独立配置

### 配置管理

- 平台配置（内置适配器的启停 + 通用适配器的完整配置）
- 展示端配置（每个展示端显示哪些平台/指标/刷新频率）
- 配置存储于数据目录，配置页可视化编辑

### API 层

- `GET /api/platforms` — 平台列表与状态
- `GET /api/metrics` — 全部平台指标
- `GET /api/metrics/:platform` — 单平台指标
- `GET /api/history/:platform` — 历史趋势
- `GET/POST/PUT /api/config/platforms` — 平台配置管理
- `GET/POST/PUT /api/config/displays` — 展示端配置管理
- `POST /api/config/test` — 测试连接与映射预览
- WebSocket `/ws` — 指标变更推送

### SQLite 存储

- `metrics` 表：各平台当前指标快照
- `history` 表：历史趋势（保留 90 天）
- `config` 表：平台与展示端配置

## 4. 数据流

```
定时触发（按 source 间隔）
  → 适配器认证并请求接口
  → JSONPath 映射为统一指标模型
  → 写入 SQLite（当前值 + 历史）
  → 通过 REST/WebSocket 推送给展示端
```

## 5. 技术选型

| 组件 | 选型 | 理由 |
|---|---|---|
| 聚合服务 | Go（fork onWatch） | 单二进制、跨平台、低资源（<60MB 内存） |
| 桌面浮窗 | Tauri | 跨平台、体积小、透明置顶窗口成熟 |
| 存储 | SQLite | 本地、零配置、单文件 |
| 字段映射 | JSONPath（`PaesslerAG/jsonpath`） | 成熟库，配置页可实时预览 |
| 配置格式 | YAML/JSON | 人类可读，配置页可视化编辑 |
| 墨水屏固件 | ESP32 + Arduino/ESP-IDF | 生态成熟，参考 rlcd-dashboard/clawdmeter |

## 6. 展示层接入协议

- 展示端通过 REST API 拉取指标，或订阅 WebSocket 实时推送
- 每个展示端有一份展示配置（`display` 配置），声明显示哪些平台/指标/刷新频率
- 新增展示端 = 新增一份展示配置 + 实现一个消费 API 的客户端，聚合服务零改动

## 7. 部署与打包

- 聚合服务编译为单二进制，Web 面板资源内嵌
- 数据目录（配置/数据库/日志）与二进制分离，位于用户目录（如 `~/.ai-meter/`）
- 打包 = 二进制 + 数据目录模板，拷贝到新电脑直接运行，无需安装
- Windows 提供 PowerShell 安装脚本（参考 onWatch 的 install.ps1）

## 8. 安全与凭证管理

- 凭证存于 `.env` 或系统凭据库，不落盘明文到数据库
- `oauth_local` 只读本机登录态文件，不复制、不传输
- 日志不打印凭证与敏感响应
- 所有数据本地存储，无遥测、无云依赖