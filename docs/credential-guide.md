# 凭证配置指南（D3）

> 交付物 D3 | 依据：requirements-fr-1.1-p0-adapters.md CRD-04
> 覆盖平台：DeepSeek / 火山 Ark / 智谱 GLM / OpenCode
> 版本：v4 | 更新：2026-09-01（智谱国内版 CREDIT_LIMIT 解析修复说明）

## 1. 通用说明

凭证有三种配置途径（按优先级）：

| 途径 | 说明 | 适用场景 |
|---|---|---|
| **面板设置页** | Settings → Providers → 对应 provider 的 ⚙️ 齿轮按钮 | 日常修改，UI 操作 |
| **`.env` 文件** | `~/.onwatch/.env`（或项目目录 `.env`） | 初始配置、批量配置 |
| **CDP 自动提取** | 仅 Ark Coding Plan，全自动 | 推荐的 Coding Plan 方式 |

安全基线：

- 数据库不存明文凭证（通用适配器建议用 `env:VAR` 引用；列表 API 自动脱敏）。
- 日志对凭证脱敏（`redact*APIKey` 模式，如 `sk-1***...***abc`）。
- 本机登录态文件（`~/.codex/auth.json` 等）只读复用，不复制、不上传。

## 2. 各平台凭证

### DeepSeek（深度求索）

| 变量 | 说明 |
|---|---|
| `DEEPSEEK_API_KEY` | DeepSeek 开放平台 API Key |

获取：https://platform.deepseek.com → API Keys 页面创建。

### 火山引擎 Ark（火山方舟）

**Coding Plan（控制台 Cookie 鉴权）——推荐方式：CDP 自动刷新**

1. 运行 `scripts/start-edge-debug.bat` 启动调试 Edge（独立配置目录，不影响日常浏览器）
2. 在该 Edge 中登录 https://console.volcengine.com/ark/region:ark+cn-beijing/openManagement（仅一次）
3. `.env` 配置占位值即可启用：

```bash
ARK_CONSOLE_COOKIE=pending-cdp-refresh
ARK_CDP_DEBUG_URL=http://localhost:9222
```

之后 onWatch 自动从浏览器提取最新 Cookie（Cookie 过期或 401 时触发），无需手动维护。

> 前提：调试 Edge 保持运行。Cookie 有效期：digest JWT 约 2 天、userInfo JWT 约 30 天；30 天内至少访问一次控制台保持登录态。

**Coding Plan（手动方式，备选）**

| 变量 | 说明 |
|---|---|
| `ARK_CONSOLE_COOKIE` | 控制台 Cookie（含 `csrfToken`） |
| `ARK_CONSOLE_WEB_ID` | `x-web-id` 请求头值（可选，实测可省略） |
| `ARK_CONSOLE_CSRF_TOKEN` | `x-csrf-token`（可选，默认从 Cookie 提取） |

获取：登录控制台 → F12 → Network → 刷新 → 找到 `GetCodingPlanUsage` 请求 → Copy as cURL → 提取 `Cookie` 与 `x-web-id`。

> 注意：手动方式 Cookie 约 2 天过期（digest），需定期重新复制；推荐改用 CDP 方式。

**Agent Plan（AK/SK 鉴权）**

| 变量 | 说明 |
|---|---|
| `ARK_ACCESS_KEY` | IAM Access Key ID |
| `ARK_SECRET_KEY` | IAM Secret Key（仅创建时展示一次） |
| `ARK_REGION` | 可选，默认 `cn-beijing` |
| `ARK_BASE_URL` | 可选，默认 `https://ark.cn-beijing.volcengineapi.com` |

获取：https://console.volcengine.com/iam/keymanage → 创建密钥对。
注意：SecretKey 需妥善保管，泄露后请立即在控制台禁用并重建。

**面板设置页配置**：Settings → Providers → Volcano Ark ⚙️，可配置 Coding Plan Cookie/WebID、Agent Plan AK/SK、Region（保存后重启 daemon 生效）。

### 智谱 GLM（Z.ai cn 区域 / 智谱 Coding Plan）

| 变量 | 说明 |
|---|---|
| `ZAI_API_KEY` | 智谱开放平台 API Key（Coding Plan 订阅后查询返回套餐额度） |
| `ZAI_REGION` | 设为 `cn`（智谱开放平台 open.bigmodel.cn）或 `global`（z.ai 国际版） |
| `ZAI_BASE_URL` | 可选；`ZAI_REGION=cn` 时默认自动指向 `https://open.bigmodel.cn/api`，无需手动设置 |

获取：https://open.bigmodel.cn → API Keys 页面创建。
面板设置：Settings → Providers → Z.ai ⚙️（API Key + Region）。

> **国内版（cn）说明（2026-09-01 验证）**：
> - 用量接口：`GET https://open.bigmodel.cn/api/monitor/usage/quota/limit`，鉴权 `Authorization: <API Key>`（不带 Bearer 前缀）。
> - 返回 `CREDIT_LIMIT` 类型 limit（5 小时窗口 + 周窗口），与 z.ai 国际版的 `TIME_LIMIT`/`TOKENS_LIMIT` 不同；解析器已支持（ADR-015）。
> - 实测返回示例：Lite 套餐，5 小时窗口 0/2000（0%）、周窗口 2005/2000（100%，已超额）。

### OpenCode（OpenCode Go 订阅）

| 变量 | 说明 |
|---|---|
| `OPENCODE_GO_WORKSPACE_ID` | 工作区 ID（`wrk_...`），来自 opencode.ai 工作区 URL |
| `OPENCODE_GO_AUTH_COOKIE` | opencode.ai 的 `auth` cookie 值（不含 `auth=` 前缀） |
| `OPENCODE_ENABLED` | 可选，`true` 时读取本机 `~/.local/share/opencode/auth.json` 为 Codex 提供凭证 |

获取：登录 https://opencode.ai → 打开工作区 → 浏览器 DevTools → Application → Cookies → 复制 `auth` 值。

## 3. 安全注意事项

1. `.env` 文件权限：Windows 建议限制为当前用户可读写；macOS/Linux 建议 `chmod 600`。
2. 不要将 `.env` 提交到 git（`.gitignore` 已覆盖）。
3. 日志中凭证已脱敏；若发现明文凭证出现在日志，立即轮换该凭证。
4. 本机登录态文件读取为只读操作，不修改、不复制、不传输。
5. 面板设置页保存的敏感字段以密码框输入，列表 API 返回时脱敏（`sk-1****abcd`）。