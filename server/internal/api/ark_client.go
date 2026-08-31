// Package api provides clients for interacting with the Volcano Engine Ark API.
package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Custom errors for Volcano Ark API failures.
var (
	ErrArkUnauthorized    = errors.New("ark: unauthorized - invalid access key or signature")
	ErrArkServerError     = errors.New("ark: server error")
	ErrArkNetworkError    = errors.New("ark: network error")
	ErrArkInvalidResponse = errors.New("ark: invalid response")
	ErrArkAPIError        = errors.New("ark: API returned error")
	ErrArkRateLimited     = errors.New("ark: rate limited")
)

const (
	arkDefaultBaseURL = "https://ark.cn-beijing.volcengineapi.com/"
	arkDefaultRegion  = "cn-beijing"
	arkService        = "ark"
	arkAction         = "GetAFPUsage"
	arkAPIVersion     = "2024-01-01"
	arkRequestBody    = "{}"
	arkTimestampFmt   = "20060102T150405Z"
	arkDateStampFmt   = "20060102"
	arkMaxBodyBytes   = 1 << 20 // 1 MiB response cap (FR-1.1 §3.2)
	arkTimeout        = 30 * time.Second
)

// ArkClient is an HTTP client for the Volcano Engine Ark GetAFPUsage API.
// Authentication uses AccessKey/SecretKey pairs with an HMAC-SHA256 V4
// signature (Volcengine's SigV4-compatible scheme).
type ArkClient struct {
	httpClient *http.Client
	accessKey  string
	secretKey  string
	baseURL    string
	region     string
	logger     *slog.Logger
	now        func() time.Time
}

// ArkClientOption configures an ArkClient.
type ArkClientOption func(*ArkClient)

// WithArkBaseURL sets a custom base URL (for testing or proxies).
// 仅当传入非空值时覆盖默认值，避免空字符串清空默认端点。
func WithArkBaseURL(baseURL string) ArkClientOption {
	return func(c *ArkClient) {
		baseURL = strings.TrimSpace(baseURL)
		if baseURL != "" {
			c.baseURL = strings.TrimRight(baseURL, "/")
		}
	}
}

// WithArkTimeout sets a custom timeout (for testing).
func WithArkTimeout(timeout time.Duration) ArkClientOption {
	return func(c *ArkClient) {
		c.httpClient.Timeout = timeout
	}
}

// WithArkClock injects a clock source so tests can produce a deterministic
// signature (golden verification).
func WithArkClock(now func() time.Time) ArkClientOption {
	return func(c *ArkClient) {
		c.now = now
	}
}

