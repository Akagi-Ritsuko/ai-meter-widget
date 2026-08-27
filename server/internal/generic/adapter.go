package generic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SnapshotStore 指标快照存储接口
type SnapshotStore interface {
	SaveGenericSnapshot(snapshot *PlatformSnapshot) error
	GetGenericSnapshot(platform string) (*PlatformSnapshot, error)
	GetAllGenericSnapshots() ([]PlatformSnapshot, error)
}

// Agent 通用适配器轮询代理，实现 agent.AgentRunner 接口
type Agent struct {
	store    ConfigStore
	snapshot SnapshotStore
	logger   *slog.Logger
	client   *http.Client

	mu        sync.RWMutex
	snapshots map[string]*PlatformSnapshot // 内存缓存，供 API 即时读取
}

// NewAgent 创建通用适配器代理
func NewAgent(store ConfigStore, snapshot SnapshotStore, logger *slog.Logger) *Agent {
	if logger == nil {
		logger = slog.Default()
	}
	return &Agent{
		store:     store,
		snapshot:  snapshot,
		logger:    logger,
		client:    &http.Client{Timeout: 30 * time.Second},
		snapshots: make(map[string]*PlatformSnapshot),
	}
}

// Run 启动轮询循环（agent.AgentRunner 接口）
func (a *Agent) Run(ctx context.Context) error {
	a.logger.Info("Generic adapter agent started")

	// 启动时立即轮询一次
	a.pollAll(ctx)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.pollAll(ctx)
		case <-ctx.Done():
			a.logger.Info("Generic adapter agent stopped")
			return nil
		}
	}
}

// pollAll 轮询所有已启用平台
func (a *Agent) pollAll(ctx context.Context) {
	platforms, err := LoadPlatforms(a.store)
	if err != nil {
		a.logger.Error("Generic adapter: 加载平台配置失败", "error", err)
		return
	}

	for _, p := range platforms {
		if !p.Enabled {
			continue
		}
		a.pollPlatform(ctx, p)
	}
}

// pollPlatform 轮询单个平台的所有数据源
func (a *Agent) pollPlatform(ctx context.Context, p PlatformConfig) {
	snapshot := &PlatformSnapshot{
		Platform:    p.Name,
		DisplayName: p.DisplayName,
		Status:      StatusOK,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
		Metrics:     &Metrics{},
	}

	// 解析凭证
	key, err := p.Auth.ResolveKey()
	if err != nil {
		snapshot.Status = StatusAuthFailed
		snapshot.Error = err.Error()
		a.saveSnapshot(snapshot)
		a.logger.Warn("Generic adapter: 凭证解析失败", "platform", p.Name, "error", err)
		return
	}

	for _, source := range p.Sources {
		body, err := a.fetchSource(ctx, p, source, key)
		if err != nil {
			a.logger.Warn("Generic adapter: 数据源请求失败",
				"platform", p.Name, "source", source.Name, "error", err)
			snapshot.Status = StatusError
			snapshot.Error = fmt.Sprintf("source %s: %v", source.Name, err)
			continue
		}

		mapped, err := mapSource(source, body)
		if err != nil {
			a.logger.Warn("Generic adapter: 字段映射失败",
				"platform", p.Name, "source", source.Name, "error", err)
			snapshot.Status = StatusError
			snapshot.Error = fmt.Sprintf("source %s: %v", source.Name, err)
			continue
		}

		metrics, err := buildMetrics(source, mapped)
		if err != nil {
			a.logger.Warn("Generic adapter: 指标组装失败",
				"platform", p.Name, "source", source.Name, "error", err)
			continue
		}
		mergeMetrics(snapshot.Metrics, metrics)
	}

	a.saveSnapshot(snapshot)
	a.logger.Debug("Generic adapter: 平台轮询完成",
		"platform", p.Name, "status", snapshot.Status)
}

// fetchSource 请求单个数据源接口
func (a *Agent) fetchSource(ctx context.Context, p PlatformConfig, source SourceConfig, key string) ([]byte, error) {
	method := source.Method
	if method == "" {
		method = http.MethodGet
	}

	req, err := http.NewRequestWithContext(ctx, method, source.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ai-meter-widget/0.1")

	// 应用认证
	applyAuth(req, p.Auth, key)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated:
		return body, nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("认证失败 (HTTP %d)", resp.StatusCode)
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("服务端错误 (HTTP %d)", resp.StatusCode)
	default:
		return nil, fmt.Errorf("意外状态码 (HTTP %d)", resp.StatusCode)
	}
}

