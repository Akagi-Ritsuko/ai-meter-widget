package tracker

import (
	"log/slog"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

func newTestArkStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newArkSnap(capturedAt time.Time, windows []api.ArkWindowSnapshot) *api.ArkSnapshot {
	return &api.ArkSnapshot{
		CapturedAt: capturedAt,
		PlanType:   "agent",
		Windows:    windows,
	}
}

func TestArkTracker_Process_FirstSnapshot(t *testing.T) {
	s := newTestArkStore(t)
	tr := NewArkTracker(s, slog.Default())

	now := time.Now().UTC()
	resetsAt := now.Add(5 * time.Hour)

	snap := newArkSnap(now, []api.ArkWindowSnapshot{
		{Name: "five_hour", Quota: 100, Used: 12.5, Percent: 12.5, ResetsAt: &resetsAt},
	})

	if err := tr.Process(snap); err != nil {
		t.Fatalf("Process: %v", err)
	}

	cycle, err := s.QueryActiveArkCycle("five_hour")
	if err != nil {
		t.Fatalf("QueryActiveArkCycle: %v", err)
	}
	if cycle == nil {
		t.Fatal("expected active cycle after first snapshot")
	}
	if cycle.PeakUtilization != 12.5 {
		t.Errorf("PeakUtilization = %f, want 12.5", cycle.PeakUtilization)
	}
}

func TestArkTracker_Process_UsageIncrease(t *testing.T) {
	s := newTestArkStore(t)
	tr := NewArkTracker(s, slog.Default())

	now := time.Now().UTC()
	resetsAt := now.Add(5 * time.Hour)

	snap1 := newArkSnap(now, []api.ArkWindowSnapshot{
		{Name: "daily", Quota: 100, Used: 12.5, Percent: 12.5, ResetsAt: &resetsAt},
	})
	if err := tr.Process(snap1); err != nil {
		t.Fatalf("Process snap1: %v", err)
	}

	snap2 := newArkSnap(now.Add(time.Minute), []api.ArkWindowSnapshot{
		{Name: "daily", Quota: 100, Used: 25, Percent: 25, ResetsAt: &resetsAt},
	})
	if err := tr.Process(snap2); err != nil {
		t.Fatalf("Process snap2: %v", err)
	}

	cycle, err := s.QueryActiveArkCycle("daily")
	if err != nil {
		t.Fatalf("QueryActiveArkCycle: %v", err)
	}
	if cycle == nil {
		t.Fatal("expected active cycle")
	}
	if cycle.PeakUtilization != 25.0 {
		t.Errorf("PeakUtilization = %f, want 25.0", cycle.PeakUtilization)
	}
	if cycle.TotalDelta != 12.5 {
		t.Errorf("TotalDelta = %f, want 12.5", cycle.TotalDelta)
	}
}

func TestArkTracker_Process_ResetDetection(t *testing.T) {
	s := newTestArkStore(t)
	tr := NewArkTracker(s, slog.Default())

	now := time.Now().UTC()
	oldReset := now.Add(1 * time.Hour)
	newReset := now.Add(6 * time.Hour)

	snap1 := newArkSnap(now, []api.ArkWindowSnapshot{
		{Name: "weekly", Quota: 100, Used: 40, Percent: 40, ResetsAt: &oldReset},
	})
	if err := tr.Process(snap1); err != nil {
		t.Fatalf("Process snap1: %v", err)
	}

	snap2 := newArkSnap(now.Add(2*time.Hour), []api.ArkWindowSnapshot{
		{Name: "weekly", Quota: 100, Used: 5, Percent: 5, ResetsAt: &newReset},
	})
	if err := tr.Process(snap2); err != nil {
		t.Fatalf("Process snap2: %v", err)
	}

	history, err := s.QueryArkCycleHistory("weekly")
	if err != nil {
		t.Fatalf("QueryArkCycleHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("completed cycles = %d, want 1", len(history))
	}

	active, err := s.QueryActiveArkCycle("weekly")
	if err != nil {
		t.Fatalf("QueryActiveArkCycle: %v", err)
	}
	if active == nil {
		t.Fatal("expected new active cycle after reset")
	}
}

func TestArkTracker_Process_IgnoresFallbackResetTimeDrift(t *testing.T) {
	s := newTestArkStore(t)
	tr := NewArkTracker(s, slog.Default())

	now := time.Now().UTC()
	resetsAt := now.Add(7 * 24 * time.Hour)
	snap1 := newArkSnap(now, []api.ArkWindowSnapshot{
		{Name: "weekly", Quota: 100, Used: 30, Percent: 30, ResetsAt: &resetsAt},
	})
	if err := tr.Process(snap1); err != nil {
		t.Fatalf("Process snap1: %v", err)
	}

	driftedReset := resetsAt.Add(-59 * time.Minute)
	snap2 := newArkSnap(now.Add(time.Minute), []api.ArkWindowSnapshot{
		{Name: "weekly", Quota: 100, Used: 31, Percent: 31, ResetsAt: &driftedReset},
	})
	if err := tr.Process(snap2); err != nil {
		t.Fatalf("Process snap2: %v", err)
	}

	history, err := s.QueryArkCycleHistory("weekly")
	if err != nil {
		t.Fatalf("QueryArkCycleHistory: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("completed cycles = %d, want 0", len(history))
	}
}

func TestArkTracker_Process_IgnoresExpiredStoredResetWithinDriftTolerance(t *testing.T) {
	s := newTestArkStore(t)
	tr := NewArkTracker(s, slog.Default())

	now := time.Now().UTC()
	storedReset := now.Add(time.Hour)
	snap1 := newArkSnap(now, []api.ArkWindowSnapshot{
		{Name: "weekly", Quota: 100, Used: 30, Percent: 30, ResetsAt: &storedReset},
	})
	if err := tr.Process(snap1); err != nil {
		t.Fatalf("Process snap1: %v", err)
	}

	currentReset := storedReset.Add(59 * time.Minute)
	snap2 := newArkSnap(storedReset.Add(3*time.Minute), []api.ArkWindowSnapshot{
		{Name: "weekly", Quota: 100, Used: 31, Percent: 31, ResetsAt: &currentReset},
	})
	if err := tr.Process(snap2); err != nil {
		t.Fatalf("Process snap2: %v", err)
	}

	history, err := s.QueryArkCycleHistory("weekly")
	if err != nil {
		t.Fatalf("QueryArkCycleHistory: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("completed cycles = %d, want 0", len(history))
	}
}

func TestArkTracker_UsageSummary(t *testing.T) {
	s := newTestArkStore(t)
	tr := NewArkTracker(s, slog.Default())

	now := time.Now().UTC()
	resetsAt := now.Add(5 * time.Hour)
	snap := newArkSnap(now, []api.ArkWindowSnapshot{
		{Name: "five_hour", Quota: 100, Used: 20, Percent: 20, ResetsAt: &resetsAt},
	})
	if _, err := s.InsertArkSnapshot(snap); err != nil {
		t.Fatalf("InsertArkSnapshot: %v", err)
	}
	if err := tr.Process(snap); err != nil {
		t.Fatalf("Process: %v", err)
	}

	summary, err := tr.UsageSummary("five_hour")
	if err != nil {
		t.Fatalf("UsageSummary: %v", err)
	}
	if summary == nil {
		t.Fatal("expected summary")
	}
	if summary.CurrentUtil != 20 {
		t.Errorf("CurrentUtil = %f, want 20", summary.CurrentUtil)
	}
	if summary.ResetsAt == nil || !summary.ResetsAt.Equal(resetsAt) {
		t.Errorf("ResetsAt = %v, want %v", summary.ResetsAt, resetsAt)
	}
}