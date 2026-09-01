package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/generic"
	"github.com/onllm-dev/onwatch/v2/internal/metrics"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

// 统一指标 REST API（Phase 3 P3-02）
//   - GET {base}/api/platforms          平台目录（标识 + 显示名 + 状态）
//   - GET {base}/api/metrics            全部平台统一指标
//   - GET {base}/api/metrics/{platform} 单平台统一指标
//
// 鉴权由全局 session 中间件统一处理，此处不做额外校验。

// unifiedMetricsPrefix 单平台路径前缀（与注册路由保持一致）
const unifiedMetricsPrefix = "/api/metrics/"

// unifiedBuiltinKeys 本批接入统一 API 的内置 provider
// （P0 四家 ark/zai/deepseek/opencode + P3-04 openrouter，其余后续批次）
var unifiedBuiltinKeys = []string{"ark", "zai", "deepseek", "opencode", "openrouter"}

// platformSummary GET /api/platforms 的单条平台摘要
type platformSummary struct {
	Platform    string `json:"platform"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
}

// Platforms 平台目录端点
func (h *Handler) Platforms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	snapshots, displayNames := h.collectUnifiedSnapshots()
	items := make([]platformSummary, 0, len(snapshots))
	for _, s := range snapshots {
		name := s.DisplayName
		if name == "" {
			name = displayNames[s.Platform]
		}
		items = append(items, platformSummary{Platform: s.Platform, DisplayName: name, Status: s.Status})
	}
	respondJSON(w, http.StatusOK, items)
}

// UnifiedMetrics 全部平台统一指标端点
func (h *Handler) UnifiedMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	snapshots, _ := h.collectUnifiedSnapshots()
	respondJSON(w, http.StatusOK, snapshots)
}

// UnifiedPlatformMetrics 单平台统一指标端点
func (h *Handler) UnifiedPlatformMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	platform := parseUnifiedPlatform(r.URL.Path)
	snapshots, _ := h.collectUnifiedSnapshots()
	for _, s := range snapshots {
		if s.Platform == platform {
			respondJSON(w, http.StatusOK, s)
			return
		}
	}
	respondError(w, http.StatusNotFound, "platform not found")
}

// parseUnifiedPlatform 解析 /api/metrics/{platform} 中的平台标识。
// 兼容 basePath 部署（如 /onwatch/api/metrics/ark）。
func parseUnifiedPlatform(path string) string {
	if idx := strings.LastIndex(path, unifiedMetricsPrefix); idx >= 0 {
		return path[idx+len(unifiedMetricsPrefix):]
	}
	return path
}

// collectUnifiedSnapshots 组装统一快照集合：内置 P0 四家 + generic 平台直通。
// 同时返回内置平台显示名映射（provider 目录）。
func (h *Handler) collectUnifiedSnapshots() ([]generic.PlatformSnapshot, map[string]string) {
	displayNames := make(map[string]string, 16)
	for _, item := range providerCatalog() {
		displayNames[item.Key] = item.Name
	}

	snapshots := make([]generic.PlatformSnapshot, 0, len(unifiedBuiltinKeys))
	if h.store == nil {
		return snapshots, displayNames
	}

	visibility := h.providerVisibilitySettings()
	for _, key := range unifiedBuiltinKeys {
		snap, include := h.unifiedBuiltinSnapshot(key, visibility)
		if !include {
			continue
		}
		snap.Platform = key
		snap.DisplayName = displayNames[key]
		snapshots = append(snapshots, *snap)
	}
	snapshots = append(snapshots, h.unifiedGenericSnapshots()...)
	return snapshots, displayNames
}

// unifiedBuiltinSnapshot 查询单个内置 provider 并按状态规则组装快照。
// include=false 表示平台不进入输出集合（telemetry 禁用，或未配置且无历史数据）。
func (h *Handler) unifiedBuiltinSnapshot(key string, visibility map[string]interface{}) (*generic.PlatformSnapshot, bool) {
	if !providerTelemetryEnabled(visibility, key) {
		return nil, false
	}
	configured := h.isProviderConfigured(key)

	switch key {
	case "ark":
		quotas, err := h.store.QueryArkLatestPerQuota()
		if err != nil {
			return unifiedConfiguredError(err, configured)
		}
		if len(quotas) == 0 {
			// PerQuota 表无数据时走整快照兜底（快照窗口映射为切片后复用同一 converter）
			snap, err := h.store.QueryLatestArk()
			if err != nil {
				return unifiedConfiguredError(err, configured)
			}
			if snap == nil {
				return unifiedAwaiting(configured)
			}
			for _, win := range snap.Windows {
				quotas = append(quotas, store.ArkLatestQuota{
					Name:        win.Name,
					Used:        win.Used,
					Limit:       win.Quota,
					Utilization: win.Percent,
					ResetsAt:    win.ResetsAt,
					SubscribeAt: win.SubscribeAt,
					CapturedAt:  snap.CapturedAt,
					PlanType:    snap.PlanType,
				})
			}
		}
		if len(quotas) == 0 {
			return unifiedAwaiting(configured)
		}
		return unifiedOK(metrics.ConvertArk(quotas), rfc3339Time(quotas[0].CapturedAt)), true

	case "zai":
		snap, err := h.store.QueryLatestZai()
		if err != nil {
			return unifiedConfiguredError(err, configured)
		}
		if snap == nil {
			return unifiedAwaiting(configured)
		}
		return unifiedOK(metrics.ConvertZai(snap), rfc3339Time(snap.CapturedAt)), true

	case "deepseek":
		snap, err := h.store.QueryLatestDeepSeek()
		if err != nil {
			return unifiedConfiguredError(err, configured)
		}
		if snap == nil {
			return unifiedAwaiting(configured)
		}
		return unifiedOK(metrics.ConvertDeepSeek(snap), rfc3339Time(snap.CapturedAt)), true

	case "openrouter":
		snap, err := h.store.QueryLatestOpenRouter()
		if err != nil {
			return unifiedConfiguredError(err, configured)
		}
		if snap == nil {
			return unifiedAwaiting(configured)
		}
		return unifiedOK(metrics.ConvertOpenRouter(snap), rfc3339Time(snap.CapturedAt)), true

	case "opencode":
		quotas, err := h.store.QueryOpenCodeLatestPerQuota()
		if err != nil {
			return unifiedConfiguredError(err, configured)
		}
		if len(quotas) == 0 {
			snap, err := h.store.QueryLatestOpenCode()
			if err != nil {
				return unifiedConfiguredError(err, configured)
			}
			if snap == nil {
				return unifiedAwaiting(configured)
			}
			for _, q := range snap.Quotas {
				quotas = append(quotas, store.OpenCodeLatestQuota{
					Name:        q.Name,
					Used:        q.Used,
					Limit:       q.Limit,
					Utilization: q.Utilization,
					Format:      string(q.Format),
					ResetsAt:    q.ResetsAt,
					CapturedAt:  snap.CapturedAt,
					AccountType: string(snap.AccountType),
					PlanName:    snap.PlanName,
				})
			}
		}
		if len(quotas) == 0 {
			return unifiedAwaiting(configured)
		}
		return unifiedOK(metrics.ConvertOpenCode(quotas), rfc3339Time(quotas[0].CapturedAt)), true
	}
	return nil, false
}

// unifiedGenericSnapshots generic 平台直通：命中存量快照直接使用（覆盖为当前配置显示名），
// 配置了但从未轮询的平台补 unconfigured 占位。
func (h *Handler) unifiedGenericSnapshots() []generic.PlatformSnapshot {
	platforms, err := generic.LoadPlatforms(h.store)
	if err != nil || len(platforms) == 0 {
		return nil
	}
	all, err := h.store.GetAllGenericSnapshots()
	if err != nil {
		all = nil
	}
	byName := make(map[string]generic.PlatformSnapshot, len(all))
	for _, s := range all {
		byName[s.Platform] = s
	}
	out := make([]generic.PlatformSnapshot, 0, len(platforms))
	for _, p := range platforms {
		if s, ok := byName[p.Name]; ok {
			s.DisplayName = p.DisplayName
			out = append(out, s)
			continue
		}
		out = append(out, generic.PlatformSnapshot{
			Platform:    p.Name,
			DisplayName: p.DisplayName,
			Status:      generic.StatusUnconfigured,
		})
	}
	return out
}

// --- 状态组装 helper（R1.6） ---

// unifiedConfiguredError 查询出错：已配置输出 error 平台，未配置则不输出
func unifiedConfiguredError(err error, configured bool) (*generic.PlatformSnapshot, bool) {
	if !configured {
		return nil, false
	}
	return &generic.PlatformSnapshot{Status: generic.StatusError, Error: err.Error()}, true
}

// unifiedAwaiting 已配置但库中无任何快照
func unifiedAwaiting(configured bool) (*generic.PlatformSnapshot, bool) {
	if !configured {
		return nil, false
	}
	return &generic.PlatformSnapshot{Status: generic.StatusError, Error: "awaiting first poll"}, true
}

// unifiedOK 有快照：ok + 转换结果 + UpdatedAt
func unifiedOK(m *generic.Metrics, updatedAt string) *generic.PlatformSnapshot {
	return &generic.PlatformSnapshot{Status: generic.StatusOK, UpdatedAt: updatedAt, Metrics: m}
}

// rfc3339Time 将时间格式化为 RFC3339 UTC 字符串（零值返回空串）
func rfc3339Time(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// --- 统一历史趋势路由（Phase 3 P3-06） ---

// unifiedHistoryPrefix 单平台历史路径前缀（与注册路由保持一致）
const unifiedHistoryPrefix = "/api/history/"

// unifiedHistoryPlatforms 内置 provider 中支持历史查询的 key（"both" 为旧路由聚合模式，不是平台）
var unifiedHistoryPlatforms = map[string]bool{
	"zai": true, "synthetic": true, "anthropic": true, "copilot": true,
	"codex": true, "antigravity": true, "minimax": true, "openrouter": true,
	"moonshot": true, "deepseek": true, "gemini": true, "cursor": true,
	"grok": true, "kimi": true, "opencode": true, "ark": true,
}

// parseUnifiedHistoryPlatform 解析 /api/history/{platform} 中的平台标识。
// 兼容 basePath 部署（如 /onwatch/api/history/ark）。
func parseUnifiedHistoryPlatform(path string) string {
	if idx := strings.LastIndex(path, unifiedHistoryPrefix); idx >= 0 {
		return path[idx+len(unifiedHistoryPrefix):]
	}
	return path
}

// UnifiedHistory 统一历史趋势端点：GET /api/history/{platform}
//   - 内置 provider：分发到既有 range 查询 + downsample（复用 dispatchHistory，支持 ?range=90d）
//   - generic 平台：generic_metrics 仅保留最新快照，无历史，返回空数据
func (h *Handler) UnifiedHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	platform := parseUnifiedHistoryPlatform(r.URL.Path)
	if platform == "" || platform == "both" {
		respondError(w, http.StatusNotFound, "platform not found")
		return
	}
	if unifiedHistoryPlatforms[platform] {
		h.dispatchHistory(platform, w, r)
		return
	}
	if h.isGenericPlatform(platform) {
		respondJSON(w, http.StatusOK, map[string]interface{}{"platform": platform, "points": []interface{}{}})
		return
	}
	respondError(w, http.StatusNotFound, "platform not found")
}

// isGenericPlatform 判断平台是否为配置驱动的 generic 平台。
func (h *Handler) isGenericPlatform(name string) bool {
	if h.store == nil {
		return false
	}
	platforms, err := generic.LoadPlatforms(h.store)
	if err != nil {
		return false
	}
	for _, p := range platforms {
		if p.Name == name {
			return true
		}
	}
	return false
}
