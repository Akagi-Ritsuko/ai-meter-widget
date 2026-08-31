package generic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- config.go ----------

func TestResolveKey_DirectValue(t *testing.T) {
	a := AuthConfig{Type: AuthTypeAPIKey, Key: "sk-direct"}
	got, err := a.ResolveKey()
	if err != nil || got != "sk-direct" {
		t.Errorf("ResolveKey direct = %q, %v; want sk-direct", got, err)
	}
}

func TestResolveKey_Env(t *testing.T) {
	os.Setenv("TEST_GENERIC_KEY", "sk-env")
	defer os.Unsetenv("TEST_GENERIC_KEY")
	a := AuthConfig{Type: AuthTypeBearer, KeyFrom: "env:TEST_GENERIC_KEY"}
	got, err := a.ResolveKey()
	if err != nil || got != "sk-env" {
		t.Errorf("ResolveKey env = %q, %v; want sk-env", got, err)
	}
}

func TestResolveKey_EnvMissing(t *testing.T) {
	a := AuthConfig{KeyFrom: "env:NONEXISTENT_VAR_XYZ"}
	if _, err := a.ResolveKey(); err == nil {
		t.Error("expected error for missing env var")
	}
}

func TestResolveKey_File(t *testing.T) {
	dir := t.TempDir()
	authFile := filepath.Join(dir, "auth.json")
	os.WriteFile(authFile, []byte(`{"tokens":{"access_token":"sk-file"}}`), 0600)
	a := AuthConfig{Type: AuthTypeOAuthLocal, KeyFrom: "file:" + authFile + ":$.tokens.access_token"}
	got, err := a.ResolveKey()
	if err != nil || got != "sk-file" {
		t.Errorf("ResolveKey file = %q, %v; want sk-file", got, err)
	}
}

