package store

import (
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
)

func newArkTestSnapshot(capturedAt time.Time, planType string, windows []api.ArkWindowSnapshot) *api.ArkSnapshot {
	return &api.ArkSnapshot{
		CapturedAt: capturedAt,
		RawJSON:    `{"raw":"true"}`,
		PlanType:   planType,
		Windows:    windows,
	}
}

func arkWindows(quota, used float64, resetsAt, subscribeAt *time.Time) []api.ArkWindowSnapshot {
	return []api.ArkWindowSnapshot{
		{Name: "five_hour", Quota: quota, Used: used, Percent: used / quota * 100, ResetsAt: resetsAt, SubscribeAt: subscribeAt},
		{Name: "daily", Quota: quota * 2, Used: used, Percent: used / (quota * 2) * 100, ResetsAt: resetsAt, SubscribeAt: subscribeAt},
	}
}

func TestArkStore_InsertAndQueryLatest(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	reset := now.Add(5 * time.Hour)
	subscribe := now.Add(-24 * time.Hour)
	snap := newArkTestSnapshot(now, "agent", arkWindows(500, 120, &reset, &subscribe))

	id, err := s.InsertArkSnapshot(snap)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if id == 0 {
		t.Error("expected id > 0")
	}

	latest, err := s.QueryLatestArk()
	if err != nil {
		t.Fatalf("query latest: %v", err)
	}
	if latest == nil {
		t.Fatal("latest = nil")
	}
	if latest.PlanType != "agent" {
		t.Errorf("PlanType = %q, want agent", latest.PlanType)
	}
	if len(latest.Windows) != 2 {
		t.Fatalf("windows = %d, want 2", len(latest.Windows))
	}
	w := latest.Windows[0]
	if w.Name != "five_hour" || w.Quota != 500 || w.Used != 120 || w.Percent != 24 {
		t.Errorf("window mismatch: %+v", w)
	}
	if w.ResetsAt == nil || !w.ResetsAt.Equal(reset) {
		t.Errorf("resetsAt = %v, want %v", w.ResetsAt, reset)
	}
	if w.SubscribeAt == nil || !w.SubscribeAt.Equal(subscribe) {
		t.Errorf("subscribeAt = %v, want %v", w.SubscribeAt, subscribe)
	}
}

