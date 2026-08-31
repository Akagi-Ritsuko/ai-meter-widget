package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Coding Plan 相关错误
var (
	ErrArkCodingPlanUnauthorized    = errors.New("ark coding plan: unauthorized - cookie expired or invalid")
	ErrArkCodingPlanServerError     = errors.New("ark coding plan: server error")
	ErrArkCodingPlanNetworkError    = errors.New("ark coding plan: network error")
	ErrArkCodingPlanInvalidResponse = errors.New("ark coding plan: invalid response")
	ErrArkCodingPlanAPIError        = errors.New("ark coding plan: API returned error")
)

const (
	arkCodingPlanURL      = "https://console.volcengine.com/api/top/ark/cn-beijing/2024-01-01/GetCodingPlanUsage?"
	arkCodingPlanReferer  = "https://console.volcengine.com/ark/region:cn-beijing/subscription/coding-plan"
	arkCodingPlanOrigin   = "https://console.volcengine.com"
	arkCodingPlanMaxBytes = 1 << 20 // 1 MiB
	arkCodingPlanTimeout  = 15 * time.Second
)

// ArkCodingPlanQuota 是 GetCodingPlanUsage 返回的单个配额窗口。
// Coding Plan 为百分比制：Percent 为已用百分比，Cap 为上限（通常 100）。
type ArkCodingPlanQuota struct {
	Level              string  `json:"Level"` // session | weekly | monthly
	Percent            float64 `json:"Percent"`
	ResetTimestamp     int64   `json:"ResetTimestamp"` // epoch 秒，-1 表示无重置
	Cap                float64 `json:"Cap"`
	RewardTotalPercent float64 `json:"RewardTotalPercent"`
}

// ArkCodingPlanResult 是 GetCodingPlanUsage 的 Result 部分。
type ArkCodingPlanResult struct {
	Status          string               `json:"Status"`
	UpdateTimestamp int64                `json:"UpdateTimestamp"`
	QuotaUsage      []ArkCodingPlanQuota `json:"QuotaUsage"`
	HasReward       bool                 `json:"HasReward"`
}

// ArkCodingPlanResponse 是 GetCodingPlanUsage 的完整响应。
type ArkCodingPlanResponse struct {
	ResponseMetadata ArkResponseMetadata `json:"ResponseMetadata"`
	Result           ArkCodingPlanResult `json:"Result"`
}

// CookieExtractor 是 Cookie 自动刷新提取器接口。
// 实现：BrowserCookieExtractor（浏览器 DB，浏览器关闭时可用）、CDPCookieExtractor（CDP，浏览器运行时可用）。
type CookieExtractor interface {
	Extract() (string, error)
}

// ArkCodingPlanClient 是火山方舟 Coding Plan 用量查询客户端。
// 鉴权方式为控制台浏览器 Cookie + x-csrf-token + x-web-id（内部接口，非 OpenAPI）。
// 支持 Cookie 自动提取刷新（CDP / 浏览器 DB）。
type ArkCodingPlanClient struct {
	httpClient     *http.Client
	cookie         string
	csrfToken      string
	webID          string
	logger         *slog.Logger
	extractor      CookieExtractor
	lastAuthFailed bool
}

// NewArkCodingPlanClient 创建 Coding Plan 客户端。
// cookie 为控制台 Cookie（含 csrfToken）；csrfToken 可显式传入，为空时从 Cookie 提取。
func NewArkCodingPlanClient(cookie, csrfToken, webID string, logger *slog.Logger) *ArkCodingPlanClient {
	if logger == nil {
		logger = slog.Default()
	}
	if csrfToken == "" {
		csrfToken = extractCSRFToken(cookie)
	}
	return &ArkCodingPlanClient{
		httpClient: &http.Client{
			Timeout: arkCodingPlanTimeout,
			Transport: &http.Transport{
				MaxIdleConns:          1,
				MaxIdleConnsPerHost:   1,
				ResponseHeaderTimeout: arkCodingPlanTimeout,
				IdleConnTimeout:       30 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ForceAttemptHTTP2:     true,
			},
		},
		cookie:    cookie,
		csrfToken: csrfToken,
		webID:     webID,
		logger:    logger,
	}
}

// SetCookieExtractor 设置 Cookie 提取器，启用自动刷新。
func (c *ArkCodingPlanClient) SetCookieExtractor(e CookieExtractor) {
	c.extractor = e
}

// CookieDaysRemaining 返回当前 Cookie 剩余有效天数（-1 表示无法解析）。
func (c *ArkCodingPlanClient) CookieDaysRemaining() float64 {
	days, err := CookieDaysRemaining(c.cookie)
	if err != nil {
		return -1
	}
	return days
}

