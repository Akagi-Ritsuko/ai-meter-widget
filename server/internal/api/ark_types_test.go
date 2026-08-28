package api

import (
	"encoding/json"
	"testing"
	"time"
)

func TestArkGetAFPUsageResponseToSnapshotStringNumbers(t *testing.T) {
	const raw = `{
		"ResponseMetadata": {
			"RequestId": "req-123",
			"Action": "GetAFPUsage",
			"Version": "2024-01-01",
			"Service": "ark",
			"Region": "cn-beijing"
		},
		"Result": {
			"PlanType": "agent",
			"AFPFiveHour": {"Quota": "500", "Used": "120", "SubscribeTime": 1720000000000, "ResetTime": 1720080000000},
			"AFPDaily":    {"Quota": "1000", "Used": "400", "SubscribeTime": 1720000000000, "ResetTime": 1720080000000},
			"AFPWeekly":   {"Quota": "7000", "Used": "2100", "SubscribeTime": 1720000000000, "ResetTime": 1720080000000},
			"AFPMonthly":  {"Quota": "30000", "Used": "9000", "SubscribeTime": 1720000000000, "ResetTime": 1720080000000}
		}
	}`

	var resp ArkGetAFPUsageResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	snap := resp.ToSnapshot(now)

	if snap.PlanType != "agent" {
		t.Errorf("PlanType = %q, want %q", snap.PlanType, "agent")
	}
	if len(snap.Windows) != 4 {
		t.Fatalf("windows = %d, want 4", len(snap.Windows))
	}

	wantOrder := []string{"five_hour", "daily", "weekly", "monthly"}
	for i, name := range wantOrder {
		if snap.Windows[i].Name != name {
			t.Errorf("window[%d].Name = %q, want %q", i, snap.Windows[i].Name, name)
		}
	}

	fiveHour := snap.Windows[0]
	if fiveHour.Quota != 500 || fiveHour.Used != 120 {
		t.Errorf("five_hour quota/used = %v/%v, want 500/120", fiveHour.Quota, fiveHour.Used)
	}
	if fiveHour.Percent != 24 {
		t.Errorf("five_hour percent = %v, want 24", fiveHour.Percent)
	}
	if fiveHour.ResetsAt == nil || fiveHour.ResetsAt.UnixMilli() != 1720080000000 {
		t.Errorf("five_hour resetsAt = %v, want epoch ms 1720080000000", fiveHour.ResetsAt)
	}
	if fiveHour.SubscribeAt == nil || fiveHour.SubscribeAt.UnixMilli() != 1720000000000 {
		t.Errorf("five_hour subscribeAt = %v, want epoch ms 1720000000000", fiveHour.SubscribeAt)
	}

	monthly := snap.Windows[3]
	if monthly.Percent != 30 {
		t.Errorf("monthly percent = %v, want 30", monthly.Percent)
	}
}

func TestArkGetAFPUsageResponseToSnapshotNumberNumbers(t *testing.T) {
	const raw = `{
		"ResponseMetadata": {},
		"Result": {
			"PlanType": "agent",
			"AFPDaily": {"Quota": 1000, "Used": 250, "SubscribeTime": 1720000000000, "ResetTime": 1720080000000},
			"AFPFiveHour": {"Quota": 500, "Used": 50, "SubscribeTime": 1720000000000, "ResetTime": 1720080000000},
			"AFPWeekly": {"Quota": 7000, "Used": 0, "SubscribeTime": 1720000000000, "ResetTime": 1720080000000},
			"AFPMonthly": {"Quota": 30000, "Used": 15000, "SubscribeTime": 1720000000000, "ResetTime": 1720080000000}
		}
	}`

	var resp ArkGetAFPUsageResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	snap := resp.ToSnapshot(time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC))

	daily := snap.Windows[1]
	if daily.Quota != 1000 || daily.Used != 250 {
		t.Errorf("daily quota/used = %v/%v, want 1000/250", daily.Quota, daily.Used)
	}
	if daily.Percent != 25 {
		t.Errorf("daily percent = %v, want 25", daily.Percent)
	}
	// Zero-used window still parsed, percent zero.
	weekly := snap.Windows[2]
	if weekly.Percent != 0 {
		t.Errorf("weekly percent = %v, want 0", weekly.Percent)
	}
}

func TestArkGetAFPUsageResponseToSnapshotZeroQuotaKept(t *testing.T) {
	const raw = `{
		"ResponseMetadata": {},
		"Result": {
			"PlanType": "agent",
			"AFPDaily": {"Quota": 0, "Used": 0},
			"AFPFiveHour": {"Quota": 500, "Used": 100, "SubscribeTime": 0, "ResetTime": 0},
			"AFPWeekly": {"Quota": 0, "Used": 0},
			"AFPMonthly": {"Quota": 0, "Used": 0}
		}
	}`

	var resp ArkGetAFPUsageResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	snap := resp.ToSnapshot(time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC))

	if len(snap.Windows) != 4 {
		t.Fatalf("windows = %d, want 4 (zero-quota windows must be kept)", len(snap.Windows))
	}
	if snap.Windows[0].Quota != 500 {
		t.Errorf("five_hour quota = %v, want 500", snap.Windows[0].Quota)
	}
	if snap.Windows[1].Quota != 0 {
		t.Errorf("daily quota = %v, want 0", snap.Windows[1].Quota)
	}
	if snap.Windows[0].ResetsAt != nil || snap.Windows[0].SubscribeAt != nil {
		t.Errorf("zero timestamps must map to nil pointers, got %v / %v", snap.Windows[0].ResetsAt, snap.Windows[0].SubscribeAt)
	}
}

func TestArkGetAFPUsageResponseToSnapshotMissingWindows(t *testing.T) {
	// Quota/Used tolerate both string and number representations (D6); when a
	// window is absent or null the client still produces a zero-valued window
	// instead of failing the whole snapshot.
	const raw = `{
		"ResponseMetadata": {},
		"Result": {
			"PlanType": "agent",
			"AFPDaily": null,
			"AFPFiveHour": {"Quota": 500, "Used": 50},
			"AFPWeekly": {},
			"AFPMonthly": null
		}
	}`

	var resp ArkGetAFPUsageResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	snap := resp.ToSnapshot(time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC))

	if len(snap.Windows) != 4 {
		t.Fatalf("windows = %d, want 4 (null/missing windows must be kept)", len(snap.Windows))
	}
	if snap.Windows[1].Quota != 0 || snap.Windows[1].Used != 0 {
		t.Errorf("missing window must degrade to zero, got %v/%v", snap.Windows[1].Quota, snap.Windows[1].Used)
	}
	if snap.Windows[0].Quota != 500 {
		t.Errorf("valid window must survive, quota = %v", snap.Windows[0].Quota)
	}
}

func TestArkQuotaWindowJSONNumber(t *testing.T) {
	var w ArkQuotaWindow
	if err := json.Unmarshal([]byte(`{"Quota": "500", "Used": 123}`), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	q, _ := w.Quota.Float64()
	u, _ := w.Used.Float64()
	if q != 500 || u != 123 {
		t.Errorf("quota/used = %v/%v, want 500/123", q, u)
	}
}