func TestArkStore_QueryRangeAndLatests(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < 3; i++ {
		snap := newArkTestSnapshot(base.Add(time.Duration(i)*time.Minute), "agent", arkWindows(500, float64(i*10), nil, nil))
		if _, err := s.InsertArkSnapshot(snap); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	snaps, err := s.QueryArkRange(base.Add(-time.Minute), time.Now().UTC())
	if err != nil {
		t.Fatalf("query range: %v", err)
	}
	if len(snaps) != 3 {
		t.Fatalf("snapshots = %d, want 3", len(snaps))
	}
	if len(snaps[0].Windows) != 2 {
		t.Fatalf("first snapshot windows = %d, want 2", len(snaps[0].Windows))
	}

	latestPerQuota, err := s.QueryArkLatestPerQuota()
	if err != nil {
		t.Fatalf("latest per quota: %v", err)
	}
	if len(latestPerQuota) != 2 {
		t.Fatalf("latest per quota = %d, want 2", len(latestPerQuota))
	}
	for _, q := range latestPerQuota {
		if q.Used != 20 {
			t.Errorf("quota %s used = %v, want 20 (latest snapshot)", q.Name, q.Used)
		}
		if q.PlanType != "agent" {
			t.Errorf("quota %s planType = %q, want agent", q.Name, q.PlanType)
		}
	}

	series, err := s.QueryArkUtilizationSeries("five_hour", base.Add(-time.Minute))
	if err != nil {
		t.Fatalf("utilization series: %v", err)
	}
	if len(series) != 3 {
		t.Fatalf("series = %d, want 3", len(series))
	}
}

func TestArkStore_CycleLifecycle(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	start := time.Now().UTC().Add(-24 * time.Hour)
	resetAt := start.Add(24 * time.Hour)

	id, err := s.CreateArkCycle("five_hour", start, nil)
	if err != nil {
		t.Fatalf("create cycle: %v", err)
	}
	if id == 0 {
		t.Error("expected cycle id > 0")
	}

	if err := s.UpdateArkCycle("five_hour", 30, 15); err != nil {
		t.Fatalf("update cycle: %v", err)
	}

	active, err := s.QueryActiveArkCycle("five_hour")
	if err != nil {
		t.Fatalf("active cycle: %v", err)
	}
	if active == nil {
		t.Fatal("active cycle = nil")
	}
	if active.PeakUtilization != 30 || active.TotalDelta != 15 {
		t.Errorf("active peak/delta = %v/%v, want 30/15", active.PeakUtilization, active.TotalDelta)
	}
	if active.CycleEnd != nil {
		t.Errorf("active cycle must have nil end")
	}

	end := resetAt
	if err := s.CloseArkCycle("five_hour", end, 40, 25); err != nil {
		t.Fatalf("close cycle: %v", err)
	}

	active, err = s.QueryActiveArkCycle("five_hour")
	if err != nil {
		t.Fatalf("active after close: %v", err)
	}
	if active != nil {
		t.Errorf("active after close = %+v, want nil", active)
	}

	history, err := s.QueryArkCycleHistory("five_hour")
	if err != nil {
		t.Fatalf("cycle history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history = %d, want 1", len(history))
	}
	if history[0].CycleEnd == nil || !history[0].CycleEnd.Equal(end) {
		t.Errorf("history end = %v, want %v", history[0].CycleEnd, end)
	}
	if history[0].PeakUtilization != 40 || history[0].TotalDelta != 25 {
		t.Errorf("history peak/delta = %v/%v, want 40/25", history[0].PeakUtilization, history[0].TotalDelta)
	}

	names, err := s.QueryAllArkQuotaNames()
	if err != nil {
		t.Fatalf("quota names: %v", err)
	}
	if len(names) != 1 || names[0] != "five_hour" {
		t.Errorf("quota names = %v, want [five_hour]", names)
	}

	since, err := s.QueryArkCyclesSince("five_hour", start.Add(-time.Minute))
	if err != nil {
		t.Fatalf("cycles since: %v", err)
	}
	if len(since) != 1 {
		t.Errorf("cycles since = %d, want 1", len(since))
	}
}

func TestArkStore_CycleOverview(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	base := time.Now().UTC().Add(-48 * time.Hour)
	// Snapshot inside the cycle window with utilization data.
	for i := 0; i < 3; i++ {
		snap := newArkTestSnapshot(base.Add(time.Duration(i)*time.Hour), "agent", arkWindows(500, float64(10+i*5), nil, nil))
		if _, err := s.InsertArkSnapshot(snap); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	cycleStart := base.Add(-time.Hour)
	resetAt := base.Add(24 * time.Hour)
	if _, err := s.CreateArkCycle("five_hour", cycleStart, &resetAt); err != nil {
		t.Fatalf("create cycle: %v", err)
	}

	overview, err := s.QueryArkCycleOverview("five_hour", 10)
	if err != nil {
		t.Fatalf("cycle overview: %v", err)
	}
	if len(overview) != 1 {
		t.Fatalf("overview = %d, want 1", len(overview))
	}
	row := overview[0]
	if row.QuotaType != "five_hour" {
		t.Errorf("QuotaType = %q", row.QuotaType)
	}
	if row.PeakTime.IsZero() {
		t.Error("PeakTime must be populated from snapshot with max utilization")
	}
	if len(row.CrossQuotas) != 2 {
		t.Errorf("CrossQuotas = %d, want 2", len(row.CrossQuotas))
	}
}