func TestResolveKey_FileBadPath(t *testing.T) {
	a := AuthConfig{KeyFrom: "file:/nonexistent/path.json:$.token"}
	if _, err := a.ResolveKey(); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestEffectiveInterval(t *testing.T) {
	s := SourceConfig{}
	if got := s.EffectiveInterval(0); got != 300 {
		t.Errorf("default interval = %d, want 300", got)
	}
	if got := s.EffectiveInterval(60); got != 60 {
		t.Errorf("platform interval = %d, want 60", got)
	}
	s2 := SourceConfig{Interval: 120}
	if got := s2.EffectiveInterval(60); got != 120 {
		t.Errorf("source interval = %d, want 120", got)
	}
}

// ---------- metrics.go ----------

func TestMapSource_JSONPathHit(t *testing.T) {
	src := SourceConfig{
		Name:    SourceBalance,
		Mapping: map[string]string{"balance.amount": "$.data.balance", "balance.currency": "$.data.currency"},
	}
	mapped, err := mapSource(src, []byte(`{"data":{"balance":12.5,"currency":"USD"}}`))
	if err != nil {
		t.Fatalf("mapSource failed: %v", err)
	}
	if mapped["balance.amount"] != float64(12.5) {
		t.Errorf("balance.amount = %v, want 12.5", mapped["balance.amount"])
	}
	if mapped["balance.currency"] != "USD" {
		t.Errorf("balance.currency = %v, want USD", mapped["balance.currency"])
	}
}

func TestMapSource_JSONPathMiss(t *testing.T) {
	src := SourceConfig{
		Name:    SourceBalance,
		Mapping: map[string]string{"balance.amount": "$.data.nonexistent"},
	}
	mapped, err := mapSource(src, []byte(`{"data":{}}`))
	if err != nil {
		t.Fatalf("mapSource failed: %v", err)
	}
	if _, ok := mapped["balance.amount"]; ok {
		t.Error("missing field should be skipped")
	}
}

func TestBuildMetrics_Quota(t *testing.T) {
	src := SourceConfig{Name: SourceQuota}
	mapped := map[string]interface{}{
		"quota.window":   "5h",
		"quota.used":     float64(75),
		"quota.total":    float64(100),
		"quota.reset_at": "2026-09-01T00:00:00Z",
	}
	m, err := buildMetrics(src, mapped)
	if err != nil {
		t.Fatalf("buildMetrics failed: %v", err)
	}
	if len(m.Quota) != 1 {
		t.Fatalf("quota count = %d, want 1", len(m.Quota))
	}
	q := m.Quota[0]
	if q.Window != "5h" || q.Used != 75 || q.Total != 100 || q.Percent != 75 {
		t.Errorf("quota = %+v", q)
	}
}

func TestBuildMetrics_QuotaPercentAuto(t *testing.T) {
	src := SourceConfig{Name: SourceQuota}
	mapped := map[string]interface{}{"quota.used": float64(25), "quota.total": float64(100)}
	m, _ := buildMetrics(src, mapped)
	if m.Quota[0].Percent != 25 {
		t.Errorf("auto percent = %v, want 25", m.Quota[0].Percent)
	}
}

func TestBuildMetrics_BalanceCostTokens(t *testing.T) {
	// balance
	m, _ := buildMetrics(SourceConfig{Name: SourceBalance}, map[string]interface{}{
		"balance.amount": float64(10), "balance.currency": "CNY",
	})
	if m.Balance == nil || m.Balance.Amount != 10 || m.Balance.Currency != "CNY" {
		t.Errorf("balance = %+v", m.Balance)
	}
	// cost
	m, _ = buildMetrics(SourceConfig{Name: SourceCost}, map[string]interface{}{
		"cost.today": float64(1.5), "cost.month": float64(30), "cost.currency": "USD",
	})
	if m.Cost == nil || m.Cost.Today != 1.5 || m.Cost.Month != 30 {
		t.Errorf("cost = %+v", m.Cost)
	}
	// tokens
	m, _ = buildMetrics(SourceConfig{Name: SourceTokens}, map[string]interface{}{
		"tokens.today": float64(1000), "tokens.month": float64(50000),
	})
	if m.Tokens == nil || m.Tokens.Today != 1000 || m.Tokens.Month != 50000 {
		t.Errorf("tokens = %+v", m.Tokens)
	}
}

func TestToFloatToString(t *testing.T) {
	if toFloat("42") != 42 || toFloat(float64(3.5)) != 3.5 || toFloat(json.Number("7")) != 7 {
		t.Error("toFloat conversion failed")
	}
	if toString(float64(12.5)) != "12.5" || toString("abc") != "abc" {
		t.Error("toString conversion failed")
	}
}

// ---------- adapter.go ----------

func TestApplyAuth(t *testing.T) {
	cases := []struct {
		name     string
		auth     AuthConfig
		key      string
		wantHdr  string
		wantVal  string
	}{
		{"api_key", AuthConfig{Type: AuthTypeAPIKey}, "k1", "Authorization", "k1"},
		{"api_key_custom_header", AuthConfig{Type: AuthTypeAPIKey, Header: "X-API-Key"}, "k1", "X-API-Key", "k1"},
		{"bearer", AuthConfig{Type: AuthTypeBearer}, "k1", "Authorization", "Bearer k1"},
		{"cookie", AuthConfig{Type: AuthTypeCookie}, "session=abc", "Authorization", "session=abc"},
		{"oauth_local", AuthConfig{Type: AuthTypeOAuthLocal}, "tok", "Authorization", "tok"},
		{"none", AuthConfig{Type: AuthTypeNone}, "", "Authorization", ""},
	}
	for _, tc := range cases {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
		applyAuth(req, tc.auth, tc.key)
		got := req.Header.Get(tc.wantHdr)
		if got != tc.wantVal {
			t.Errorf("%s: header %s = %q, want %q", tc.name, tc.wantHdr, got, tc.wantVal)
		}
	}
}

func TestFetchSource_StatusCodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.Write([]byte(`{"ok":true}`))
		case "/unauth":
			w.WriteHeader(http.StatusUnauthorized)
		case "/servererr":
			w.WriteHeader(http.StatusInternalServerError)
		case "/other":
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer server.Close()

	agent := NewAgent(nil, nil, nil)
	auth := AuthConfig{Type: AuthTypeNone}

	if _, err := agent.fetchSource(context.Background(), PlatformConfig{}, SourceConfig{URL: server.URL + "/ok"}, ""); err != nil {
		t.Errorf("ok path failed: %v", err)
	}
	if _, err := agent.fetchSource(context.Background(), PlatformConfig{}, SourceConfig{URL: server.URL + "/unauth"}, ""); err == nil || !strings.Contains(err.Error(), "认证失败") {
		t.Errorf("unauth path: %v", err)
	}
	if _, err := agent.fetchSource(context.Background(), PlatformConfig{}, SourceConfig{URL: server.URL + "/servererr"}, ""); err == nil || !strings.Contains(err.Error(), "服务端错误") {
		t.Errorf("servererr path: %v", err)
	}
	if _, err := agent.fetchSource(context.Background(), PlatformConfig{}, SourceConfig{URL: server.URL + "/other"}, ""); err == nil {
		t.Error("other status should error")
	}
	_ = auth
}

