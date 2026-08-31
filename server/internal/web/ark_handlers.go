package web

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/tracker"
)

// SetArkTracker sets the Ark tracker for usage summary enrichment.
func (h *Handler) SetArkTracker(t *tracker.ArkTracker) {
	h.arkTracker = t
}

func (h *Handler) currentArk(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, h.buildArkCurrent())
}

// arkInsightsResponse is the JSON payload for Ark deep insights.
type arkInsightsResponse struct {
	Stats    []arkInsightStat `json:"stats"`
	Insights []insightItem    `json:"insights"`
}

// arkInsightStat is a stats-row shape that carries linked forecast metadata for the Ark dashboard.
type arkInsightStat struct {
	Value    string `json:"value"`
	Label    string `json:"label"`
	Sublabel string `json:"sublabel,omitempty"`
	Key      string `json:"key,omitempty"`
	Metric   string `json:"metric,omitempty"`
	Severity string `json:"severity,omitempty"`
	Desc     string `json:"desc,omitempty"`
}

var arkQuotaDisplayOrder = map[string]int{
	"five_hour":  1,
	"daily":      2,
	"weekly":     3,
	"monthly":    4,
	"cp_session": 5,
	"cp_weekly":  6,
	"cp_monthly": 7,
}

var arkDisplayNames = map[string]string{
	"five_hour":  "5-Hour",
	"daily":      "Daily",
	"weekly":     "Weekly",
	"monthly":    "Monthly",
	"cp_session": "Coding 5-Hour",
	"cp_weekly":  "Coding Weekly",
	"cp_monthly": "Coding Monthly",
}

func arkDisplayName(name string) string {
	if dn, ok := arkDisplayNames[name]; ok {
		return dn
	}
	return name
}

func arkQuotaOrder(name string) int {
	if order, ok := arkQuotaDisplayOrder[name]; ok {
		return order
	}
	return 99
}

type arkQuotaRate struct {
	Rate          float64
	HasRate       bool
	TimeToReset   time.Duration
	TimeToExhaust time.Duration
	ExhaustsFirst bool
	ProjectedPct  float64
}

func (h *Handler) computeArkRate(quotaName string, currentUtil float64, summary *tracker.ArkSummary) arkQuotaRate {
	var result arkQuotaRate

	if summary != nil && summary.ResetsAt != nil {
		result.TimeToReset = time.Until(*summary.ResetsAt)
	}

	if h.store != nil {
		points, err := h.store.QueryArkUtilizationSeries(quotaName, time.Now().Add(-30*time.Minute))
		if err == nil && len(points) >= 2 {
			first := points[0]
			last := points[len(points)-1]
			elapsed := last.CapturedAt.Sub(first.CapturedAt)
			if elapsed >= 5*time.Minute {
				delta := last.Utilization - first.Utilization
				if delta > 0 {
					result.Rate = delta / elapsed.Hours()
					result.HasRate = true
				} else {
					result.HasRate = true
				}
			}
		}
	}

	if !result.HasRate && summary != nil && summary.CurrentRate > 0 {
		result.Rate = summary.CurrentRate
		result.HasRate = true
	}

	if result.HasRate && result.Rate > 0 {
		remaining := 100 - currentUtil
		if remaining > 0 {
			result.TimeToExhaust = time.Duration(remaining / result.Rate * float64(time.Hour))
		}
		if result.TimeToReset > 0 {
			result.ProjectedPct = currentUtil + (result.Rate * result.TimeToReset.Hours())
			if result.ProjectedPct > 100 {
				result.ProjectedPct = 100
			}
			result.ExhaustsFirst = result.TimeToExhaust > 0 && result.TimeToExhaust < result.TimeToReset
		}
	}

	return result
}

