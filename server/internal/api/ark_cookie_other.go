//go:build !windows

package api

import (
	"fmt"
	"log/slog"
)

// BrowserCookieExtractor 在非 Windows 平台暂不支持浏览器 Cookie 提取。
type BrowserCookieExtractor struct {
	logger *slog.Logger
}

// NewBrowserCookieExtractor 创建浏览器 Cookie 提取器（非 Windows 返回空实现）。
func NewBrowserCookieExtractor(logger *slog.Logger) *BrowserCookieExtractor {
	if logger == nil {
		logger = slog.Default()
	}
	return &BrowserCookieExtractor{logger: logger}
}

// Extract 非 Windows 平台暂不支持，返回错误。
func (e *BrowserCookieExtractor) Extract() (string, error) {
	return "", fmt.Errorf("浏览器 Cookie 提取暂仅支持 Windows（macOS/Linux 请手动配置 ARK_CONSOLE_COOKIE）")
}