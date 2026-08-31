package api

import (
	"strings"
	"testing"
)

// TestBrowserCookieExtractManual 手动验证浏览器 Cookie 提取（依赖本机浏览器状态）
func TestBrowserCookieExtractManual(t *testing.T) {
	e := NewBrowserCookieExtractor(nil)
	cookie, err := e.Extract()
	if err != nil {
		t.Skipf("浏览器 Cookie 提取失败（可能未登录控制台）: %v", err)
	}
	if cookie == "" {
		t.Skip("提取到空 Cookie")
	}
	// 校验关键字段
	for _, key := range []string{"digest", "userInfo", "csrfToken"} {
		if !strings.Contains(cookie, key+"=") {
			t.Errorf("提取的 Cookie 缺少 %s", key)
		}
	}
	// 校验过期时间
	days, err := CookieDaysRemaining(cookie)
	if err != nil {
		t.Errorf("CookieDaysRemaining failed: %v", err)
	} else {
		t.Logf("浏览器 Cookie 提取成功，剩余 %.1f 天", days)
	}
}