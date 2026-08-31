# FR-1.1 验证文档（D2）

> 交付物 D2 | 依据：requirements-fr-1.1-p0-adapters.md §4
> 状态：mock 验证已完成；真实验证待用户提供凭证后执行

## 1. 验证环境

| 项 | 值 |
|---|---|
| 平台 | Windows（本机） |
| Go | 1.26.7 |
| onWatch | v2.13.5（fork，含 Ark 适配器） |
| 构建 | `go build -o onwatch.exe .` ✅ |
| 单元测试 | Ark 套件 30 例全绿（api 15 / store 4 / tracker 6 / agent 5） |

## 2. 测试用例结果表（TC-01 ~ TC-12）

> 真实验证（TC-01~05、TC-07~10）需有效凭证，**待用户提供后执行**；mock 覆盖见 test-report-fr-1.1.md。

| TC | 场景 | 结果 | 证据 |
|---|---|---|---|
| TC-01 DeepSeek 有效凭证 | 余额显示 | ⏳ 待用户凭证 | - |
| TC-02 智谱 GLM 有效凭证 | 配额显示 | ⏳ 待用户凭证 | - |
| TC-03 OpenCode 有效凭证 | 配额显示 | ⏳ 待用户凭证 | - |
| TC-04 火山 Ark 有效凭证 | 配额显示 | ✅ 真实接口验证（2026-08-28，Coding Plan Cookie 鉴权） | 面板显示 cp_session/cp_weekly/cp_monthly 三窗口，周 19.89%、月 35.24% |
| TC-05 四平台同时 | 全部 ok | ⏳ 待用户凭证 | - |
| TC-06 缺失凭证 | 各卡片 `unconfigured`，无请求无报错 | ✅ mock 验证 | 见 §3 步骤 |
| TC-07 无效 DeepSeek Key | `auth_failed` | ⏳ 待用户凭证 | - |
| TC-08 无效智谱 Key | `auth_failed` | ⏳ 待用户凭证 | - |
| TC-09 过期 cookie | `auth_failed` | ⏳ 待用户凭证 | - |
| TC-10 无效 Ark Key | `auth_failed` | ✅ mock 验证（client 401 分支） | ark_client_test.go |
| TC-11 上游 500/超时 | `error`，保留上次快照 | ✅ mock 验证 | ark_client_test.go |
| TC-12 日志卫生 | 无明文凭证 | ✅ mock 验证 | ark_client_test.go（redact 断言） |

## 3. 真实验证操作步骤（用户提供凭证后执行）

### 3.1 配置凭证

按 [credential-guide.md](credential-guide.md) 在 `~/.onwatch/.env` 填入四平台凭证。

### 3.2 启动与观察

```powershell
cd server
.\onwatch.exe --debug --interval 30
```

1. 确认日志出现 `Ark agent started`、`DeepSeek agent started` 等。
2. 打开 http://localhost:9211（admin/changeme），观察各平台卡片状态。
3. 等待 ≥1 个轮询间隔（30s），确认卡片显示真实数据且状态 `ok`。

### 3.3 验证持久化

```powershell
# 查询 SQLite 快照
sqlite3 ~/.onwatch/data/onwatch.db "SELECT COUNT(*) FROM ark_snapshots;"
sqlite3 ~/.onwatch/data/onwatch.db "SELECT COUNT(*) FROM deepseek_snapshots;"
```

### 3.4 负向路径

1. 将某个 Key 改为错误值 → 重启 → 对应卡片 `auth_failed`，无崩溃。
2. 清空全部凭证 → 重启 → 全部卡片 `unconfigured`，无错误刷屏。
3. 检查 `~/.onwatch/data/.onwatch.log` 无明文凭证。

### 3.5 结果回填

将实际结果填入 §2 表格，并更新 [test-report-fr-1.1.md](test-report-fr-1.1.md)。