// NewArkClient creates a new Volcano Ark GetAFPUsage API client.
func NewArkClient(accessKey, secretKey string, logger *slog.Logger, opts ...ArkClientOption) *ArkClient {
	if logger == nil {
		logger = slog.Default()
	}
	c := &ArkClient{
		httpClient: &http.Client{
			Timeout: arkTimeout,
			Transport: &http.Transport{
				MaxIdleConns:          1,
				MaxIdleConnsPerHost:   1,
				ResponseHeaderTimeout: arkTimeout,
				IdleConnTimeout:       30 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ForceAttemptHTTP2:     true,
			},
		},
		accessKey: accessKey,
		secretKey: secretKey,
		baseURL:   strings.TrimRight(arkDefaultBaseURL, "/"),
		region:    arkDefaultRegion,
		logger:    logger,
		now:       time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// FetchUsage retrieves the Agent Plan quota snapshot (four rolling windows)
// from the GetAFPUsage API.
func (c *ArkClient) FetchUsage(ctx context.Context) (*ArkSnapshot, error) {
	now := c.now().UTC()
	payloadHash := arkSHA256Hex([]byte(arkRequestBody))
	amzDate := now.Format(arkTimestampFmt)
	dateStamp := now.Format(arkDateStampFmt)

	endpoint, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("%w: parse base url: %v", ErrArkNetworkError, err)
	}
	q := endpoint.Query()
	q.Set("Action", arkAction)
	q.Set("Version", arkAPIVersion)
	endpoint.RawQuery = q.Encode()
	host := endpoint.Hostname()

	canonicalRequest := "POST\n/\n" + endpoint.RawQuery + "\n" +
		"content-type:application/json\n" +
		"host:" + host + "\n" +
		"x-content-sha256:" + payloadHash + "\n" +
		"x-date:" + amzDate + "\n" +
		"\n" +
		"content-type;host;x-content-sha256;x-date\n" +
		payloadHash

	stringToSign := "HMAC-SHA256\n" + amzDate + "\n" +
		dateStamp + "/" + c.region + "/" + arkService + "/request\n" +
		arkSHA256Hex([]byte(canonicalRequest))

	signature := arkSign(stringToSign, c.secretKey, dateStamp, c.region)

	authorization := "HMAC-SHA256 Credential=" + c.accessKey + "/" + dateStamp + "/" + c.region + "/" + arkService + "/request" +
		", SignedHeaders=content-type;host;x-content-sha256;x-date" +
		", Signature=" + signature

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(arkRequestBody))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrArkNetworkError, err)
	}
	req.Host = host
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Date", amzDate)
	req.Header.Set("X-Content-Sha256", payloadHash)
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Accept", "application/json")

	c.logger.Debug("fetching Ark AFP usage",
		"url", endpoint.String(),
		"region", c.region,
		"access_key", redactArkAccessKey(c.accessKey),
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", ErrArkNetworkError, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, arkMaxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrArkInvalidResponse, err)
	}
	if len(body) > arkMaxBodyBytes {
		return nil, fmt.Errorf("%w: response body exceeds %d bytes", ErrArkInvalidResponse, arkMaxBodyBytes)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		// Parse below.
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrArkUnauthorized
	case http.StatusTooManyRequests:
		c.logger.Debug("Ark API rate limited",
			"status", resp.StatusCode,
			"retry_after", resp.Header.Get("Retry-After"),
		)
		return nil, ErrArkRateLimited
	default:
		if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("%w: http %d", ErrArkServerError, resp.StatusCode)
		}
		return nil, fmt.Errorf("%w: http %d: %s", ErrArkInvalidResponse, resp.StatusCode, sanitizeArkMessage(string(body)))
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("%w: empty response body", ErrArkInvalidResponse)
	}

	var parsed ArkGetAFPUsageResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrArkInvalidResponse, err)
	}

	if parsed.ResponseMetadata.Error != nil {
		arkErr := parsed.ResponseMetadata.Error
		if arkIsAuthErrorCode(arkErr.Code) {
			return nil, fmt.Errorf("%w: %s: %s", ErrArkUnauthorized, arkErr.Code, arkErr.Message)
		}
		return nil, fmt.Errorf("%w: code=%s, message=%s", ErrArkAPIError, arkErr.Code, arkErr.Message)
	}

	snap := parsed.ToSnapshot(now)
	snap.RawJSON = string(body)
	return snap, nil
}

// arkSign derives the SigV4 signing key chain and returns the hex signature.
func arkSign(stringToSign, secretKey, dateStamp, region string) string {
	kDate := arkHMAC([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := arkHMAC(kDate, []byte(region))
	kService := arkHMAC(kRegion, []byte(arkService))
	kSigning := arkHMAC(kService, []byte("request"))
	return hex.EncodeToString(arkHMAC(kSigning, []byte(stringToSign)))
}

func arkHMAC(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func arkSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// arkIsAuthErrorCode reports whether a Volcengine error code indicates an
// authentication/signature failure.
func arkIsAuthErrorCode(code string) bool {
	upper := strings.ToUpper(code)
	for _, needle := range []string{"SIGNATURE", "INVALIDACCESSKEY", "ACCESSDENIED", "FORBIDDEN", "UNAUTHORIZED", "AUTH"} {
		if strings.Contains(upper, needle) {
			return true
		}
	}
	return false
}

func sanitizeArkMessage(text string) string {
	s := strings.TrimSpace(text)
	if s == "" {
		return "unknown"
	}
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

// redactArkAccessKey masks the access key for logging (never log SecretKey or
// response bodies).
func redactArkAccessKey(key string) string {
	if key == "" {
		return "(empty)"
	}
	if len(key) < 8 {
		return "***...***"
	}
	return key[:4] + "***...***" + key[len(key)-3:]
}

// IsArkAuthError reports whether err is an authentication failure that should
// surface as auth_failed in the panel.
func IsArkAuthError(err error) bool {
	return errors.Is(err, ErrArkUnauthorized)
}