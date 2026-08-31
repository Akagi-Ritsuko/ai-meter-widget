package api

import (
	"fmt"
	"log/slog"
)

// CompositeCookieExtractor 依次尝试多个 Cookie 提取器，返回第一个成功的。
// 顺序：CDP（浏览器运行时）→ 浏览器 DB（浏览器关闭时）。
type CompositeCookieExtractor struct {
	extractors []CookieExtractor
	logger     *slog.Logger
}

// NewCompositeCookieExtractor 创建组合提取器。
func NewCompositeCookieExtractor(extractors ...CookieExtractor) *CompositeCookieExtractor {
	return &CompositeCookieExtractor{extractors: extractors}
}

// Extract 依次尝试各提取器，返回第一个成功的 Cookie。
func (c *CompositeCookieExtractor) Extract() (string, error) {
	var lastErr error
	for _, e := range c.extractors {
		cookie, err := e.Extract()
		if err == nil && cookie != "" {
			return cookie, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("无可用 Cookie 提取器")
	}
	return "", lastErr
}