package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// CookieTokenExpiry 表示 Cookie 中某个 JWT 凭证的过期信息。
type CookieTokenExpiry struct {
	Name    string    // digest | userInfo
	Expires time.Time // 过期时间
}

// ParseCookieExpiry 解析火山控制台 Cookie 中 JWT 凭证的过期时间。
// 返回所有可解析的凭证过期时间；无任何可解析凭证时返回错误。
func ParseCookieExpiry(cookie string) ([]CookieTokenExpiry, error) {
	values := parseCookiePairs(cookie)
	var result []CookieTokenExpiry

	for _, name := range []string{"digest", "userInfo"} {
		val, ok := values[name]
		if !ok || val == "" {
			continue
		}
		exp, err := parseJWTExp(val)
		if err != nil {
			continue // 单个凭证解析失败不阻塞整体
		}
		result = append(result, CookieTokenExpiry{Name: name, Expires: exp})
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("cookie 中未找到可解析的 JWT 凭证（digest/userInfo）")
	}
	return result, nil
}

// EarliestCookieExpiry 返回 Cookie 中最早过期的凭证时间。
func EarliestCookieExpiry(cookie string) (time.Time, error) {
	expiries, err := ParseCookieExpiry(cookie)
	if err != nil {
		return time.Time{}, err
	}
	earliest := expiries[0].Expires
	for _, e := range expiries[1:] {
		if e.Expires.Before(earliest) {
			earliest = e.Expires
		}
	}
	return earliest, nil
}

// CookieDaysRemaining 返回 Cookie 剩余有效天数（按最早过期凭证计算）。
// 返回负数表示已过期。
func CookieDaysRemaining(cookie string) (float64, error) {
	exp, err := EarliestCookieExpiry(cookie)
	if err != nil {
		return 0, err
	}
	return time.Until(exp).Hours() / 24, nil
}

// parseCookiePairs 将 Cookie 字符串解析为 name->value 映射。
func parseCookiePairs(cookie string) map[string]string {
	result := make(map[string]string)
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.Index(part, "=")
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(part[:idx])
		value := strings.TrimSpace(part[idx+1:])
		result[name] = value
	}
	return result
}

// parseJWTExp 解析 JWT 的 exp 声明（payload 为 base64url 编码的 JSON）。
func parseJWTExp(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, fmt.Errorf("无效的 JWT 格式")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// 部分 JWT 使用标准 base64 填充
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return time.Time{}, fmt.Errorf("JWT payload 解码失败: %w", err)
		}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, fmt.Errorf("JWT claims 解析失败: %w", err)
	}
	if claims.Exp == 0 {
		return time.Time{}, fmt.Errorf("JWT 无 exp 声明")
	}
	return time.Unix(claims.Exp, 0), nil
}