func buildArkBurnRateInsight(w api.ArkWindowSnapshot, rate arkQuotaRate) insightItem {
	item := insightItem{
		Key:   fmt.Sprintf("forecast_%s", w.Name),
		Title: fmt.Sprintf("%s Burn Rate", arkDisplayName(w.Name)),
	}

	resetStr := ""
	if rate.TimeToReset > 0 {
		resetStr = formatDuration(rate.TimeToReset)
	}
	projected := w.Percent
	if rate.ProjectedPct > projected {
		projected = rate.ProjectedPct
	}
	sublabel := fmt.Sprintf("~%.0f%% by reset", projected)
	if resetStr != "" {
		sublabel = fmt.Sprintf("~%.0f%% by reset in %s", projected, resetStr)
	}

	if !rate.HasRate {
		item.Type = "forecast"
		item.Severity = "info"
		item.Metric = "Analyzing..."
		item.Sublabel = sublabel
		item.Desc = fmt.Sprintf("Currently at %.0f%%. Collecting more snapshots to estimate burn rate and refine reset projection.", w.Percent)
		return item
	}

	if rate.Rate < 0.01 {
		item.Type = "forecast"
		item.Severity = "positive"
		item.Metric = "Idle"
		item.Sublabel = sublabel
		item.Desc = fmt.Sprintf("Currently at %.0f%%. No meaningful burn detected recently, so this quota looks stable through the rest of the cycle.", w.Percent)
		return item
	}

	item.Type = "forecast"
	item.Metric = fmt.Sprintf("%.1f%%/hr", rate.Rate)
	if rate.ExhaustsFirst {
		exhaustStr := formatDuration(rate.TimeToExhaust)
		item.Severity = "negative"
		item.Sublabel = sublabel
		item.Desc = fmt.Sprintf("Currently at %.0f%%. At this rate, projected %.0f%% by reset and likely to exhaust in %s before reset.", w.Percent, projected, exhaustStr)
		return item
	}

	if rate.ProjectedPct >= 80 {
		item.Severity = "warning"
		item.Sublabel = sublabel
		item.Desc = fmt.Sprintf("Currently at %.0f%%. At this rate, projected %.0f%% by reset.", w.Percent, projected)
		return item
	}

	item.Severity = "positive"
	item.Sublabel = sublabel
	item.Desc = fmt.Sprintf("Currently at %.0f%%. At this rate, projected %.0f%% by reset.", w.Percent, projected)
	return item
}

func (h *Handler) buildArkCurrent() map[string]interface{} {
	now := time.Now().UTC()
	response := map[string]interface{}{
		"capturedAt": now.Format(time.RFC3339),
		"quotas":     []interface{}{},
	}

	if h.store == nil {
		return response
	}

	latest, err := h.store.QueryLatestArk()
	if err != nil || latest == nil {
		return response
	}

	response["capturedAt"] = latest.CapturedAt.Format(time.RFC3339)
	response["planType"] = latest.PlanType

	latestPerQuota, err := h.store.QueryArkLatestPerQuota()
	if err != nil || len(latestPerQuota) == 0 {
		for _, w := range latest.Windows {
			quotaMap := map[string]interface{}{
				"name":          w.Name,
				"displayName":   arkDisplayName(w.Name),
				"utilization":   w.Percent,
				"used":          w.Used,
				"limit":         w.Quota,
				"format":        "percent",
				"status":        utilStatus(w.Percent),
				"lastUpdatedAt": latest.CapturedAt.Format(time.RFC3339),
				"ageSeconds":    int64(now.Sub(latest.CapturedAt).Seconds()),
			}
			if w.ResetsAt != nil {
				timeUntilReset := time.Until(*w.ResetsAt)
				quotaMap["resetsAt"] = w.ResetsAt.Format(time.RFC3339)
				quotaMap["timeUntilReset"] = formatDuration(timeUntilReset)
				quotaMap["timeUntilResetSeconds"] = int64(timeUntilReset.Seconds())
			}
			if h.arkTracker != nil {
				if summary, sErr := h.arkTracker.UsageSummary(w.Name); sErr == nil && summary != nil {
					quotaMap["currentRate"] = summary.CurrentRate
					quotaMap["projectedUtil"] = summary.ProjectedUtil
				}
			}
			response["quotas"] = append(response["quotas"].([]interface{}), quotaMap)
		}
		applyDisplayModeToResponse(response, h.getDisplayMode("ark"))
		return response
	}

	sort.SliceStable(latestPerQuota, func(i, j int) bool {
		left := arkQuotaOrder(latestPerQuota[i].Name)
		right := arkQuotaOrder(latestPerQuota[j].Name)
		if left != right {
			return left < right
		}
		return latestPerQuota[i].Name < latestPerQuota[j].Name
	})

	var quotas []interface{}
	for _, q := range latestPerQuota {
		age := now.Sub(q.CapturedAt)
		qMap := map[string]interface{}{
			"name":          q.Name,
			"displayName":   arkDisplayName(q.Name),
			"utilization":   q.Utilization,
			"used":          q.Used,
			"limit":         q.Limit,
			"format":        "percent",
			"status":        utilStatus(q.Utilization),
			"lastUpdatedAt": q.CapturedAt.Format(time.RFC3339),
			"ageSeconds":    int64(age.Seconds()),
			"isStale":       age > 30*time.Minute,
		}
		if q.ResetsAt != nil {
			timeUntilReset := time.Until(*q.ResetsAt)
			qMap["resetsAt"] = q.ResetsAt.Format(time.RFC3339)
			qMap["timeUntilReset"] = formatDuration(timeUntilReset)
			qMap["timeUntilResetSeconds"] = int64(timeUntilReset.Seconds())
		}
		if h.arkTracker != nil {
			if summary, sErr := h.arkTracker.UsageSummary(q.Name); sErr == nil && summary != nil {
				qMap["currentRate"] = summary.CurrentRate
				qMap["projectedUtil"] = summary.ProjectedUtil
			}
		}
		quotas = append(quotas, qMap)
	}
	response["quotas"] = quotas
	applyDisplayModeToResponse(response, h.getDisplayMode("ark"))
	return response
}

