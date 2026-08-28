package agent

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
	"github.com/onllm-dev/onwatch/v2/internal/tracker"
)

const (
	arkAgentTestAK = "test-ak-agent"
	arkAgentTestSK = "test-sk-agent"
)

func newArkAgentTestClient(t *testing.T, srv *httptest.Server) *api.ArkClient {
	t.Helper()
	logBuf := &safeBuffer{}
	return api.NewArkClient(arkAgentTestAK, arkAgentTestSK, slog.New(slog.NewTextHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})), api.WithArkBaseURL(srv.URL))
}

func TestNewArkAgent_Basic(t *testing.T) {
	a := NewArkAgent(nil, nil, nil, 60*time.Second, nil, nil)
	if a == nil {
		t.Fatal("nil agent")
	}
	a.SetPollingCheck(func() bool { return true })
	a.SetNotifier(nil)
}

func TestArkAgent_Poll_NoClientSafe(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer st.Close()

	tr := tracker.NewArkTracker(st, nil)
	ag := NewArkAgent(nil, st, tr, time.Second, nil, NewSessionManager(st, "ark", 60*time.Second, nil))
	ag.poll(context.Background())
}

func TestArkAgent_Poll_FetchErrorNoInsert(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer st.Close()

	client := newArkAgentTestClient(t, srv)
	tr := tracker.NewArkTracker(st, nil)
	ag := NewArkAgent(client, st, tr, time.Second, slog.Default(), NewSessionManager(st, "ark", 60*time.Second, nil))

	ag.poll(context.Background())

	latest, err := st.QueryLatestArk()
	if err != nil {
		t.Fatalf("QueryLatestArk: %v", err)
	}
	if latest != nil {
		t.Fatal("expected no snapshot after fetch failure")
	}
}

func TestArkAgent_Poll_SuccessInsertsAndTracks(t *testing.T) {
	body := `{"ResponseMetadata":{"RequestId":"r1"},"Result":{"PlanType":"agent",` +
		`"AFPFiveHour":{"Quota":"500","Used":"50","SubscribeTime":1720000000000,"ResetTime":1720080000000},` +
		`"AFPDaily":{"Quota":"1000","Used":"250","SubscribeTime":1720000000000,"ResetTime":1720080000000},` +
		`"AFPWeekly":{"Quota":"7000","Used":"0","SubscribeTime":1720000000000,"ResetTime":1720080000000},` +
		`"AFPMonthly":{"Quota":"30000","Used":"0","SubscribeTime":1720000000000,"ResetTime":1720080000000}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	}))
	defer srv.Close()

	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer st.Close()

	client := newArkAgentTestClient(t, srv)
	tr := tracker.NewArkTracker(st, slog.Default())
	ag := NewArkAgent(client, st, tr, time.Second, slog.Default(), NewSessionManager(st, "ark", 60*time.Second, nil))

	ag.poll(context.Background())

	latest, err := st.QueryLatestArk()
	if err != nil {
		t.Fatalf("QueryLatestArk: %v", err)
	}
	if latest == nil {
		t.Fatal("expected inserted snapshot")
	}
	if len(latest.Windows) != 4 {
		t.Fatalf("windows = %d, want 4", len(latest.Windows))
	}
	if latest.PlanType != "agent" {
		t.Errorf("PlanType = %q, want agent", latest.PlanType)
	}

	// Tracker must have created cycles for windows with positive usage.
	cycle, err := st.QueryActiveArkCycle("daily")
	if err != nil {
		t.Fatalf("QueryActiveArkCycle: %v", err)
	}
	if cycle == nil {
		t.Fatal("expected active cycle after tracker.Process")
	}
}

func TestArkAgent_Poll_PollingCheckDisabled(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ResponseMetadata":{},"Result":{}}`))
	}))
	defer srv.Close()

	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer st.Close()

	client := newArkAgentTestClient(t, srv)
	tr := tracker.NewArkTracker(st, nil)
	ag := NewArkAgent(client, st, tr, time.Second, slog.Default(), nil)
	ag.SetPollingCheck(func() bool { return false })

	ag.poll(context.Background())

	if callCount != 0 {
		t.Fatalf("server called %d times, want 0 when polling disabled", callCount)
	}
}

// safeBuffer is a minimal concurrency-safe logger sink placeholder; the agent
// tests only assert on store state, not on log output.
type safeBuffer struct {
	bytes []byte
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.bytes = append(b.bytes, p...)
	return len(p), nil
}

func (b *safeBuffer) String() string { return string(b.bytes) }