package api

import (
	"encoding/base64"
	"strconv"
	"testing"
	"time"
)

// makeJWT 构造一个带指定 exp 的 JWT（payload 为 base64url 编码的 JSON）
func makeJWT(exp int64) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"exp":` + strconv.FormatInt(exp, 10) + `,"iat":` + strconv.FormatInt(exp-3600, 10) + `}`))
	return header + "." + payload + ".signature"
}

func TestParseCookieExpiry(t *testing.T) {
	now := time.Now().Unix()
	digestExp := now + 2*86400 // 2 天后
	userExp := now + 30*86400  // 30 天后

	cookie := "digest=" + makeJWT(digestExp) + "; userInfo=" + makeJWT(userExp) + "; csrfToken=abc"

	expiries, err := ParseCookieExpiry(cookie)
	if err != nil {
		t.Fatalf("ParseCookieExpiry failed: %v", err)
	}
	if len(expiries) != 2 {
		t.Fatalf("expiries count = %d, want 2", len(expiries))
	}
	// digest 应最先过期
	earliest, err := EarliestCookieExpiry(cookie)
	if err != nil {
		t.Fatalf("EarliestCookieExpiry failed: %v", err)
	}
	if earliest.Unix() != digestExp {
		t.Errorf("earliest expiry = %d, want %d", earliest.Unix(), digestExp)
	}
}

func TestCookieDaysRemaining(t *testing.T) {
	now := time.Now().Unix()

	// 2 天后过期
	days, err := CookieDaysRemaining("digest=" + makeJWT(now+2*86400))
	if err != nil {
		t.Fatalf("CookieDaysRemaining failed: %v", err)
	}
	if days < 1.9 || days > 2.1 {
		t.Errorf("days remaining = %v, want ~2", days)
	}

	// 已过期
	days, err = CookieDaysRemaining("digest=" + makeJWT(now-3600))
	if err != nil {
		t.Fatalf("CookieDaysRemaining failed: %v", err)
	}
	if days >= 0 {
		t.Errorf("days remaining for expired = %v, want negative", days)
	}

	// 无 JWT
	if _, err := CookieDaysRemaining("session=abc"); err == nil {
		t.Error("expected error for cookie without JWT")
	}
}

func TestParseCookiePairs(t *testing.T) {
	pairs := parseCookiePairs("a=1; b=2; c=3")
	if pairs["a"] != "1" || pairs["b"] != "2" || pairs["c"] != "3" {
		t.Errorf("parseCookiePairs = %v", pairs)
	}
}