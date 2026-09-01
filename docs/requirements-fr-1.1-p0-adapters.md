<!--
 * @Author: guotao
 * @Date: 2026-08-27
 * @LastEditors: guotao
 * @LastEditTime: 2026-08-27 23:00:00
 * @FilePath: \ai-meter-widget\docs\requirements-fr-1.1-p0-adapters.md
 * @Description: FR-1.1 P0 平台内置适配器（DeepSeek / 火山 Ark / 智谱 / OpenCode）详细需求文档
 *
 * Copyright (c) 2026 by lzlj, All Rights Reserved.
-->
# FR-1.1 P0 Platform Built-in Adapters — Detailed Requirements

> Priority: **P0** | Phase: **2** | Status: Not started
> Related plan: [phase2-plan.md](phase2-plan.md) (tasks P2-09 … P2-12)
> Requirement source: [requirements.md](requirements.md) §FR-1.1

## 1. Feature Overview

FR-1.1 requires built-in (code-level) adapters for the four P0 platforms so that, once credentials are configured, the onWatch daemon polls each platform's official service and shows quota/balance usage on the Web panel at localhost:9211.

| Platform | Status in codebase | Implementation type |
|---|---|---|
| DeepSeek（深度求索） | ✅ Already built in | Verify existing adapter after credential configuration |
| 火山引擎 Ark (Volcano Engine Ark) | ❌ Not present anywhere in the codebase | **Develop from scratch**, full integration |
| 智谱 GLM (Zhipu AI, via Z.ai cn region) | ✅ Already built in | Verify existing adapter after credential configuration |
| OpenCode（OpenCode Go 订阅 / auth.json） | ✅ Already built in | Verify existing adapter after credential configuration |

A built-in adapter follows the standard platform-integration contract shared by every provider in onWatch (see §3.1). The 火山 Ark adapter must be added into that same contract — no special-casing.

### 1.1 Supported metrics per platform

| Platform | Metrics to display | Source |
|---|---|---|
| DeepSeek | Balance (amount, currency), availability | Official balance API `GET /user/balance` |
| 火山 Ark | Quota (usage / limit, per resource) | Official quota interface `POST ?Action=GetAFPUsage&Version=2024-01-01`（四窗口：5h/daily/weekly/monthly，详见 D5 调研文档） |
| 智谱 GLM | Quota (usage / limit) | Official quota API via Z.ai cn region |
| OpenCode | Quota (OpenCode Go subscription: used / limit, format currency or percent, reset time) | Dashboard scrape `https://opencode.ai/workspace/{workspace_id}/go` |

### 1.2 Terminology

- **Adapter**: full-stack Go implementation of one provider = HTTP client (`internal/api`) + polling agent (`internal/agent`) + tracker (`internal/tracker`) + persistence (`internal/store`) + Web handlers & panel card (`internal/web`).
- **Credential**: API key / token / cookie stored in `~/.onwatch/.env` or read read-only from a local login-state file.

## 2. Functional Requirements

### 2.1 火山 Ark — new adapter development (FR-1.1#Ark)

| ID | Requirement |
|---|---|
| ARK-01 | Research the official Volcano Engine Ark usage/quota API first (auth method, endpoint, response schema) and document the findings before implementation. |
| ARK-02 | Implement full adapter stack following the existing DeepSeek/Minimax adapter structure: `internal/api/ark_*.go`, `internal/agent/ark_agent.go`, `internal/tracker/ark_tracker.go`, `internal/store/ark_store.go`, `internal/web/ark_handlers.go` (+ panel card). 路由接入统一分发端点（current/history/cycles/summary/insights/logging-history），**不新增专属路由**，与 DeepSeek/OpenCode 一致。 |
| ARK-03 | Authentication: Access Key 成对凭证 `ARK_ACCESS_KEY` + `ARK_SECRET_KEY`（HMAC-SHA256 V4 签名，service=`ark`，region=`cn-beijing`）。Credential is read from `.env` only, never stored in the DB in plaintext. |
| ARK-04 | Register the agent with `AgentManager.RegisterFactory("ark", …)` so it starts/stops like every other provider. |
| ARK-05 | If the key is unset, the panel must show `unconfigured` status (no error spam, no partial cards). |
| ARK-06 | Mock-server unit tests must cover client parsing and tracker behavior without external network. |

