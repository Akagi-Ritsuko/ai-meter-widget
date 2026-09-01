package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/config"
)

// P3-06 统一历史趋势：90d 档位 + /api/history/{platform} 统一路由

func TestParseTimeRange90d(t *testing.T) {
	d, err := parseTimeRange("90d")
	if err != nil {
		t.Fatalf("parseTimeRange(\"90d\") 返回错误: %v", err)
	}
	if d != 90*24*time.Hour {
		t.Errorf("parseTimeRange(\"90d\") = %v, 期望 %v", d, 90*24*time.Hour)
	}
}

func TestParseTimeRangeLegacyUnchanged(t *testing.T) {
	// 既有档位保持兼容，非法档位仍报错
	for _, rangeStr := range []string{"", "1h", "6h", "24h", "1d", "7d", "30d"} {
		if _, err := parseTimeRange(rangeStr); err != nil {
			t.Errorf("parseTimeRange(%q) 不应报错: %v", rangeStr, err)
		}
	}
	if _, err := parseTimeRange("45d"); err == nil {
		t.Error("parseTimeRange(\"45d\") 应报错")
	}
}

func TestUnifiedHistoryBuiltin90d(t *testing.T) {
	h := newUnifiedTestHandler(t, &config.Config{ZaiAPIKey: "test-key"})
	insertUnifiedZaiSnapshot(t, h)

	w := httptest.NewRecorder()
	h.UnifiedHistory(w, httptest.NewRequest(http.MethodGet, "/api/history/zai?range=90d", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, 期望 200\nbody: %s", w.Code, w.Body.String())
	}
	var points []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &points); err != nil {
		t.Fatalf("解析响应失败: %v\nbody: %s", err, w.Body.String())
	}
	if len(points) != 1 {
		t.Fatalf("点数 = %d, 期望 1", len(points))
	}
	if _, ok := points[0]["capturedAt"]; !ok {
		t.Errorf("zai 历史点缺少 capturedAt 字段: %v", points[0])
	}
}

func TestUnifiedHistoryUnknownPlatform(t *testing.T) {
	h := newUnifiedTestHandler(t, &config.Config{ZaiAPIKey: "test-key"})

	t.Run("未知平台404", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.UnifiedHistory(w, httptest.NewRequest(http.MethodGet, "/api/history/doesnotexist", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, 期望 404", w.Code)
		}
	})

	t.Run("空平台404", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.UnifiedHistory(w, httptest.NewRequest(http.MethodGet, "/api/history/", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, 期望 404", w.Code)
		}
	})

	t.Run("both非统一路由平台404", func(t *testing.T) {
		// "both" 是旧路由 ?provider=both 的聚合模式，不是平台标识
		w := httptest.NewRecorder()
		h.UnifiedHistory(w, httptest.NewRequest(http.MethodGet, "/api/history/both", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, 期望 404", w.Code)
		}
	})
}

func TestUnifiedHistoryMethodNotAllowed(t *testing.T) {
	h := newUnifiedTestHandler(t, &config.Config{ZaiAPIKey: "test-key"})
	w := httptest.NewRecorder()
	h.UnifiedHistory(w, httptest.NewRequest(http.MethodPost, "/api/history/zai", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, 期望 405", w.Code)
	}
}

func TestUnifiedHistoryGenericEmptyData(t *testing.T) {
	// generic 平台仅存最新快照（upsert），无历史数据 → 返回空数据而非 404
	h := newUnifiedTestHandler(t, nil)
	saveUnifiedGenericFixtures(t, h)

	for _, platform := range []string{"gen-ok", "gen-empty"} {
		w := httptest.NewRecorder()
		h.UnifiedHistory(w, httptest.NewRequest(http.MethodGet, "/api/history/"+platform, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, 期望 200", platform, w.Code)
		}
		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: 解析响应失败: %v", platform, err)
		}
		if body["platform"] != platform {
			t.Errorf("%s: platform = %v", platform, body["platform"])
		}
		points, ok := body["points"].([]interface{})
		if !ok || len(points) != 0 {
			t.Errorf("%s: points 应为空数组, got %v", platform, body["points"])
		}
	}
}

func TestUnifiedHistoryInvalidRange(t *testing.T) {
	h := newUnifiedTestHandler(t, &config.Config{ZaiAPIKey: "test-key"})
	w := httptest.NewRecorder()
	h.UnifiedHistory(w, httptest.NewRequest(http.MethodGet, "/api/history/zai?range=45d", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, 期望 400", w.Code)
	}
}

func TestHistoryLegacyRouteUnchanged(t *testing.T) {
	// 旧路由 /api/history?provider=xxx 行为不变（含 90d 档位与未知 provider 报错）
	h := newUnifiedTestHandler(t, &config.Config{ZaiAPIKey: "test-key"})
	insertUnifiedZaiSnapshot(t, h)

	t.Run("既有分发不受影响", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.History(w, httptest.NewRequest(http.MethodGet, "/api/history?provider=zai&range=30d", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, 期望 200\nbody: %s", w.Code, w.Body.String())
		}
	})

	t.Run("旧路由90d", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.History(w, httptest.NewRequest(http.MethodGet, "/api/history?provider=zai&range=90d", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, 期望 200\nbody: %s", w.Code, w.Body.String())
		}
	})

	t.Run("未知provider仍400", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.History(w, httptest.NewRequest(http.MethodGet, "/api/history?provider=doesnotexist", nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, 期望 400", w.Code)
		}
	})
}
