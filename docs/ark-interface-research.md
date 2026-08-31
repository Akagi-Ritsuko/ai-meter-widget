# 火山方舟 Ark 用量接口调研（D5）

> 交付物 D5 | 依据：requirements-fr-1.1-p0-adapters.md ARK-01
> 调研日期：2026-08-27 | 状态：已批准（编码前调研）
> 版本：v3 | 更新：2026-08-31（新增 §7 Coding Plan、§8 Cookie 自动刷新、§9 实施变更记录）
> 关联决策：[decisions.md](decisions.md) ADR-006 ~ ADR-010、ADR-011 ~ ADR-013

## 目录

- §1-§6：Agent Plan（GetAFPUsage，AK/SK 鉴权）
- §7：Coding Plan（GetCodingPlanUsage，Cookie 鉴权）
- §8：Cookie 自动刷新机制（CDP + 过期检测）
- §9：实施变更记录（问题与修复）

---

## 1. 接口选型

| 项 | 值 |
|---|---|
| 接口 | `GetAFPUsage`（Agent Plan 用量查询） |
| 端点 | `POST https://ark.cn-beijing.volcengineapi.com/?Action=GetAFPUsage&Version=2024-01-01` |
| 请求体 | `{}`（空对象） |
| 鉴权 | **仅支持 Access Key 鉴权**（`AccessKey` + `SecretKey` 成对，HMAC-SHA256 V4 签名） |
| 签名参数 | service=`ark`，region=`cn-beijing`，SignedHeaders=`content-type;host;x-content-sha256;x-date` |

> 企业版用量查询为 `GetSeatAFPUsage`（需 SeatID 列表），不在 P0 范围，仅记录。

## 2. 响应 Schema

```
ResponseMetadata { RequestId, Action, Version, Service, Region, Error{Code, Message} }
Result {
  PlanType: string,
  AFPDaily:     { Quota, Used, SubscribeTime, ResetTime },
  AFPFiveHour:  { Quota, Used, SubscribeTime, ResetTime },
  AFPWeekly:    { Quota, Used, SubscribeTime, ResetTime },
  AFPMonthly:   { Quota, Used, SubscribeTime, ResetTime }
}
```

每个窗口字段：

| 字段 | 类型 | 说明 |
|---|---|---|
| `Quota` | string 或 number | 配额总量（官方文档为 string，API Explorer 示例为 number，需容错解析） |
| `Used` | string 或 number | 已用量（同上） |
| `SubscribeTime` | int64 | 订阅时间（epoch 毫秒） |
| `ResetTime` | int64 | 重置时间（epoch 毫秒） |

## 3. HMAC-SHA256 V4 签名流程

```
CanonicalRequest = POST\n/\n<canonical query>\n
  content-type:application/json\nhost:<host>\nx-content-sha256:<hash>\nx-date:<X-Date>\n\n
  content-type;host;x-content-sha256;x-date\n<hash>

StringToSign = "HMAC-SHA256\n<X-Date>\n<date>/<region>/ark/request\nhex(sha256(canonicalRequest))"

签名密钥链：
  kDate    = HMAC-SHA256(SecretKey, date)
  kRegion  = HMAC-SHA256(kDate, region)
  kService = HMAC-SHA256(kRegion, "ark")
  kSigning = HMAC-SHA256(kService, "request")

Authorization = "HMAC-SHA256 Credential=<AK>/<date>/<region>/ark/request,
  SignedHeaders=content-type;host;x-content-sha256;x-date, Signature=<hex>"
```

请求头：`Content-Type: application/json`、`Host`、`X-Date`（`YYYYMMDDTHHMMSSZ` UTC）、`X-Content-Sha256`、`Authorization`。

## 4. 凭证获取

- 火山引擎控制台 → IAM → 密钥管理：https://console.volcengine.com/iam/keymanage
- 创建 Access Key 后获得 `AccessKey` 与 `SecretKey`（SecretKey 仅创建时展示一次，需妥善保存）

## 5. 限流与错误处理

| 错误码 | 处理 |
|---|---|
| `SignatureDoesNotMatch` / `InvalidAccessKey` / `AccessDenied` / `Forbidden` | `auth_failed` |
| HTTP 429 | `error`（rate_limited），尊重 `Retry-After` |
| HTTP 5xx / 网络 / 超时 | `error`，保留上次快照 |
| 其他 `ResponseMetadata.Error` | `error`（记录 code + message） |

## 6. 备注

- `GetInferenceUsage`（ApiKeyID 查询）为 2026-08-12 后新 Key 的用量查询方式，本期不采用（P0 为 Agent Plan 配额）。
- Quota/Used 类型不一致（string vs number）→ 实现采用 `json.Number` 全链路容错解析（决策 D6）。

---

## 7. Coding Plan（编程套餐）接口调研（2026-08-28 补充）

> 用户实际订阅为 Coding Plan（个人版），与 Agent Plan 是两套独立接口。

### 7.1 接口