### 2.2 DeepSeek — verify built-in (FR-1.1#DeepSeek)

| ID | Requirement |
|---|---|
| DSK-01 | With `DEEPSEEK_API_KEY` set in `.env`, the DeepSeek dashboard card displays balance (amount + currency) and availability. |
| DSK-02 | With key missing/empty: `unconfigured` status; with invalid key: `auth_failed`; transient failure: `error` with retry on next poll. |
| DSK-03 | No regression to existing auto-detect / poll behavior verified in Phase 1. |

### 2.3 智谱 GLM — verify built-in (FR-1.1#Zhipu)

| ID | Requirement |
|---|---|
| ZHP-01 | The built-in Z.ai adapter serves 智谱 GLM through the cn region: `ZAI_REGION=cn` with `ZAI_API_KEY` set. Quota card displays usage / limit. → **✅ 接口真实验证通过（2026-09-01，Lite 套餐数据正常返回）；CREDIT_LIMIT 解析已修复（ADR-015），面板显示待重启确认** |
| ZHP-02 | `ZAI_REGION=global` continues to work as before (no behavior change). |
| ZHP-03 | Missing credential → `unconfigured`; invalid credential → `auth_failed` with clear message. |

### 2.4 OpenCode — verify built-in (FR-1.1#OpenCode)

| ID | Requirement |
|---|---|
| OPC-01 | OpenCode Go subscription quota is displayed when `OPENCODE_GO_WORKSPACE_ID` + `OPENCODE_GO_AUTH_COOKIE` are configured (dashboard scrape). |
| OPC-02 | The `OPENCODE_ENABLED=true` path (read `~/.local/share/opencode/auth.json` to feed Codex credentials) keeps working unchanged. |
| OPC-03 | Missing config → `unconfigured`; stale/invalid cookie → `auth_failed`, no noisy error logs. |

### 2.5 Standard platform interface (all adapters)

| ID | Requirement |
|---|---|
| STD-01 | Every adapter implements the shared contract: an `agent.AgentRunner` registered via `AgentManager.RegisterFactory(key, factory)`, returning typed snapshots through a tracker, persisted via a store, exposed through Web handlers with a dashboard card. |
| STD-02 | Providers are enabled/disabled independently through existing config (`*_ENABLED` or "credential present" opt-in rules), consistent with onWatch conventions. |
| STD-03 | Each adapter reports one of the platform statuses: `ok` / `error` / `auth_failed` / `unconfigured`, consumed by the same dashboard rendering code. |

### 2.6 Credential management

| ID | Requirement |
|---|---|
| CRD-01 | Credentials are configured exclusively in `~/.onwatch/.env` (env vars) or read-only from local auth files; never entered in the DB in plaintext. |
| CRD-02 | Logging never prints full credentials; API keys are redacted (existing pattern: `redact*APIKey`, showing e.g. `sk-1***...***abc`). |
| CRD-03 | `oauth_local`-style credentials (`~/.codex/auth.json`, `~/.local/share/opencode/auth.json`) are read in a read-only manner — no copy, no upload. |
| CRD-04 | A credential guide (Deliverable D3) documents exactly which variables each adapter needs and where to obtain them. |

## 3. Technical Specifications

### 3.1 Adapter architecture (shared contract)

```
AgentManager (RegisterFactory "deepseek" | "ark" | "zai" | "opencode")
   └─ Agent (AgentRunner, poll loop, interval from config)
        └─ HTTP Client (internal/api)
             ├─ authenticate (Bearer / api_key / cookie)
             ├─ GET official endpoint
             └─ parse → typed response / snapshot
        └─ Tracker → normalized snapshot (quota/balance/cost/tokens)
        └─ Store → SQLite persistence
   └─ Web handlers (/api/<provider>/…) + dashboard card
```

