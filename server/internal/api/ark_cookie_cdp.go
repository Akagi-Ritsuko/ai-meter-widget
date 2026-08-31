package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// CDPCookieExtractor 通过 Chrome DevTools Protocol 从运行中的浏览器提取火山控制台 Cookie。
// 需要浏览器以 --remote-debugging-port=9222 启动（Edge/Chrome 均可）。
type CDPCookieExtractor struct {
	debugURL string
	logger   *slog.Logger
	client   *http.Client
}

// NewCDPCookieExtractor 创建 CDP Cookie 提取器。
// debugURL 为浏览器调试端口地址，默认 http://localhost:9222。
func NewCDPCookieExtractor(debugURL string, logger *slog.Logger) *CDPCookieExtractor {
	if logger == nil {
		logger = slog.Default()
	}
	if debugURL == "" {
		debugURL = "http://localhost:9222"
	}
	return &CDPCookieExtractor{
		debugURL: strings.TrimRight(debugURL, "/"),
		logger:   logger,
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

// cdpTarget 是 CDP /json 返回的页面 target。
type cdpTarget struct {
	Type                string `json:"type"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// cdpCookie 是 CDP Network.getAllCookies 返回的单个 Cookie。
type cdpCookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
}

// cdpResponse 是 CDP 命令响应。
type cdpResponse struct {
	ID     int64 `json:"id"`
	Result struct {
		Cookies []cdpCookie `json:"cookies"`
	} `json:"result"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Extract 从运行中的浏览器提取 console.volcengine.com 的 Cookie。
func (e *CDPCookieExtractor) Extract() (string, error) {
	// 1. 获取页面 target
	targets, err := e.listTargets()
	if err != nil {
		return "", err
	}
	if len(targets) == 0 {
		return "", fmt.Errorf("CDP: 未找到页面 target（浏览器调试端口未开启？）")
	}

	// 2. 连接第一个页面的 WebSocket
	wsURL := targets[0].WebSocketDebuggerURL
	if wsURL == "" {
		return "", fmt.Errorf("CDP: target 无 WebSocket 地址")
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return "", fmt.Errorf("CDP: WebSocket 连接失败: %w", err)
	}
	defer conn.Close()

	// 3. 发送 Network.getAllCookies
	cmd := map[string]interface{}{
		"id":     1,
		"method": "Network.getAllCookies",
		"params": map[string]interface{}{},
	}
	if err := conn.WriteJSON(cmd); err != nil {
		return "", fmt.Errorf("CDP: 发送命令失败: %w", err)
	}

	// 4. 读取响应
	var resp cdpResponse
	if err := conn.ReadJSON(&resp); err != nil {
		return "", fmt.Errorf("CDP: 读取响应失败: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("CDP: 命令错误: %s", resp.Error.Message)
	}

	// 5. 过滤 volcengine.com 相关 Cookie（含父域 .volcengine.com，digest/userInfo/csrfToken 存于此）
	var parts []string
	for _, c := range resp.Result.Cookies {
		if strings.Contains(c.Domain, "volcengine.com") {
			parts = append(parts, c.Name+"="+c.Value)
		}
	}

	if len(parts) == 0 {
		return "", fmt.Errorf("CDP: 浏览器中未找到 console.volcengine.com 的 Cookie（请先登录控制台）")
	}

	e.logger.Info("CDP cookie extracted", "count", len(parts))
	return strings.Join(parts, "; "), nil
}

// listTargets 获取浏览器页面 target 列表。
func (e *CDPCookieExtractor) listTargets() ([]cdpTarget, error) {
	resp, err := e.client.Get(e.debugURL + "/json")
	if err != nil {
		return nil, fmt.Errorf("CDP: 无法连接调试端口 %s（浏览器需以 --remote-debugging-port 启动）: %w", e.debugURL, err)
	}
	defer resp.Body.Close()

	var targets []cdpTarget
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, fmt.Errorf("CDP: 解析 target 列表失败: %w", err)
	}

	// 优先选择 type=page 的 target
	var pages []cdpTarget
	for _, t := range targets {
		if t.Type == "page" {
			pages = append(pages, t)
		}
	}
	if len(pages) > 0 {
		return pages, nil
	}
	return targets, nil
}