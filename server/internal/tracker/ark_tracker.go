package tracker

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

const arkResetDriftTolerance = 90 * time.Minute

// ArkTracker tracks per-window quota consumption for Volcano Engine Ark
// (GetAFPUsage). It mirrors the OpenCode tracker: a snapshot holds multiple
// windows (five_hour/daily/weekly/monthly), each with its own reset cycle.
type ArkTracker struct {
	store      *store.Store
	logger     *slog.Logger
	lastValues map[string]float64
	lastResets map[string]string
	hasLast    bool
	onReset    func(quotaName string)
}

func (t *ArkTracker) SetOnReset(fn func(string)) {
	t.onReset = fn
}

type ArkSummary struct {
	QuotaName       string
	CurrentUtil     float64
	ResetsAt        *time.Time
	TimeUntilReset  time.Duration
	CurrentRate     float64
	ProjectedUtil   float64
	CompletedCycles int
	AvgPerCycle     float64
	PeakCycle       float64
	TotalTracked    float64
	TrackingSince   time.Time
}

func NewArkTracker(store *store.Store, logger *slog.Logger) *ArkTracker {
	if logger == nil {
		logger = slog.Default()
	}
	return &ArkTracker{
		store:      store,
		logger:     logger,
		lastValues: make(map[string]float64),
		lastResets: make(map[string]string),
	}
}

func (t *ArkTracker) Process(snapshot *api.ArkSnapshot) error {
	for _, w := range snapshot.Windows {
		if err := t.processWindow(w, snapshot.CapturedAt); err != nil {
			return fmt.Errorf("ark tracker: %s: %w", w.Name, err)
		}
	}

	t.hasLast = true
	return nil
}

func (t *ArkTracker) processWindow(w api.ArkWindowSnapshot, capturedAt time.Time) error {
	quotaName := w.Name
	currentUtil := w.Percent

	cycle, err := t.store.QueryActiveArkCycle(quotaName)
	if err != nil {
		return fmt.Errorf("failed to query active cycle: %w", err)
	}

	if cycle == nil {
		_, err := t.store.CreateArkCycle(quotaName, capturedAt, w.ResetsAt)
		if err != nil {
			return fmt.Errorf("failed to create cycle: %w", err)
		}
		if err := t.store.UpdateArkCycle(quotaName, currentUtil, 0); err != nil {
			return fmt.Errorf("failed to set initial peak: %w", err)
		}
		t.lastValues[quotaName] = currentUtil
		if w.ResetsAt != nil {
			t.lastResets[quotaName] = w.ResetsAt.Format(time.RFC3339Nano)
		}
		t.logger.Info("Created new Ark quota cycle",
			"quota", quotaName,
			"resetsAt", w.ResetsAt,
			"initialUtil", currentUtil,
		)
		return nil
	}

	resetDetected := false
	resetReason := ""
	storedResetPassed := cycle.ResetsAt != nil && capturedAt.After(cycle.ResetsAt.Add(2*time.Minute))
	currentResetIsFuture := w.ResetsAt != nil && w.ResetsAt.After(capturedAt)
	if storedResetPassed && !currentResetIsFuture {
		resetDetected = true
		resetReason = "time-based (stored ResetsAt passed)"
	}

	if !resetDetected {
		if w.ResetsAt != nil && cycle.ResetsAt != nil {
			diff := w.ResetsAt.Sub(*cycle.ResetsAt)
			if diff < 0 {
				diff = -diff
			}
			if diff > arkResetDriftTolerance {
				resetDetected = true
				resetReason = "api-based (ResetsAt changed)"
			}
		} else if w.ResetsAt != nil && cycle.ResetsAt == nil {
			resetDetected = true
			resetReason = "api-based (new ResetsAt appeared)"
		}
	}

	if resetDetected {
		cycleEndTime := capturedAt
		if cycle.ResetsAt != nil && capturedAt.After(*cycle.ResetsAt) {
			cycleEndTime = *cycle.ResetsAt
		}

		if t.hasLast {
			if lastUtil, ok := t.lastValues[quotaName]; ok {
				delta := currentUtil - lastUtil
				if delta > 0 {
					cycle.TotalDelta += delta
				}
				if currentUtil > cycle.PeakUtilization {
					cycle.PeakUtilization = currentUtil
				}
			}
		}

		if err := t.store.CloseArkCycle(quotaName, cycleEndTime, cycle.PeakUtilization, cycle.TotalDelta); err != nil {
			return fmt.Errorf("failed to close cycle: %w", err)
		}

		if _, err := t.store.CreateArkCycle(quotaName, capturedAt, w.ResetsAt); err != nil {
			return fmt.Errorf("failed to create new cycle: %w", err)
		}
		if err := t.store.UpdateArkCycle(quotaName, currentUtil, 0); err != nil {
			return fmt.Errorf("failed to set initial peak: %w", err)
		}

		t.lastValues[quotaName] = currentUtil
		if w.ResetsAt != nil {
			t.lastResets[quotaName] = w.ResetsAt.Format(time.RFC3339Nano)
		}
		t.logger.Info("Detected Ark quota reset",
			"quota", quotaName,
			"reason", resetReason,
			"oldResetsAt", cycle.ResetsAt,
			"newResetsAt", w.ResetsAt,
			"cycleEndTime", cycleEndTime,
			"totalDelta", cycle.TotalDelta,
		)
		if t.onReset != nil {
			t.onReset(quotaName)
		}
		return nil
	}

	if t.hasLast {
		if lastUtil, ok := t.lastValues[quotaName]; ok {
			delta := currentUtil - lastUtil
			if delta > 0 {
				cycle.TotalDelta += delta
			}
			if currentUtil > cycle.PeakUtilization {
				cycle.PeakUtilization = currentUtil
			}
			if err := t.store.UpdateArkCycle(quotaName, cycle.PeakUtilization, cycle.TotalDelta); err != nil {
				return fmt.Errorf("failed to update cycle: %w", err)
			}
		} else {
			if currentUtil > cycle.PeakUtilization {
				cycle.PeakUtilization = currentUtil
				if err := t.store.UpdateArkCycle(quotaName, cycle.PeakUtilization, cycle.TotalDelta); err != nil {
					return fmt.Errorf("failed to update cycle: %w", err)
				}
			}
		}
	} else {
		if currentUtil > cycle.PeakUtilization {
			cycle.PeakUtilization = currentUtil
			if err := t.store.UpdateArkCycle(quotaName, cycle.PeakUtilization, cycle.TotalDelta); err != nil {
				return fmt.Errorf("failed to update cycle: %w", err)
			}
		}
	}

	t.lastValues[quotaName] = currentUtil
	if w.ResetsAt != nil {
		t.lastResets[quotaName] = w.ResetsAt.Format(time.RFC3339Nano)
	}
	return nil
}