### 3.2 API integration details

| Item | DeepSeek | 智谱 GLM (Z.ai cn) | OpenCode Go | 火山 Ark |
|---|---|---|---|---|
| Env vars | `DEEPSEEK_API_KEY` | `ZAI_API_KEY`, `ZAI_BASE_URL`, `ZAI_REGION=cn` | `OPENCODE_GO_WORKSPACE_ID`, `OPENCODE_GO_AUTH_COOKIE` (opt-in), `OPENCODE_ENABLED` (auth.json path) | `ARK_ACCESS_KEY` + `ARK_SECRET_KEY`（可选 `ARK_REGION`/`ARK_BASE_URL`） |
| Endpoint | `https://api.deepseek.com/user/balance` | `https://api.z.ai/api/monitor/usage/quota/limit` (base overridable) | `https://opencode.ai/workspace/{workspace_id}/go` | `https://ark.cn-beijing.volcengineapi.com/?Action=GetAFPUsage&Version=2024-01-01` |
| Method | GET | GET | GET (dashboard scrape) | POST（Body `{}`） |
| Auth header | `Authorization: Bearer <key>` | `Authorization: Bearer <key>` | Cookie `auth=<value>` (value only) + browser-like User-Agent; redirects not followed | **HMAC-SHA256 V4 签名**（AK/SK，service=`ark`，region=`cn-beijing`） |
| Response size cap | 64 KiB | (per client impl) | 2 MiB | 1 MiB |
| Timeout | 30 s (request + transport) | 30 s | 10 s scrape timeout | 30 s (follow existing pattern) |

### 3.3 Request / response format requirements

- All requests send `Accept: application/json` (dashboard scrape: browser User-Agent, cookie auth).
- Responses must be parsed into typed structs; unmarshal failure is a recoverable provider error (`error`), never a crash.
- Adapter output must map to the shared snapshot model consumed by the dashboard (quota windows with used/limit/percent/reset, balance with amount/currency, etc.).
- `ark` response parsing: define types in `internal/api/ark_types.go`; map official quota fields → `used`/`limit`/`percent`/`reset_at`.

### 3.4 Error handling and fallback

| Condition | Handling required |
|---|---|
| 401 / 403 | Mark snapshot `auth_failed`, surface clear reason in UI; do not retry before next poll cycle. |
| 429 / rate limit | Mark `error` (rate_limited); respect `Retry-After` if provided; back off to next scheduled poll. |
| 5xx / network / timeout / parse failure | Mark `error`; keep last good snapshot visible (stale-while-revalidate). |
| Empty / oversized response | Treat as `invalid response` error; bounded read (see §3.2 caps) to protect memory. |
| Missing credential | `unconfigured`; agent idle (no requests sent). |
| Existing last-good data | Preserved in DB; dashboard keeps showing it with a timestamp rather than blanking out. |

### 3.5 Performance benchmarks and timeouts

| Metric | Target |
|---|---|
| Daemon memory (all four adapters enabled) | Stays within Phase-1 NFR-2 budget < 60 MB sustained 24 h |
| Poll interval | Follows `ONWATCH_POLL_INTERVAL` (default 120 s; min 10 s, max 3600 s); Ark adapter must respect the same global value and not add its own aggressive schedule |
| Per-poll latency | Individual provider request completes within its timeout (30 s max), i.e. a dead upstream never blocks the poll loop (per-source goroutines or ctx deadlines) |
| Response size | Bounded reads enforced (see caps in §3.2) — no unbounded allocation |
| Request rate | One request per provider per poll cycle; no burst loops; reused HTTP client with connection pooling |

## 4. Verification Criteria

> Prerequisite for real-API steps: valid credentials for each platform are provided by the user (DeepSeek key, 智谱 key + cn region, OpenCode workspace/cookie, Ark key).

### 4.1 Step-by-step validation process

