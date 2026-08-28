package api

import (
	"encoding/json"
	"time"
)

// arkWindowOrder defines the canonical display order of Ark quota windows.
var arkWindowOrder = []struct {
	Name string
	Get  func(r *ArkGetAFPUsageResponse) ArkQuotaWindow
}{
	{"five_hour", func(r *ArkGetAFPUsageResponse) ArkQuotaWindow { return r.Result.AFPFiveHour }},
	{"daily", func(r *ArkGetAFPUsageResponse) ArkQuotaWindow { return r.Result.AFPDaily }},
	{"weekly", func(r *ArkGetAFPUsageResponse) ArkQuotaWindow { return r.Result.AFPWeekly }},
	{"monthly", func(r *ArkGetAFPUsageResponse) ArkQuotaWindow { return r.Result.AFPMonthly }},
}

// ArkErrorInfo mirrors the error object Volcengine returns inside
// ResponseMetadata when the request itself fails.
type ArkErrorInfo struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

// ArkResponseMetadata mirrors Volcengine's standard response metadata.
type ArkResponseMetadata struct {
	RequestID string       `json:"RequestId"`
	Action    string       `json:"Action"`
	Version   string       `json:"Version"`
	Service   string       `json:"Service"`
	Region    string       `json:"Region"`
	Error     *ArkErrorInfo `json:"Error,omitempty"`
}

// ArkQuotaWindow is one rolling quota window of the GetAFPUsage result.
//
// Quota and Used are json.Number because the official docs describe them as
// strings while some API Explorer samples return numbers - both must parse.
type ArkQuotaWindow struct {
	Quota         json.Number `json:"Quota"`
	Used          json.Number `json:"Used"`
	SubscribeTime int64       `json:"SubscribeTime"`
	ResetTime     int64       `json:"ResetTime"`
}

// ArkGetAFPUsageResult holds the four rolling windows for an Agent Plan.
type ArkGetAFPUsageResult struct {
	PlanType    string         `json:"PlanType"`
	AFPDaily    ArkQuotaWindow `json:"AFPDaily"`
	AFPFiveHour ArkQuotaWindow `json:"AFPFiveHour"`
	AFPWeekly   ArkQuotaWindow `json:"AFPWeekly"`
	AFPMonthly  ArkQuotaWindow `json:"AFPMonthly"`
}

// ArkGetAFPUsageResponse is the full GetAFPUsage API response.
type ArkGetAFPUsageResponse struct {
	ResponseMetadata ArkResponseMetadata  `json:"ResponseMetadata"`
	Result           ArkGetAFPUsageResult `json:"Result"`
}

// ArkWindowSnapshot is the normalized form of one quota window used by the
// rest of the platform (tracker, store, web).
type ArkWindowSnapshot struct {
	Name        string
	Quota       float64
	Used        float64
	Percent     float64
	ResetsAt    *time.Time
	SubscribeAt *time.Time
}

// ArkSnapshot is the normalized snapshot of all four quota windows.
type ArkSnapshot struct {
	ID         int64
	CapturedAt time.Time
	RawJSON    string
	PlanType   string
	Windows    []ArkWindowSnapshot
}

// ToSnapshot normalizes the API response into an ArkSnapshot. Windows with a
// zero quota are kept (so the panel can distinguish unconfigured states), and
// unparsable numbers degrade to zero instead of failing the whole snapshot.
func (r *ArkGetAFPUsageResponse) ToSnapshot(now time.Time) *ArkSnapshot {
	snap := &ArkSnapshot{
		CapturedAt: now,
		PlanType:   r.Result.PlanType,
	}
	for _, item := range arkWindowOrder {
		quota, _ := item.Get(r).Quota.Float64()
		used, _ := item.Get(r).Used.Float64()
		ws := ArkWindowSnapshot{
			Name:  item.Name,
			Quota: quota,
			Used:  used,
		}
		if quota > 0 {
			ws.Percent = used / quota * 100
		}
		if item.Get(r).SubscribeTime > 0 {
			t := time.UnixMilli(item.Get(r).SubscribeTime)
			ws.SubscribeAt = &t
		}
		if item.Get(r).ResetTime > 0 {
			t := time.UnixMilli(item.Get(r).ResetTime)
			ws.ResetsAt = &t
		}
		snap.Windows = append(snap.Windows, ws)
	}
	return snap
}