func (t *ArkTracker) UsageSummary(quotaName string) (*ArkSummary, error) {
	activeCycle, err := t.store.QueryActiveArkCycle(quotaName)
	if err != nil {
		return nil, fmt.Errorf("failed to query active cycle: %w", err)
	}

	history, err := t.store.QueryArkCycleHistory(quotaName)
	if err != nil {
		return nil, fmt.Errorf("failed to query cycle history: %w", err)
	}

	summary := &ArkSummary{
		QuotaName:       quotaName,
		CompletedCycles: len(history),
	}

	if len(history) > 0 {
		var totalDelta float64
		summary.TrackingSince = history[len(history)-1].CycleStart

		for _, cycle := range history {
			totalDelta += cycle.TotalDelta
			if cycle.PeakUtilization > summary.PeakCycle {
				summary.PeakCycle = cycle.PeakUtilization
			}
		}
		summary.AvgPerCycle = totalDelta / float64(len(history))
		summary.TotalTracked = totalDelta
	}

	if activeCycle != nil {
		summary.TotalTracked += activeCycle.TotalDelta
		if activeCycle.PeakUtilization > summary.PeakCycle {
			summary.PeakCycle = activeCycle.PeakUtilization
		}
		if activeCycle.ResetsAt != nil {
			summary.ResetsAt = activeCycle.ResetsAt
			summary.TimeUntilReset = time.Until(*activeCycle.ResetsAt)
		}

		latest, err := t.store.QueryLatestArk()
		if err != nil {
			return nil, fmt.Errorf("failed to query latest: %w", err)
		}

		if latest != nil {
			for _, w := range latest.Windows {
				if w.Name == quotaName {
					summary.CurrentUtil = w.Percent
					if summary.ResetsAt == nil && w.ResetsAt != nil {
						summary.ResetsAt = w.ResetsAt
						summary.TimeUntilReset = time.Until(*w.ResetsAt)
					}
					break
				}
			}

			elapsed := time.Since(activeCycle.CycleStart)
			if elapsed.Minutes() >= 30 && activeCycle.TotalDelta > 0 {
				summary.CurrentRate = activeCycle.TotalDelta / elapsed.Hours()
				if summary.ResetsAt != nil {
					hoursLeft := time.Until(*summary.ResetsAt).Hours()
					if hoursLeft > 0 {
						projected := summary.CurrentUtil + (summary.CurrentRate * hoursLeft)
						if projected > 100 {
							projected = 100
						}
						summary.ProjectedUtil = projected
					}
				}
			}
		}
	}

	return summary, nil
}