// refreshCookieIfNeeded 在 Cookie 快过期或上次认证失败时，尝试从浏览器提取新 Cookie。
func (c *ArkCodingPlanClient) refreshCookieIfNeeded() {
	if c.extractor == nil {
		return
	}

	days, err := CookieDaysRemaining(c.cookie)
	needsRefresh := c.lastAuthFailed || err != nil || days < 1.0

	if !needsRefresh {
		return
	}

	newCookie, extractErr := c.extractor.Extract()
	if extractErr != nil {
		c.logger.Warn("Ark Coding Plan Cookie 自动刷新失败，请手动更新 ARK_CONSOLE_COOKIE",
			"error", extractErr)
		return
	}

	c.cookie = newCookie
	c.csrfToken = extractCSRFToken(newCookie)
	c.lastAuthFailed = false
	c.logger.Info("Ark Coding Plan Cookie 已从浏览器自动刷新")
}

// extractCSRFToken 从 Cookie 中提取 csrfToken 值。
func extractCSRFToken(cookie string) string {
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "csrfToken=") {
			return strings.TrimPrefix(part, "csrfToken=")
		}
	}
	return ""
}

// FetchUsage 查询 Coding Plan 用量。
func (c *ArkCodingPlanClient) FetchUsage(ctx context.Context) (*ArkCodingPlanResponse, error) {
	// Cookie 快过期或上次认证失败时，尝试浏览器自动刷新
	c.refreshCookieIfNeeded()

	reqCtx, cancel := context.WithTimeout(ctx, arkCodingPlanTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, arkCodingPlanURL, strings.NewReader("{}"))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrArkCodingPlanNetworkError, err)
	}

	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", arkCodingPlanOrigin)
	req.Header.Set("Referer", arkCodingPlanReferer)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36 Edg/151.0.0.0")
	req.Header.Set("Cookie", c.cookie)
	if c.csrfToken != "" {
		req.Header.Set("X-Csrf-Token", c.csrfToken)
	}
	if c.webID != "" {
		req.Header.Set("X-Web-Id", c.webID)
	}

	c.logger.Debug("fetching Ark Coding Plan usage",
		"url", arkCodingPlanURL,
		"has_cookie", c.cookie != "",
		"has_csrf", c.csrfToken != "",
		"has_web_id", c.webID != "",
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", ErrArkCodingPlanNetworkError, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, arkCodingPlanMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrArkCodingPlanInvalidResponse, err)
	}
	if len(body) > arkCodingPlanMaxBytes {
		return nil, fmt.Errorf("%w: response exceeds %d bytes", ErrArkCodingPlanInvalidResponse, arkCodingPlanMaxBytes)
	}

	switch {
	case resp.StatusCode == http.StatusOK:
		// continue
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		c.lastAuthFailed = true
		return nil, ErrArkCodingPlanUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, fmt.Errorf("%w: status %d", ErrArkCodingPlanServerError, resp.StatusCode)
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("%w: status %d", ErrArkCodingPlanServerError, resp.StatusCode)
	default:
		return nil, fmt.Errorf("%w: unexpected status %d", ErrArkCodingPlanInvalidResponse, resp.StatusCode)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("%w: empty response body", ErrArkCodingPlanInvalidResponse)
	}

	var parsed ArkCodingPlanResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%w: parse response: %v", ErrArkCodingPlanInvalidResponse, err)
	}

	if parsed.ResponseMetadata.Error != nil {
		code := parsed.ResponseMetadata.Error.Code
		if strings.Contains(code, "Signature") || strings.Contains(code, "InvalidAccessKey") ||
			strings.Contains(code, "AccessDenied") || strings.Contains(code, "Forbidden") ||
			strings.Contains(code, "Unauthorized") {
			c.lastAuthFailed = true
			return nil, ErrArkCodingPlanUnauthorized
		}
		return nil, fmt.Errorf("%w: %s: %s", ErrArkCodingPlanAPIError, code, parsed.ResponseMetadata.Error.Message)
	}

	c.lastAuthFailed = false
	return &parsed, nil
}

// ToSnapshot 将 Coding Plan 响应归一化为 ArkSnapshot（与 Agent Plan 共用存储结构）。
// 窗口名使用 cp_ 前缀区分：cp_session / cp_weekly / cp_monthly。
// 百分比制：Quota=Cap，Used=Percent，Percent=Percent。
func (r *ArkCodingPlanResponse) ToSnapshot(now time.Time) *ArkSnapshot {
	snap := &ArkSnapshot{
		CapturedAt: now,
		PlanType:   "coding_plan",
	}
	for _, q := range r.Result.QuotaUsage {
		ws := ArkWindowSnapshot{
			Name:    "cp_" + q.Level,
			Quota:   q.Cap,
			Used:    q.Percent,
			Percent: q.Percent,
		}
		if q.ResetTimestamp > 0 {
			t := time.Unix(q.ResetTimestamp, 0)
			ws.ResetsAt = &t
		}
		snap.Windows = append(snap.Windows, ws)
	}
	return snap
}