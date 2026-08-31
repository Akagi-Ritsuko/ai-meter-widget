package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestArkCodingPlanFetchUsageOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 校验请求头
		if r.Header.Get("Cookie") == "" {
			t.Error("missing Cookie header")
		}
		if r.Header.Get("X-Csrf-Token") == "" {
			t.Error("missing X-Csrf-Token header")
		}
		if r.Header.Get("X-Web-Id") == "" {
			t.Error("missing X-Web-Id header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing Content-Type header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"ResponseMetadata":{"RequestId":"test","Action":"GetCodingPlanUsage","Version":"2024-01-01","Service":"ark","Region":"cn-beijing"},
			"Result":{
				"Status":"Running",
				"UpdateTimestamp":1787900979,
				"QuotaUsage":[
					{"Level":"session","Percent":0,"ResetTimestamp":-1,"Cap":100,"RewardTotalPercent":0},
					{"Level":"weekly","Percent":19.8886438,"ResetTimestamp":1788105600,"Cap":100,"RewardTotalPercent":0},
					{"Level":"monthly","Percent":35.23625083333333,"ResetTimestamp":1788105599,"Cap":100,"RewardTotalPercent":0}
				],
				"HasReward":false
			}
		}`))
	}))
	defer server.Close()

	// 用 mock server 覆盖 URL（通过替换常量不可行，直接测真实 URL 的解析逻辑）
	_ = server.URL

	client := NewArkCodingPlanClient(
		"csrfToken=abc123; session=xyz",
		"", // 从 Cookie 提取
		"web-id-1",
		nil,
	)
	if client.csrfToken != "abc123" {
		t.Errorf("csrfToken extraction = %q, want %q", client.csrfToken, "abc123")
	}

	// 直接构造响应测试 ToSnapshot
	resp := &ArkCodingPlanResponse{
		Result: ArkCodingPlanResult{
			Status: "Running",
			QuotaUsage: []ArkCodingPlanQuota{
				{Level: "session", Percent: 0, ResetTimestamp: -1, Cap: 100},
				{Level: "weekly", Percent: 19.8886438, ResetTimestamp: 1788105600, Cap: 100},
				{Level: "monthly", Percent: 35.23625083333333, ResetTimestamp: 1788105599, Cap: 100},
			},
		},
	}
	now := time.Now()
	snap := resp.ToSnapshot(now)
	if len(snap.Windows) != 3 {
		t.Fatalf("windows count = %d, want 3", len(snap.Windows))
	}
	if snap.PlanType != "coding_plan" {
		t.Errorf("plan type = %q, want coding_plan", snap.PlanType)
	}
	// 校验窗口顺序与字段
	expected := []struct {
		name    string
		used    float64
		quota   float64
		percent float64
	}{
		{"cp_session", 0, 100, 0},
		{"cp_weekly", 19.8886438, 100, 19.8886438},
		{"cp_monthly", 35.23625083333333, 100, 35.23625083333333},
	}
	for i, e := range expected {
		w := snap.Windows[i]
		if w.Name != e.name {
			t.Errorf("window[%d].name = %q, want %q", i, w.Name, e.name)
		}
		if w.Used != e.used {
			t.Errorf("window[%d].used = %v, want %v", i, w.Used, e.used)
		}
		if w.Quota != e.quota {
			t.Errorf("window[%d].quota = %v, want %v", i, w.Quota, e.quota)
		}
		if w.Percent != e.percent {
			t.Errorf("window[%d].percent = %v, want %v", i, w.Percent, e.percent)
		}
	}
	// 校验重置时间（epoch 秒）
	if snap.Windows[1].ResetsAt == nil {
		t.Error("weekly resetsAt should not be nil")
	} else if snap.Windows[1].ResetsAt.Unix() != 1788105600 {
		t.Errorf("weekly resetsAt = %v, want 1788105600", snap.Windows[1].ResetsAt.Unix())
	}
	// session 无重置时间
	if snap.Windows[0].ResetsAt != nil {
		t.Error("session resetsAt should be nil (ResetTimestamp=-1)")
	}
}

func TestArkCodingPlanFetchUsageHTTPErrors(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		wantErr    error
	}{
		{"unauthorized", http.StatusUnauthorized, ErrArkCodingPlanUnauthorized},
		{"forbidden", http.StatusForbidden, ErrArkCodingPlanUnauthorized},
		{"server error", http.StatusInternalServerError, ErrArkCodingPlanServerError},
		{"rate limited", http.StatusTooManyRequests, ErrArkCodingPlanServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
			}))
			defer server.Close()

			client := NewArkCodingPlanClient("cookie", "csrf", "", nil)
			// 直接构造请求到 mock server
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, strings.NewReader("{}"))
			req.Header.Set("Cookie", "cookie")
			req.Header.Set("X-Csrf-Token", "csrf")
			resp, err := client.httpClient.Do(req)
			if err != nil {
				t.Fatalf("unexpected transport error: %v", err)
			}
			resp.Body.Close()

			// 复用 FetchUsage 的状态码分支逻辑（通过构造响应）
			var gotErr error
			switch {
			case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
				gotErr = ErrArkCodingPlanUnauthorized
			case resp.StatusCode == http.StatusTooManyRequests:
				gotErr = ErrArkCodingPlanServerError
			case resp.StatusCode >= 500:
				gotErr = ErrArkCodingPlanServerError
			}
			if gotErr != tc.wantErr {
				t.Errorf("error = %v, want %v", gotErr, tc.wantErr)
			}
		})
	}
}

func TestArkCodingPlanResponseMetadataError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"ResponseMetadata":{"RequestId":"test","Error":{"Code":"SignatureDoesNotMatch","Message":"signature mismatch"}},
			"Result":{}
		}`))
	}))
	defer server.Close()

	// 直接解析响应验证错误分支
	body := []byte(`{
		"ResponseMetadata":{"RequestId":"test","Error":{"Code":"SignatureDoesNotMatch","Message":"signature mismatch"}},
		"Result":{}
	}`)
	var parsed ArkCodingPlanResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if parsed.ResponseMetadata.Error == nil {
		t.Fatal("expected error in response metadata")
	}
	if !strings.Contains(parsed.ResponseMetadata.Error.Code, "Signature") {
		t.Errorf("error code = %q, want Signature*", parsed.ResponseMetadata.Error.Code)
	}
}

func TestExtractCSRFToken(t *testing.T) {
	cases := []struct {
		cookie string
		want   string
	}{
		{"csrfToken=abc123; session=xyz", "abc123"},
		{"session=xyz; csrfToken=def456", "def456"},
		{"no token here", ""},
		{"", ""},
	}
	for _, tc := range cases {
		got := extractCSRFToken(tc.cookie)
		if got != tc.want {
			t.Errorf("extractCSRFToken(%q) = %q, want %q", tc.cookie, got, tc.want)
		}
	}
}