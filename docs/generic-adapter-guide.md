# 通用适配器零代码接入指南（P2-15）

> 交付物 P2-15 | 覆盖需求：FR-1.3 / FR-3.1~3.4 / FR-4.1~4.3 / NFR-4
> 目标：不写一行采集代码，仅通过配置页即可接入任意有 HTTP 接口的 AI 平台。

## 1. 配置模型

每个平台由 4 部分组成：**基本信息 + 认证方式 + 数据源 + 字段映射**。

```json
{
  "name": "my_platform",          // 平台标识（唯一）
  "display_name": "My Platform",  // 显示名称
  "enabled": true,                // 是否启用
  "interval": 300,                // 轮询间隔（秒），默认 300
  "auth": {                       // 认证方式
    "type": "api_key",            // api_key | bearer | cookie | oauth_local | none
    "header": "Authorization",    // 请求头名称（默认 Authorization）
    "key": "",                    // 凭证值（直接填写，列表会脱敏）
    "key_from": "env:MY_KEY"      // 凭证来源（推荐）：env:VAR 或 file:path:jsonpath
  },
  "sources": [                    // 数据源（可多个）
    {
      "name": "balance",          // quota | balance | cost | tokens
      "url": "https://api.example.com/v1/balance",
      "method": "GET",
      "interval": 0,              // 覆盖平台级间隔（秒），0 用平台级
      "mapping": {                // 统一字段 -> JSONPath 表达式
        "balance.amount": "$.data.balance",
        "balance.currency": "$.data.currency"
      },
      "static": {}                // 统一字段 -> 静态值（如 quota.window -> "5h"）
    }
  ]
}
```

## 2. 统一指标字段

| 数据源类型 | 可用字段 |
|---|---|
| `quota` | `quota.window`（窗口名）、`quota.used`、`quota.total`、`quota.percent`、`quota.reset_at`、`quota.unit` |
| `balance` | `balance.amount`、`balance.currency` |
| `cost` | `cost.today`、`cost.month`、`cost.currency` |
| `tokens` | `tokens.today`、`tokens.month` |

> `quota.percent` 未映射时自动计算：`used / total * 100`。

## 3. 四种认证方式示例

### 3.1 api_key（请求头携带 Key）

```json
"auth": { "type": "api_key", "header": "Authorization", "key_from": "env:MY_API_KEY" }
```
请求头：`Authorization: <key>`（可自定义 header 名）。

### 3.2 bearer（Bearer Token）

```json
"auth": { "type": "bearer", "key_from": "env:MY_TOKEN" }
```
请求头：`Authorization: Bearer <key>`。

### 3.3 cookie（Cookie 认证）

```json
"auth": { "type": "cookie", "key": "session=abc123; csrf=xyz" }
```
请求头：`Authorization: session=abc123; csrf=xyz`（整串 Cookie）。

### 3.4 oauth_local（本机登录态文件）

```json
"auth": { "type": "oauth_local", "key_from": "file:~/.codex/auth.json:$.tokens.access_token" }
```
从本机登录态文件读取凭证（只读，不复制不上传）。支持相对路径（`~` 展开为用户目录）。

## 4. JSONPath 表达式示例

| 场景 | 表达式 |
|---|---|
| 嵌套对象 | `$.data.usage.tokens` |
| 数组首元素 | `$.data.quotas[0].used` |
| 数组过滤 | `$.data.quotas[?(@.level=='weekly')].percent` |
| 多级路径 | `$.result.plan.quota.limit` |

## 5. 完整接入示例（可复现）

以接入一个返回余额的模拟平台为例：

1. **打开配置页**：登录面板 → 设置 → 通用适配器 →（或直接访问 `/generic`）
2. **填写基本信息**：名称 `mock_balance`、显示名 `Mock Balance`、间隔 30s
3. **认证方式**：选 `none`（无需认证）
4. **添加数据源**：
   - 类型：`balance`
   - URL：`http://localhost:18080/balance`
   - 映射：`balance.amount` → `$.data.balance`，`balance.currency` → `$.data.currency`
5. **点击"测试连接"**：应显示 `balance: 成功` + 映射结果
6. **点击"保存平台"**：列表出现新平台
7. **等待轮询**：`GET /api/generic/metrics/mock_balance` 返回指标数据

## 6. 测试连接排错

| 现象 | 原因 | 处理 |
|---|---|---|
| `认证失败 (HTTP 401/403)` | 凭证错误或过期 | 检查 key/key_from，重新获取凭证 |
| `服务端错误 (HTTP 5xx)` | 上游故障 | 稍后重试，检查接口地址 |
| 映射字段为空 | JSONPath 表达式不匹配 | 用"测试连接"的原始返回体核对路径 |
| `凭证解析失败` | env 变量未设置或文件路径错误 | 检查 env:VAR 或 file:path:jsonpath 格式 |
| 平台状态 `auth_failed` | 凭证无效 | 面板显示错误原因，修正后重试 |

## 7. 安全说明

- 凭证建议用 `env:VAR` 引用（密钥写入 `.env`，数据库只存引用）
- 列表 API 对凭证脱敏（`sk-1****abcd`）
- 日志不打印凭证