func TestPollPlatform_AuthFailed(t *testing.T) {
	// 凭证解析失败 → auth_failed
	agent := NewAgent(nil, nil, nil)
	p := PlatformConfig{
		Name:    "bad",
		Enabled: true,
		Auth:    AuthConfig{KeyFrom: "env:NONEXISTENT_VAR_XYZ"},
		Sources: []SourceConfig{{Name: SourceBalance, URL: "http://example.com"}},
	}
	agent.pollPlatform(context.Background(), p)
	snap, ok := agent.GetSnapshot("bad")
	if !ok {
		t.Fatal("snapshot not saved")
	}
	if snap.Status != StatusAuthFailed {
		t.Errorf("status = %s, want auth_failed", snap.Status)
	}
}

func TestTestConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"balance":5,"currency":"USD"}}`))
	}))
	defer server.Close()

	agent := NewAgent(nil, nil, nil)
	p := PlatformConfig{
		Name: "test",
		Auth: AuthConfig{Type: AuthTypeNone},
		Sources: []SourceConfig{{
			Name:    SourceBalance,
			URL:     server.URL,
			Mapping: map[string]string{"balance.amount": "$.data.balance"},
		}},
	}
	result, err := agent.TestConnection(context.Background(), p)
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
	if len(result.Sources) != 1 || !result.Sources[0].OK {
		t.Errorf("source result = %+v", result.Sources)
	}
	if result.Sources[0].Metrics.Balance == nil || result.Sources[0].Metrics.Balance.Amount != 5 {
		t.Errorf("balance = %+v", result.Sources[0].Metrics.Balance)
	}
}

// ---------- handlers.go ----------

type mockStore struct {
	platforms []PlatformConfig
}

func (m *mockStore) GetSetting(key string) (string, error) {
	if key != settingsKey {
		return "", nil
	}
	data, _ := json.Marshal(m.platforms)
	return string(data), nil
}
func (m *mockStore) SetSetting(key, value string) error {
	if key == settingsKey {
		json.Unmarshal([]byte(value), &m.platforms)
	}
	return nil
}

func TestHandler_UpsertAndList(t *testing.T) {
	store := &mockStore{}
	agent := NewAgent(store, nil, nil)
	h := NewHandler(agent, store)

	// 新增
	body := `{"name":"p1","display_name":"P1","enabled":true,"interval":60,"auth":{"type":"none"},"sources":[{"name":"balance","url":"http://x","mapping":{"balance.amount":"$.a"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/generic/platforms", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.UpsertPlatform(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("upsert status = %d, want 200", w.Code)
	}

	// 列表
	req2 := httptest.NewRequest(http.MethodGet, "/api/generic/platforms", nil)
	w2 := httptest.NewRecorder()
	h.ListPlatforms(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("list status = %d, want 200", w2.Code)
	}
	var list []PlatformConfig
	json.Unmarshal(w2.Body.Bytes(), &list)
	if len(list) != 1 || list[0].Name != "p1" {
		t.Errorf("list = %+v", list)
	}

	// 同名更新（覆盖）
	h.UpsertPlatform(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/generic/platforms", strings.NewReader(`{"name":"p1","display_name":"P1v2","enabled":true,"interval":60,"auth":{"type":"none"},"sources":[{"name":"balance","url":"http://x","mapping":{"balance.amount":"$.a"}}]}`)))
	req3 := httptest.NewRequest(http.MethodGet, "/api/generic/platforms", nil)
	w3 := httptest.NewRecorder()
	h.ListPlatforms(w3, req3)
	json.Unmarshal(w3.Body.Bytes(), &list)
	if len(list) != 1 || list[0].DisplayName != "P1v2" {
		t.Errorf("update should overwrite, list = %+v", list)
	}
}

func TestHandler_Validation(t *testing.T) {
	store := &mockStore{}
	agent := NewAgent(store, nil, nil)
	h := NewHandler(agent, store)

	// 空名称 → 400
	w := httptest.NewRecorder()
	h.UpsertPlatform(w, httptest.NewRequest(http.MethodPost, "/api/generic/platforms", strings.NewReader(`{"name":"","sources":[]}`)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty name status = %d, want 400", w.Code)
	}

	// 无数据源 → 400
	w2 := httptest.NewRecorder()
	h.UpsertPlatform(w2, httptest.NewRequest(http.MethodPost, "/api/generic/platforms", strings.NewReader(`{"name":"p","sources":[]}`)))
	if w2.Code != http.StatusBadRequest {
		t.Errorf("no sources status = %d, want 400", w2.Code)
	}
}

func TestHandler_DeleteNotFound(t *testing.T) {
	store := &mockStore{}
	agent := NewAgent(store, nil, nil)
	h := NewHandler(agent, store)

	w := httptest.NewRecorder()
	h.DeletePlatform(w, httptest.NewRequest(http.MethodDelete, "/api/generic/platforms/nonexistent", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("delete missing status = %d, want 404", w.Code)
	}
}

func TestHandler_GetMetrics(t *testing.T) {
	store := &mockStore{}
	agent := NewAgent(store, nil, nil)
	h := NewHandler(agent, store)

	// 无数据 → 空数组
	w := httptest.NewRecorder()
	h.GetMetrics(w, httptest.NewRequest(http.MethodGet, "/api/generic/metrics", nil))
	if w.Code != http.StatusOK {
		t.Errorf("metrics status = %d, want 200", w.Code)
	}
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("empty metrics = %s, want []", w.Body.String())
	}

	// 有快照
	agent.saveSnapshot(&PlatformSnapshot{Platform: "p1", Status: StatusOK, Metrics: &Metrics{}})
	w2 := httptest.NewRecorder()
	h.GetMetrics(w2, httptest.NewRequest(http.MethodGet, "/api/generic/metrics", nil))
	if !strings.Contains(w2.Body.String(), "p1") {
		t.Errorf("metrics should contain p1: %s", w2.Body.String())
	}

	// 单平台
	w3 := httptest.NewRecorder()
	h.GetPlatformMetrics(w3, httptest.NewRequest(http.MethodGet, "/api/generic/metrics/p1", nil))
	if w3.Code != http.StatusOK {
		t.Errorf("single metrics status = %d, want 200", w3.Code)
	}
	// 不存在
	w4 := httptest.NewRecorder()
	h.GetPlatformMetrics(w4, httptest.NewRequest(http.MethodGet, "/api/generic/metrics/nope", nil))
	if w4.Code != http.StatusNotFound {
		t.Errorf("missing single metrics status = %d, want 404", w4.Code)
	}
}

func TestPollPlatform_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"balance":8,"currency":"USD"}}`))
	}))
	defer server.Close()

	store := &mockStore{}
	agent := NewAgent(store, nil, nil)
	p := PlatformConfig{
		Name:    "ok_platform",
		Enabled: true,
		Auth:    AuthConfig{Type: AuthTypeNone},
		Sources: []SourceConfig{{
			Name:    SourceBalance,
			URL:     server.URL,
			Mapping: map[string]string{"balance.amount": "$.data.balance", "balance.currency": "$.data.currency"},
		}},
	}
	agent.pollPlatform(context.Background(), p)
	snap, ok := agent.GetSnapshot("ok_platform")
	if !ok {
		t.Fatal("snapshot not saved")
	}
	if snap.Status != StatusOK {
		t.Errorf("status = %s, want ok", snap.Status)
	}
	if snap.Metrics.Balance == nil || snap.Metrics.Balance.Amount != 8 {
		t.Errorf("balance = %+v", snap.Metrics.Balance)
	}
}

func TestSaveAndFindPlatform(t *testing.T) {
	store := &mockStore{}
	platforms := []PlatformConfig{{Name: "a", DisplayName: "A"}, {Name: "b", DisplayName: "B"}}
	if err := SavePlatforms(store, platforms); err != nil {
		t.Fatalf("SavePlatforms failed: %v", err)
	}
	loaded, err := LoadPlatforms(store)
	if err != nil || len(loaded) != 2 {
		t.Fatalf("LoadPlatforms = %+v, %v", loaded, err)
	}
	if p := FindPlatform(loaded, "b"); p == nil || p.DisplayName != "B" {
		t.Errorf("FindPlatform b = %+v", p)
	}
	if p := FindPlatform(loaded, "zzz"); p != nil {
		t.Errorf("FindPlatform zzz should be nil")
	}
}

func TestHandler_TestConnectionHandler(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"balance":3,"currency":"CNY"}}`))
	}))
	defer server.Close()

	store := &mockStore{}
	agent := NewAgent(store, nil, nil)
	h := NewHandler(agent, store)

	body := `{"name":"t","auth":{"type":"none"},"sources":[{"name":"balance","url":"` + server.URL + `","mapping":{"balance.amount":"$.data.balance"}}]}`
	w := httptest.NewRecorder()
	h.TestConnection(w, httptest.NewRequest(http.MethodPost, "/api/generic/test", strings.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Errorf("test status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Errorf("test result = %s", w.Body.String())
	}
}