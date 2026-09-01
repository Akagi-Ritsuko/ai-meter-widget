package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/config"
	"github.com/onllm-dev/onwatch/v2/internal/generic"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

func newUnifiedTestHandler(t *testing.T, cfg *config.Config) *Handler {
	t.Helper()
	db, err := store.New(filepath.Join(t.TempDir(), "unified.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Handler{store: db, config: cfg}
}

func insertUnifiedZaiSnapshot(t *testing.T, h *Handler) {
	t.Helper()
	if _, err := h.store.InsertZaiSnapshot(&api.ZaiSnapshot{
		CapturedAt:       time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		TimeLimit:        6000,
		TimeUsage:        1500,
		TimePercentage:   25,
		TokensLimit:      70000,
		TokensUsage:      21000,
		TokensPercentage: 30,
	}); err != nil {
		t.Fatal(err)
	}
}

func insertUnifiedOpenRouterSnapshot(t *testing.T, h *Handler) {
	t.Helper()
	if _, err := h.store.InsertOpenRouterSnapshot(&api.OpenRouterSnapshot{
		CapturedAt:   time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		UsageDaily:   1.25,
		UsageMonthly: 30.5,
	}); err != nil {
		t.Fatal(err)
	}
}

func saveUnifiedGenericFixtures(t *testing.T, h *Handler) {
	t.Helper()
	// 两个 generic 平台：gen-ok 有快照（快照内 display_name 为旧值，应被配置显示名覆盖），
	// gen-empty 配置了但从未轮询
	if err := generic.SavePlatforms(h.store, []generic.PlatformConfig{
		{Name: "gen-ok", DisplayName: "Gen OK", Enabled: true},
		{Name: "gen-empty", DisplayName: "Gen Empty", Enabled: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.store.SaveGenericSnapshot(&generic.PlatformSnapshot{
		Platform:    "gen-ok",
		DisplayName: "stale-name",
		Status:      generic.StatusOK,
		UpdatedAt:   "2026-09-01T11:00:00Z",
		Metrics:     &generic.Metrics{Quota: []generic.QuotaMetric{{Window: "5h", Percent: 10}}},
	}); err != nil {
		t.Fatal(err)
	}
}

func decodeUnifiedBody(t *testing.T, w *httptest.ResponseRecorder) []generic.PlatformSnapshot {
	t.Helper()
	var snapshots []generic.PlatformSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snapshots); err != nil {
		t.Fatalf("解析响应失败: %v\nbody: %s", err, w.Body.String())
	}
	return snapshots
}

func findUnifiedSnapshot(snapshots []generic.PlatformSnapshot, platform string) *generic.PlatformSnapshot {
	for i := range snapshots {
		if snapshots[i].Platform == platform {
			return &snapshots[i]
		}
	}
	return nil
}

func TestUnifiedPlatformsList(t *testing.T) {
	h := newUnifiedTestHandler(t, &config.Config{ZaiAPIKey: "test-key"})
	insertUnifiedZaiSnapshot(t, h)
	saveUnifiedGenericFixtures(t, h)

	w := httptest.NewRecorder()
	h.Platforms(w, httptest.NewRequest(http.MethodGet, "/api/platforms", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, 期望 200", w.Code)
	}
	var items []platformSummary
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}

	var zai, genOK, genEmpty *platformSummary
	for i := range items {
		switch items[i].Platform {
		case "zai":
			zai = &items[i]
		case "gen-ok":
			genOK = &items[i]
		case "gen-empty":
			genEmpty = &items[i]
		case "deepseek", "ark", "opencode", "openrouter":
			t.Errorf("未配置且无数据的平台 %q 不应出现在目录中", items[i].Platform)
		}
	}
	if zai == nil || zai.Status != generic.StatusOK || zai.DisplayName != "Z.ai" {
		t.Errorf("zai 目录项错误: %+v", zai)
	}
	if genOK == nil || genOK.Status != generic.StatusOK || genOK.DisplayName != "Gen OK" {
		t.Errorf("gen-ok 目录项错误: %+v", genOK)
	}
	if genEmpty == nil || genEmpty.Status != generic.StatusUnconfigured {
		t.Errorf("gen-empty 应以 unconfigured 列出: %+v", genEmpty)
	}
}

func TestUnifiedMetricsAll(t *testing.T) {
	h := newUnifiedTestHandler(t, &config.Config{ZaiAPIKey: "test-key"})
	insertUnifiedZaiSnapshot(t, h)
	saveUnifiedGenericFixtures(t, h)

	w := httptest.NewRecorder()
	h.UnifiedMetrics(w, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, 期望 200", w.Code)
	}
	snapshots := decodeUnifiedBody(t, w)

	zai := findUnifiedSnapshot(snapshots, "zai")
	if zai == nil {
		t.Fatal("zai 应出现在统一指标中")
	}
	if zai.Status != generic.StatusOK {
		t.Errorf("zai status = %q, 期望 ok", zai.Status)
	}
	if zai.DisplayName != "Z.ai" {
		t.Errorf("zai display_name = %q, 期望 \"Z.ai\"", zai.DisplayName)
	}
	if zai.UpdatedAt != "2026-09-01T12:00:00Z" {
		t.Errorf("zai updated_at = %q, 期望快照 CapturedAt RFC3339 UTC", zai.UpdatedAt)
	}
	if zai.Metrics == nil || len(zai.Metrics.Quota) != 2 {
		t.Fatalf("zai metrics 应含 2 个窗口, got %+v", zai.Metrics)
	}
	if zai.Metrics.Quota[0].Window != "5h" || zai.Metrics.Quota[0].Used != 1500 ||
		zai.Metrics.Quota[0].Total != 6000 || zai.Metrics.Quota[0].Percent != 25 {
		t.Errorf("5h 窗口数据错误: %+v", zai.Metrics.Quota[0])
	}
	if zai.Metrics.Quota[1].Window != "weekly" || zai.Metrics.Quota[1].Percent != 30 {
		t.Errorf("weekly 窗口数据错误: %+v", zai.Metrics.Quota[1])
	}

	genOK := findUnifiedSnapshot(snapshots, "gen-ok")
	if genOK == nil || genOK.Status != generic.StatusOK || genOK.DisplayName != "Gen OK" {
		t.Errorf("gen-ok 直通快照错误: %+v", genOK)
	}
	genEmpty := findUnifiedSnapshot(snapshots, "gen-empty")
	if genEmpty == nil || genEmpty.Status != generic.StatusUnconfigured {
		t.Errorf("gen-empty 应补 unconfigured 占位: %+v", genEmpty)
	}
}

func TestUnifiedMetricsSinglePlatform(t *testing.T) {
	h := newUnifiedTestHandler(t, &config.Config{ZaiAPIKey: "test-key"})
	insertUnifiedZaiSnapshot(t, h)

	t.Run("命中平台返回200", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.UnifiedPlatformMetrics(w, httptest.NewRequest(http.MethodGet, "/api/metrics/zai", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, 期望 200", w.Code)
		}
		var snap generic.PlatformSnapshot
		if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		if snap.Platform != "zai" || snap.Status != generic.StatusOK || snap.Metrics == nil {
			t.Errorf("单平台快照错误: %+v", snap)
		}
	})

	t.Run("未知平台返回404", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.UnifiedPlatformMetrics(w, httptest.NewRequest(http.MethodGet, "/api/metrics/doesnotexist", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, 期望 404", w.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		if body["error"] != "platform not found" {
			t.Errorf("error = %q, 期望 \"platform not found\"", body["error"])
		}
	})
}

func TestUnifiedMethodsNotAllowed(t *testing.T) {
	h := newUnifiedTestHandler(t, &config.Config{ZaiAPIKey: "test-key"})
	cases := []struct {
		method string
		path   string
		call   func(w http.ResponseWriter, r *http.Request)
	}{
		{http.MethodPost, "/api/platforms", h.Platforms},
		{http.MethodPost, "/api/metrics", h.UnifiedMetrics},
		{http.MethodPost, "/api/metrics/zai", h.UnifiedPlatformMetrics},
		{http.MethodDelete, "/api/metrics/zai", h.UnifiedPlatformMetrics},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		c.call(w, httptest.NewRequest(c.method, c.path, nil))
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s status = %d, 期望 405", c.method, c.path, w.Code)
		}
	}
}

func TestUnifiedAwaitingFirstPoll(t *testing.T) {
	// deepseek 已配置但从未轮询 → error + "awaiting first poll"
	h := newUnifiedTestHandler(t, &config.Config{DeepSeekAPIKey: "test-key"})

	w := httptest.NewRecorder()
	h.UnifiedPlatformMetrics(w, httptest.NewRequest(http.MethodGet, "/api/metrics/deepseek", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, 期望 200（单平台查询失败不影响 HTTP 状态）", w.Code)
	}
	var snap generic.PlatformSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if snap.Platform != "deepseek" || snap.Status != generic.StatusError {
		t.Errorf("快照状态错误: %+v", snap)
	}
	if snap.Error != "awaiting first poll" {
		t.Errorf("error = %q, 期望 \"awaiting first poll\"", snap.Error)
	}
	if snap.Metrics != nil || snap.UpdatedAt != "" {
		t.Errorf("error 状态下 Metrics 应为 nil、UpdatedAt 应为空: %+v", snap)
	}
}

func TestUnifiedUnconfiguredOmitted(t *testing.T) {
	// 无任何凭证（config 为 nil）→ 内置平台全部不输出，仅 generic 平台保留
	h := newUnifiedTestHandler(t, nil)
	insertUnifiedZaiSnapshot(t, h)
	saveUnifiedGenericFixtures(t, h)

	w := httptest.NewRecorder()
	h.UnifiedMetrics(w, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))
	snapshots := decodeUnifiedBody(t, w)
	for _, s := range snapshots {
		switch s.Platform {
		case "zai":
			// zai 虽未配置但库中有历史快照 → 应保留（D5 门控：已配置 或 有快照）
			if s.Status != generic.StatusOK {
				t.Errorf("zai status = %q, 期望 ok", s.Status)
			}
		case "ark", "deepseek", "opencode", "openrouter":
			t.Errorf("未配置且无数据的平台 %q 不应输出", s.Platform)
		}
	}
	if findUnifiedSnapshot(snapshots, "zai") == nil {
		t.Fatal("有历史快照的未配置平台 zai 应保留")
	}
	if findUnifiedSnapshot(snapshots, "gen-ok") == nil || findUnifiedSnapshot(snapshots, "gen-empty") == nil {
		t.Fatal("generic 平台应全部列出")
	}
}

func TestUnifiedOpenRouterCost(t *testing.T) {
	// P3-04：openrouter cost 真实输出（内置侧 1/2）
	h := newUnifiedTestHandler(t, &config.Config{OpenRouterAPIKey: "test-key"})
	insertUnifiedOpenRouterSnapshot(t, h)

	t.Run("单平台cost输出", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.UnifiedPlatformMetrics(w, httptest.NewRequest(http.MethodGet, "/api/metrics/openrouter", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, 期望 200", w.Code)
		}
		var snap generic.PlatformSnapshot
		if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		if snap.Platform != "openrouter" || snap.Status != generic.StatusOK {
			t.Fatalf("快照错误: %+v", snap)
		}
		if snap.Metrics == nil || snap.Metrics.Cost == nil {
			t.Fatalf("openrouter 应输出 cost, got %+v", snap.Metrics)
		}
		if snap.Metrics.Cost.Today != 1.25 || snap.Metrics.Cost.Month != 30.5 || snap.Metrics.Cost.Currency != "USD" {
			t.Errorf("cost = %v/%v %s, 期望 1.25/30.5 USD",
				snap.Metrics.Cost.Today, snap.Metrics.Cost.Month, snap.Metrics.Cost.Currency)
		}
		// 平台无 tokens 数据源 → JSON 输出 null 而非 0
		if snap.Metrics.Tokens != nil {
			t.Errorf("tokens 应为 null, got %+v", snap.Metrics.Tokens)
		}
	})

	t.Run("已配置但无快照awaiting", func(t *testing.T) {
		h2 := newUnifiedTestHandler(t, &config.Config{OpenRouterAPIKey: "test-key"})
		w := httptest.NewRecorder()
		h2.UnifiedPlatformMetrics(w, httptest.NewRequest(http.MethodGet, "/api/metrics/openrouter", nil))
		var snap generic.PlatformSnapshot
		if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		if snap.Status != generic.StatusError || snap.Error != "awaiting first poll" {
			t.Errorf("快照错误: %+v", snap)
		}
	})
}

func TestUnifiedGenericCostTokensPassthrough(t *testing.T) {
	// P3-04：generic mock 平台 cost/tokens 直通输出（通用适配器侧 2/2）
	h := newUnifiedTestHandler(t, nil)
	if err := generic.SavePlatforms(h.store, []generic.PlatformConfig{
		{Name: "gen-paid", DisplayName: "Gen Paid", Enabled: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.store.SaveGenericSnapshot(&generic.PlatformSnapshot{
		Platform:    "gen-paid",
		DisplayName: "Gen Paid",
		Status:      generic.StatusOK,
		UpdatedAt:   "2026-09-01T11:00:00Z",
		Metrics: &generic.Metrics{
			Cost:   &generic.CostMetric{Today: 0.8, Month: 12.4, Currency: "USD"},
			Tokens: &generic.TokensMetric{Today: 123456, Month: 2345678},
		},
	}); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	h.UnifiedPlatformMetrics(w, httptest.NewRequest(http.MethodGet, "/api/metrics/gen-paid", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, 期望 200", w.Code)
	}
	var snap generic.PlatformSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if snap.Metrics == nil || snap.Metrics.Cost == nil || snap.Metrics.Tokens == nil {
		t.Fatalf("generic 平台应直通 cost/tokens, got %+v", snap.Metrics)
	}
	if snap.Metrics.Cost.Today != 0.8 || snap.Metrics.Cost.Month != 12.4 || snap.Metrics.Cost.Currency != "USD" {
		t.Errorf("cost = %v/%v %s, 期望 0.8/12.4 USD",
			snap.Metrics.Cost.Today, snap.Metrics.Cost.Month, snap.Metrics.Cost.Currency)
	}
	if snap.Metrics.Tokens.Today != 123456 || snap.Metrics.Tokens.Month != 2345678 {
		t.Errorf("tokens = %v/%v, 期望 123456/2345678",
			snap.Metrics.Tokens.Today, snap.Metrics.Tokens.Month)
	}
}

func TestParseUnifiedPlatform(t *testing.T) {
	if got := parseUnifiedPlatform("/api/metrics/ark"); got != "ark" {
		t.Errorf("parseUnifiedPlatform(\"/api/metrics/ark\") = %q, 期望 \"ark\"", got)
	}
	if got := parseUnifiedPlatform("/onwatch/api/metrics/zai"); got != "zai" {
		t.Errorf("basePath 场景 = %q, 期望 \"zai\"", got)
	}
	if got := parseUnifiedPlatform("/api/metrics/"); got != "" {
		t.Errorf("空前缀 = %q, 期望空串", got)
	}
}