func (h *Handler) historyArk(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.store == nil {
		respondJSON(w, http.StatusOK, []interface{}{})
		return
	}

	rangeParam := r.URL.Query().Get("range")
	if rangeParam == "" {
		rangeParam = "7d"
	}

	now := time.Now().UTC()
	var start time.Time
	switch rangeParam {
	case "1h":
		start = now.Add(-1 * time.Hour)
	case "6h":
		start = now.Add(-6 * time.Hour)
	case "24h", "1d":
		start = now.Add(-24 * time.Hour)
	case "3d":
		start = now.Add(-3 * 24 * time.Hour)
	case "30d":
		start = now.Add(-30 * 24 * time.Hour)
	case "7d":
		start = now.Add(-7 * 24 * time.Hour)
	default:
		start = now.Add(-7 * 24 * time.Hour)
	}

	snapshots, err := h.store.QueryArkRange(start, now, 200)
	if err != nil {
		h.logger.Error("failed to query Ark history", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to query history")
		return
	}

	type historyEntry struct {
		CapturedAt string                   `json:"capturedAt"`
		Windows    []map[string]interface{} `json:"windows"`
	}

	result := make([]historyEntry, 0, len(snapshots))
	for _, snap := range snapshots {
		entry := historyEntry{
			CapturedAt: snap.CapturedAt.Format(time.RFC3339),
		}
		for _, w := range snap.Windows {
			wMap := map[string]interface{}{
				"name":        w.Name,
				"utilization": w.Percent,
				"used":        w.Used,
				"limit":       w.Quota,
				"format":      "percent",
			}
			if w.ResetsAt != nil {
				wMap["resetsAt"] = w.ResetsAt.Format(time.RFC3339)
			}
			entry.Windows = append(entry.Windows, wMap)
		}
		result = append(result, entry)
	}

	respondJSON(w, http.StatusOK, result)
}

func (h *Handler) cyclesArk(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.store == nil {
		respondJSON(w, http.StatusOK, []interface{}{})
		return
	}

	quotaName := r.URL.Query().Get("type")
	if quotaName == "" {
		quotaName = "five_hour"
	}

	active, err := h.store.QueryActiveArkCycle(quotaName)
	if err != nil {
		h.logger.Error("failed to query active Ark cycle", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to query cycles")
		return
	}

	history, err := h.store.QueryArkCycleHistory(quotaName, 50)
	if err != nil {
		h.logger.Error("failed to query Ark cycle history", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to query cycles")
		return
	}

	var cycles []map[string]interface{}
	if active != nil {
		cycleMap := map[string]interface{}{
			"id":              active.ID,
			"quotaName":       active.QuotaName,
			"cycleStart":      active.CycleStart.Format(time.RFC3339),
			"cycleEnd":        nil,
			"peakUtilization": active.PeakUtilization,
			"totalDelta":      active.TotalDelta,
			"isActive":        true,
		}
		if active.ResetsAt != nil {
			cycleMap["resetsAt"] = active.ResetsAt.Format(time.RFC3339)
			cycleMap["timeUntilReset"] = formatDuration(time.Until(*active.ResetsAt))
		}
		cycles = append(cycles, cycleMap)
	}

	for _, c := range history {
		cycleMap := map[string]interface{}{
			"id":              c.ID,
			"quotaName":       c.QuotaName,
			"cycleStart":      c.CycleStart.Format(time.RFC3339),
			"cycleEnd":        c.CycleEnd.Format(time.RFC3339),
			"peakUtilization": c.PeakUtilization,
			"totalDelta":      c.TotalDelta,
			"isActive":        false,
		}
		if c.ResetsAt != nil {
			cycleMap["resetsAt"] = c.ResetsAt.Format(time.RFC3339)
		}
		cycles = append(cycles, cycleMap)
	}

	respondJSON(w, http.StatusOK, cycles)
}

func (h *Handler) cycleOverviewArk(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.store == nil {
		respondJSON(w, http.StatusOK, []interface{}{})
		return
	}

	groupBy := r.URL.Query().Get("group_by")
	if groupBy == "" {
		groupBy = "five_hour"
	}

	overview, err := h.store.QueryArkCycleOverview(groupBy, 50)
	if err != nil {
		h.logger.Error("failed to query Ark cycle overview", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to query cycle overview")
		return
	}

	respondJSON(w, http.StatusOK, overview)
}

func (h *Handler) summaryArk(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	respondJSON(w, http.StatusOK, h.buildArkSummaryMap())
}

func (h *Handler) insightsArk(w http.ResponseWriter, _ *http.Request, rangeDur time.Duration) {
	hidden := h.getHiddenInsightKeys()
	respondJSON(w, http.StatusOK, h.buildArkInsights(hidden, rangeDur))
}

func (h *Handler) arkQuotaNames() []string {
	if h.store == nil {
		return nil
	}
	names, err := h.store.QueryAllArkQuotaNames()
	if err != nil {
		return nil
	}
	return names
}

func (h *Handler) buildArkSummaryMap() map[string]interface{} {
	if h.store == nil || h.arkTracker == nil {
		return map[string]interface{}{}
	}

	quotaNames, err := h.store.QueryAllArkQuotaNames()
	if err != nil {
		h.logger.Error("failed to query Ark quota names", "error", err)
		return map[string]interface{}{}
	}

	result := make(map[string]interface{})
	for _, name := range quotaNames {
		summary, err := h.arkTracker.UsageSummary(name)
		if err != nil || summary == nil {
			continue
		}
		entry := map[string]interface{}{
			"currentUtil":     summary.CurrentUtil,
			"completedCycles": summary.CompletedCycles,
			"peakCycle":       summary.PeakCycle,
			"avgPerCycle":     summary.AvgPerCycle,
			"totalTracked":    summary.TotalTracked,
		}
		if summary.ResetsAt != nil {
			entry["resetsAt"] = summary.ResetsAt.Format(time.RFC3339)
			entry["timeUntilReset"] = formatDuration(summary.TimeUntilReset)
		}
		result[name] = entry
	}
	return result
}

func (h *Handler) buildArkInsights(hidden map[string]bool, _ time.Duration) arkInsightsResponse {
	resp := arkInsightsResponse{Stats: []arkInsightStat{}, Insights: []insightItem{}}

	if h.store == nil {
		return resp
	}

	latest, err := h.store.QueryLatestArk()
	if err != nil || latest == nil || len(latest.Windows) == 0 {
		return resp
	}

	if latest.PlanType != "" {
		resp.Stats = append(resp.Stats, arkInsightStat{
			Label: "Plan",
			Value: latest.PlanType,
		})
	}

	windows := append([]api.ArkWindowSnapshot(nil), latest.Windows...)
	sort.SliceStable(windows, func(i, j int) bool {
		left := arkQuotaOrder(windows[i].Name)
		right := arkQuotaOrder(windows[j].Name)
		if left != right {
			return left < right
		}
		return windows[i].Name < windows[j].Name
	})

	summaries := map[string]*tracker.ArkSummary{}
	if h.arkTracker != nil {
		for _, w := range windows {
			summary, err := h.arkTracker.UsageSummary(w.Name)
			if err == nil && summary != nil {
				summaries[w.Name] = summary
			}
		}
	}

	preferredQuotas := []string{"five_hour", "daily", "weekly", "monthly", "cp_session", "cp_weekly", "cp_monthly"}
	selected := make([]api.ArkWindowSnapshot, 0, len(preferredQuotas))
	for _, name := range preferredQuotas {
		for _, w := range windows {
			if w.Name == name {
				selected = append(selected, w)
				break
			}
		}
	}
	if len(selected) == 0 {
		selected = windows
	}

	for _, w := range selected {
		rate := h.computeArkRate(w.Name, w.Percent, summaries[w.Name])
		insightKey := fmt.Sprintf("forecast_%s", w.Name)
		if hidden[insightKey] {
			continue
		}
		value := "Analyzing..."
		if rate.HasRate {
			value = fmt.Sprintf("%.1f%%/hr", rate.Rate)
		}
		insight := buildArkBurnRateInsight(w, rate)
		resp.Stats = append(resp.Stats, arkInsightStat{
			Key:      insightKey,
			Label:    fmt.Sprintf("%s Burn Rate", arkDisplayName(w.Name)),
			Value:    value,
			Sublabel: insight.Sublabel,
			Metric:   insight.Metric,
			Severity: insight.Severity,
			Desc:     insight.Desc,
		})
	}

	return resp
}

func (h *Handler) loggingHistoryArk(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{"logs": []interface{}{}})
		return
	}

	start, end, limit := h.loggingHistoryRangeAndLimit(r)
	snapshots, err := h.store.QueryArkRange(start, end, limit)
	if err != nil {
		h.logger.Error("failed to query Ark snapshots", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to query logging history")
		return
	}

	windowSet := map[string]bool{}
	for _, snap := range snapshots {
		for _, w := range snap.Windows {
			windowSet[w.Name] = true
		}
	}

	quotaNames := make([]string, 0, len(windowSet))
	for qn := range windowSet {
		quotaNames = append(quotaNames, qn)
	}
	if len(quotaNames) == 0 {
		quotaNames = []string{"five_hour", "daily", "weekly", "monthly"}
	} else {
		sort.SliceStable(quotaNames, func(i, j int) bool {
			left := arkQuotaOrder(quotaNames[i])
			right := arkQuotaOrder(quotaNames[j])
			if left != right {
				return left < right
			}
			return quotaNames[i] < quotaNames[j]
		})
	}

	capturedAt := make([]time.Time, 0, len(snapshots))
	ids := make([]int64, 0, len(snapshots))
	series := make([]map[string]loggingHistoryCrossQuota, 0, len(snapshots))

	for _, snap := range snapshots {
		capturedAt = append(capturedAt, snap.CapturedAt)
		ids = append(ids, snap.ID)
		row := make(map[string]loggingHistoryCrossQuota, len(snap.Windows))
		for _, w := range snap.Windows {
			row[w.Name] = loggingHistoryCrossQuota{
				Name:     w.Name,
				Value:    w.Used,
				Limit:    w.Quota,
				Percent:  w.Percent,
				HasValue: w.Used > 0 || w.Quota > 0,
				HasLimit: w.Quota > 0,
			}
		}
		series = append(series, row)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"provider":   "ark",
		"quotaNames": quotaNames,
		"logs":       loggingHistoryRowsFromSnapshots(capturedAt, ids, quotaNames, series),
	})
}