// applyAuth 根据认证类型设置请求头
func applyAuth(req *http.Request, auth AuthConfig, key string) {
	if key == "" {
		return
	}
	header := auth.Header
	if header == "" {
		header = "Authorization"
	}

	switch auth.Type {
	case AuthTypeBearer:
		req.Header.Set(header, "Bearer "+key)
	case AuthTypeCookie:
		req.Header.Set(header, key)
	case AuthTypeAPIKey, AuthTypeOAuthLocal, AuthTypeNone:
		req.Header.Set(header, key)
	}
}

// saveSnapshot 保存快照到存储与内存缓存
func (a *Agent) saveSnapshot(snapshot *PlatformSnapshot) {
	a.mu.Lock()
	a.snapshots[snapshot.Platform] = snapshot
	a.mu.Unlock()

	if a.snapshot != nil {
		if err := a.snapshot.SaveGenericSnapshot(snapshot); err != nil {
			a.logger.Error("Generic adapter: 保存快照失败", "platform", snapshot.Platform, "error", err)
		}
	}
}

// GetSnapshot 从内存缓存读取平台快照
func (a *Agent) GetSnapshot(platform string) (*PlatformSnapshot, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	s, ok := a.snapshots[platform]
	return s, ok
}

// GetAllSnapshots 读取全部平台快照
func (a *Agent) GetAllSnapshots() []PlatformSnapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]PlatformSnapshot, 0, len(a.snapshots))
	for _, s := range a.snapshots {
		result = append(result, *s)
	}
	return result
}

// TestConnection 测试连接：请求接口并返回映射结果（供配置页"测试连接"使用）
func (a *Agent) TestConnection(ctx context.Context, p PlatformConfig) (*TestResult, error) {
	key, err := p.Auth.ResolveKey()
	if err != nil {
		return nil, err
	}

	result := &TestResult{Platform: p.Name, Sources: make([]SourceTestResult, 0, len(p.Sources))}

	for _, source := range p.Sources {
		body, err := a.fetchSource(ctx, p, source, key)
		if err != nil {
			result.Sources = append(result.Sources, SourceTestResult{
				Name:  source.Name,
				OK:    false,
				Error: err.Error(),
			})
			continue
		}

		mapped, err := mapSource(source, body)
		if err != nil {
			result.Sources = append(result.Sources, SourceTestResult{
				Name:  source.Name,
				OK:    false,
				Error: err.Error(),
			})
			continue
		}

		metrics, err := buildMetrics(source, mapped)
		if err != nil {
			result.Sources = append(result.Sources, SourceTestResult{
				Name:  source.Name,
				OK:    false,
				Error: err.Error(),
			})
			continue
		}

		result.Sources = append(result.Sources, SourceTestResult{
			Name:    source.Name,
			OK:      true,
			Metrics: metrics,
			Raw:     string(body),
		})
	}

	return result, nil
}

// TestResult 测试连接结果
type TestResult struct {
	Platform string              `json:"platform"`
	Sources  []SourceTestResult  `json:"sources"`
}

// SourceTestResult 单个数据源测试结果
type SourceTestResult struct {
	Name    string   `json:"name"`
	OK      bool     `json:"ok"`
	Error   string   `json:"error,omitempty"`
	Metrics *Metrics `json:"metrics,omitempty"`
	Raw     string   `json:"raw,omitempty"`
}

// MarshalJSON 自定义序列化，确保空 Metrics 输出为 null 而非空对象
func (s *PlatformSnapshot) MarshalJSON() ([]byte, error) {
	type alias PlatformSnapshot
	if s.Metrics != nil && len(s.Metrics.Quota) == 0 && s.Metrics.Balance == nil &&
		s.Metrics.Cost == nil && s.Metrics.Tokens == nil {
		cp := *s
		cp.Metrics = nil
		return json.Marshal((*alias)(&cp))
	}
	return json.Marshal((*alias)(s))
}

// 确保 strings 包被使用（applyAuth 中已使用）
var _ = strings.TrimSpace