1. **Build & unit tests**: `go build -o onwatch.exe .` and `go test ./...` pass (Ark + generic suites included).
2. **Start daemon**: run `onwatch --debug`; confirm log shows agents started: `deepseek`, `ark`, `zai`, `opencode`.
3. **Configure credentials**: populate the four credential blocks in `~/.onwatch/.env` (per Deliverable D3); restart daemon.
4. **Observe per-platform status**: open dashboard; each card shows `ok` with real data within `ONWATCH_POLL_INTERVAL`.
5. **Verify persistence & history**: snapshots persist in SQLite; `/api/history` returns rows.
6. **Negative-path suite**: execute §4.2 / §4.3 cases; confirm status transitions and no panic/no credential leakage.
7. **Performance check**: sample daemon memory over the run; verify < 60 MB and per-poll ≤ 30 s.
8. **Summarize**: fill Deliverable D4 report; tick roadmap acceptance boxes.

### 4.2 Test cases — successful authentication (valid credentials)

| TC | Action | Expected result |
|---|---|---|
| TC-01 DeepSeek | Set valid `DEEPSEEK_API_KEY`, wait ≤ 1 poll interval | Card shows balance (e.g. `$12.50 USD`) + availability, status `ok` |
| TC-02 智谱 GLM | Set valid `ZAI_API_KEY` + `ZAI_REGION=cn` | Card shows quota used/limit, status `ok` |
| TC-03 OpenCode | Set valid workspace ID + auth cookie | Card shows OpenCode Go quota (currency or percent format + reset time), status `ok` |
| TC-04 火山 Ark | Set valid `ARK_ACCESS_KEY` + `ARK_SECRET_KEY` | Card shows Ark quota, status `ok` |
| TC-05 All four | All credentials valid simultaneously | All four cards `ok`, daemon stable, no inter-provider interference |

### 4.3 Test cases — error handling (invalid credentials)

| TC | Action | Expected result |
|---|---|---|
| TC-06 Missing key | Unset all four credentials | Each card `unconfigured`; no requests sent; no error spam |
| TC-07 Invalid DeepSeek key | Wrong key value | `auth_failed`; clear message; next poll retries; no panic |
| TC-08 Invalid 智谱 key | Wrong key / wrong region | `auth_failed` with provider-specific message |
| TC-09 Expired/revoked cookie | OpenCode cookie cleared in browser | `auth_failed`; no noisy stack logs |
| TC-10 Invalid Ark key | Wrong key | `auth_failed` (and `unconfigured` if feature toggles treat unset separately) |
| TC-11 Upstream 500 / timeout | Mock server returns 500 / hangs | `error`; last-good snapshot still displayed; poll loop not blocked |
| TC-12 Log hygiene | Run all above | No full credential string appears anywhere in `.onwatch.log` |

### 4.4 End-to-end functionality scenarios

| Scenario | Steps | Pass criteria |
|---|---|---|
| E2E-1 Fresh start | Empty `.env` → start → dashboard | All cards `unconfigured`, panel renders normally |
| E2E-2 Configure → data | Fill DeepSeek + Ark keys → restart → wait 2 intervals | Both cards show real data, `ok`, with reset info where applicable |
| E2E-3 Credential rotation | Replace key → restart | New data, old status history retained |
| E2E-4 Extended soak | Run ≥ 24 h with all four enabled (Phase-1 NFR-2 follow-up) | Memory stable < 60 MB; no goroutine leak; per-poll latency bounded |

## 5. Deliverables

| ID | Deliverable | Owner point-of-contact in plan | Notes |
|---|---|---|---|
| D1 | 火山 Ark adapter source code (api/agent/tracker/store/web + tests) | P2-09 | Complete integration into AgentManager + dashboard card + route |
| D2 | Verification documentation updated for all four adapters | P2-10/11/12 | Result tables for TC-01…TC-12 with evidence (screenshots/log excerpts) |
| D3 | Credential configuration guide | P2-13 / docs | Exact `.env` variables, where to obtain each credential, redaction & security notes |
| D4 | Test results & validation report | P2-16 | Executed test matrix, memory/perf measurements, open items, roadmap tick updates |
| D5 | 火山 Ark interface research note (pre-implementation) | P2-09 phase 1 | Endpoint, auth, response schema, rate limits — approved before coding |