| 项 | 值 |
|---|---|
| 接口 | `GetCodingPlanUsage`（控制台内部接口，非 OpenAPI） |
| 端点 | `POST https://console.volcengine.com/api/top/ark/cn-beijing/2024-01-01/GetCodingPlanUsage?` |
| 请求体 | `{}` |
| 鉴权 | **浏览器 Cookie** + `x-csrf-token`（从 Cookie 的 `csrfToken` 提取）+ `x-web-id` |
| 参考实现 | [ArkCodingPlanUsage](https://github.com/xiaokaiyyy/ArkCodingPlanUsage)（MIT） |

### 7.2 响应 Schema（2026-08-28 真实接口验证）

```
ResponseMetadata { RequestId, Action, Version, Service, Region, Error{Code, Message} }
Result {
  Status: string,          // "Running"
  UpdateTimestamp: int64,  // epoch 秒
  QuotaUsage: [
    { Level: "session", Percent: 0,        ResetTimestamp: -1,        Cap: 100, RewardTotalPercent: 0 },
    { Level: "weekly",  Percent: 19.89,    ResetTimestamp: 1788105600, Cap: 100, RewardTotalPercent: 0 },
    { Level: "monthly", Percent: 35.24,    ResetTimestamp: 1788105599, Cap: 100, RewardTotalPercent: 0 }
  ],
  HasReward: bool
}
```

| 字段 | 说明 |
|---|---|
| `Level` | `session`（5h 滚动窗口）/ `weekly` / `monthly` |
| `Percent` | 已用百分比（0-100），**百分比制**（区别于 Agent Plan 的 used/limit 制） |
| `ResetTimestamp` | 重置时间（epoch 秒），`-1` 表示无重置（session 窗口） |
| `Cap` | 上限（通常 100） |

### 7.3 凭证获取

**方式一（推荐）：CDP 自动提取**
- 运行 `scripts/start-edge-debug.bat` 启动调试 Edge → 登录控制台一次 → onWatch 自动提取刷新
- `.env` 配置占位值 `ARK_CONSOLE_COOKIE=pending-cdp-refresh` 即可启用

**方式二：手动复制**
1. 登录 https://console.volcengine.com/ark/region:ark+cn-beijing/openManagement
2. F12 → Network → 刷新 → 找到 `GetCodingPlanUsage` 请求 → Copy as cURL
3. 提取 `Cookie`（含 `csrfToken`）与 `x-web-id`，填入 `.env` 或面板设置（Settings → Providers → Volcano Ark ⚙️）

### 7.4 注意事项

- Cookie 会过期（digest JWT 约 2 天，userInfo JWT 约 30 天），CDP 自动刷新可消除手动操作；手动方式需定期更新。
- 企业版 Coding Plan 用 `GetSeatInfoUsage`（AK/SK + SeatID），不在本期范围。

---

## 8. Cookie 自动刷新机制（2026-08-31 验证通过）

### 8.1 架构

```
Cookie 快过期（<1天）/ 401 / 解析失败
  → CompositeCookieExtractor
      ├─ CDPCookieExtractor（优先，浏览器运行时）
      │    GET localhost:9222/json → WebSocket → Network.getAllCookies
      │    过滤 domain 含 volcengine.com 的 Cookie
      └─ BrowserCookieExtractor（兜底，浏览器关闭时）
           Edge/Chrome SQLite + DPAPI/v10/v20 解密
  → 更新 client.cookie + csrfToken（从 Cookie 提取）
```

### 8.2 关键实现要点

| 要点 | 说明 |
|---|---|
| **父域过滤** | digest/userInfo/csrfToken 存储在 `.volcengine.com`（非 console 子域），过滤条件必须匹配 `volcengine.com` |
| **JWT 过期检测** | 解析 `digest`（约 2 天）与 `userInfo`（约 30 天）的 `exp`，取最早过期时间；剩余 <3 天日志预警 |
| **x-web-id** | 实测非必需，可不配置 |
| **触发条件** | `lastAuthFailed \|\| Cookie 解析失败 \|\| 剩余 <1 天` |
| **启动脚本** | `scripts/start-edge-debug.bat`：注册表定位 Edge + 独立配置目录（`%LOCALAPPDATA%\ai-meter-widget-edge`）规避启动加速接管 + 纯 ASCII 避免 cmd 编码问题 |

### 8.3 端到端验证记录（2026-08-31）

| 步骤 | 结果 |
|---|---|
| 无效 Cookie（`csrfToken=expired-invalid`）启动 onwatch | provider 启用，首次请求 401 |
| CDP 自动触发 | 日志 `Cookie 已从浏览器自动刷新`，提取 35 个 Cookie |
| 刷新后请求 | 成功，weekly 8.61%（重置 09-07）、monthly 4.30%（重置 09-30） |
| 二次调用 | 复用已刷新 Cookie，不重复提取 |

---

## 9. 实施变更记录（问题与修复）

| 日期 | 问题 | 根因 | 修复 | 关联 |
|---|---|---|---|---|
| 2026-08-28 | 冒烟测试请求发到无主机地址（`?Action=GetAFPUsage`） | `WithArkBaseURL("")` 清空默认端点 | 空值时不覆盖默认值 | ADR-007 |
| 2026-08-31 | CDP 提取缺 digest/userInfo/csrfToken | 这些 Cookie 存于父域 `.volcengine.com`，过滤条件只匹配 `console.volcengine.com` | 过滤改为匹配 `volcengine.com` | ADR-010 |
| 2026-08-31 | onwatch 修复后仍失败 | 旧二进制未包含 CDP 修复 | 重新构建 onwatch.exe | - |
| 2026-08-31 | 面板 Ark 标签页无数据、last-updated 不更新 | 前端缺失容器/渲染函数/fetch 分支 | 补全三件套 + ark.svg 图标 | ADR-011 |
| 2026-08-31 | Provider Controls 中 Ark 无设置入口 | `providerSettingsConfig` 未注册 ark | 前后端各补 Ark 设置（Cookie/AK/SK/Region） | ADR-012 |
| 2026-08-31 | Insights 卡片与配额卡片样式不一致 | `.insight-card` 独立样式 | 基础样式对齐 `.quota-card` | ADR-014 |