## 6. Dependencies and Constraints

### 6.1 API rate limits and usage quotas (per provider)

- **DeepSeek / 智谱 / Ark**: official APIs may apply per-key rate limits; adapters must treat 429 as a non-fatal, back-off-able error (§3.4) and must not exceed the global poll interval frequency.
- **OpenCode dashboard**: scraping is a browser-session flow — respect it as low-frequency (default 120 s interval is fine); handle redirects/anti-scrape pages as `auth_failed` / `error` rather than hammering.
- **火山 Ark**: rate-limit policy to be recorded in D5 during interface research and reflected in the client (Retry-After handling).

### 6.2 Compatibility requirements

- Go 1.25.7 module (`go.mod`), build via `go build -o onwatch.exe .` on Windows; cross-compile targets macOS/Linux (Phase-1 NFR-1 follow-up) must not rely on Windows-only code — keep path handling via `filepath` / OS-agnostic helpers (existing pattern in codebase).
- Ark adapter must register with the same `AgentManager` API used by built-ins (no fork-specific entry points).
- Dashboard card must reuse the existing per-provider card/status rendering; no layout changes outside the adapter's own card template.
- Windows-first runtime (local user dirs via `platform_windows.go` equivalents); login-state file reads must honor `OPENCODE_HOME`/XDG conventions on other OSes.

### 6.3 Security requirements for credential storage

- Credentials only in `~/.onwatch/.env` (mode-restricted) or read-only local auth files (NFR-3).
- DB stores metric snapshots only — never credentials, never plaintext keys (P2-13/P2-14 hardening also covers generic adapter input).
- Logs redact credential material; sensitive responses (tokens/cookies) never logged (see §3.4 TC-12).
- No telemetry / external upload; all data local (architecture §8).

## 7. Acceptance Criteria

Phase 2 / FR-1.1 is accepted when **all** of the following hold:

1. **All four adapters connect**: with valid credentials, DeepSeek, 火山 Ark, 智谱 GLM, and OpenCode each show real usage data on the Web panel (status `ok`). Real-API validation performed at least once per platform.
2. **Response time**: every provider poll completes within its timeout (≤ 30 s; OpenCode scrape ≤ 10 s) and does not block the polling loop; dashboard/API remain responsive.
3. **Error handling & logging**: all negative cases in §4.3 behave per §3.4 (correct status, preserved last-good data, no panics) and no credential material is logged.
4. **100% test pass**: `go test ./...` green; Ark suite (client/tracker/store) and regression suites pass; verification matrix TC-01…TC-12 executes with 100% pass rate for all cases run (cases skipped for lack of user credentials are explicitly listed in D4, not treated as failed).
5. **Documentation**: D1–D5 complete and linked from [phase2-plan.md](phase2-plan.md) / roadmap.md.

---

### Appendix A — current codebase facts (verified 2026-08-27)

| Fact | Reference |
|---|---|
| DeepSeek client: base `https://api.deepseek.com`, `GET /user/balance`, Bearer auth, 30 s timeout, 64 KiB cap, redacted logging | `server/internal/api/deepseek_client.go` |
| Z.ai client: base `https://api.z.ai/api/monitor/usage/quota/limit`, 30 s timeout, redacted logging | `server/internal/api/zai_client.go` |
| OpenCode: dashboard scrape `https://opencode.ai/workspace/{id}/go`, cookie auth, 10 s timeout, 2 MiB cap, redirects not followed; `~/.local/share/opencode/auth.json` feeds Codex | `server/internal/api/opencode_client.go`, `server/internal/api/opencode.go` |
| Credential env vars & defaults | `server/config/app/config.go`, `server/.env.example` |
| Adapter registration contract | `server/internal/agent/manager.go` (`RegisterFactory`, `AgentRunner`) |
| 火山 Ark | not present — new implementation (this plan) |