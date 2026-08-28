// Package agent provides the background polling agent for onWatch.
package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/notify"
	"github.com/onllm-dev/onwatch/v2/internal/store"
	"github.com/onllm-dev/onwatch/v2/internal/tracker"
)

// ArkAgent manages the background polling loop for Volcano Engine Ark
// (GetAFPUsage) quota tracking.
type ArkAgent struct {
	client       *api.ArkClient
	store        *store.Store
	tracker      *tracker.ArkTracker
	interval     time.Duration
	logger       *slog.Logger
	sm           *SessionManager
	notifier     *notify.NotificationEngine
	pollingCheck func() bool
}

// SetPollingCheck sets a function that is called before each poll.
// If it returns false, the poll is skipped (provider polling disabled).
func (a *ArkAgent) SetPollingCheck(fn func() bool) {
	a.pollingCheck = fn
}

// SetNotifier sets the notification engine for sending alerts.
func (a *ArkAgent) SetNotifier(n *notify.NotificationEngine) {
	a.notifier = n
}

// NewArkAgent creates a new ArkAgent with the given dependencies.
func NewArkAgent(client *api.ArkClient, store *store.Store, tr *tracker.ArkTracker, interval time.Duration, logger *slog.Logger, sm *SessionManager) *ArkAgent {
	if logger == nil {
		logger = slog.Default()
	}
	return &ArkAgent{
		client:   client,
		store:    store,
		tracker:  tr,
		interval: interval,
		logger:   logger,
		sm:       sm,
	}
}

// Run starts the Ark agent's polling loop. It polls immediately,
// then continues at the configured interval until the context is cancelled.
func (a *ArkAgent) Run(ctx context.Context) error {
	a.logger.Info("Ark agent started", "interval", a.interval)

	// Ensure any active session is closed on exit
	defer func() {
		if a.sm != nil {
			a.sm.Close()
		}
		a.logger.Info("Ark agent stopped")
	}()

	// Poll immediately on start
	a.poll(ctx)

	// Create ticker for periodic polling
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	// Main polling loop
	for {
		select {
		case <-ticker.C:
			a.poll(ctx)
		case <-ctx.Done():
			return nil
		}
	}
}

// poll performs a single Ark poll cycle: fetch usage, store snapshot,
// update tracker and fire notifications.
func (a *ArkAgent) poll(ctx context.Context) {
	if a.client == nil {
		return
	}
	if a.pollingCheck != nil && !a.pollingCheck() {
		return // polling disabled for this provider
	}

	snapshot, err := a.client.FetchUsage(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		a.logger.Error("Failed to fetch Ark usage", "error", err)
		return
	}

	if _, err := a.store.InsertArkSnapshot(snapshot); err != nil {
		a.logger.Error("Failed to insert Ark snapshot", "error", err)
		return
	}

	// Process with tracker (log error but don't stop)
	if a.tracker != nil {
		if err := a.tracker.Process(snapshot); err != nil {
			a.logger.Error("Ark tracker processing failed", "error", err)
		}
	}

	// Check notification thresholds per quota window
	if a.notifier != nil {
		for _, w := range snapshot.Windows {
			a.notifier.Check(notify.QuotaStatus{
				Provider:    "ark",
				QuotaKey:    w.Name,
				Utilization: w.Percent,
				Limit:       w.Quota,
			})
		}
	}

	// Report to session manager for usage-based session detection
	if a.sm != nil {
		var values []float64
		for _, w := range snapshot.Windows {
			values = append(values, w.Used)
		}
		a.sm.ReportPoll(values)
	}

	// Log poll completion with per-window utilization.
	a.logger.Info("Ark poll complete",
		"plan", snapshot.PlanType,
		"windows", len(snapshot.Windows),
	)
	for _, w := range snapshot.Windows {
		a.logger.Debug("Ark window usage",
			"window", w.Name,
			"used", w.Used,
			"quota", w.Quota,
			"utilization", w.Percent,
			"resets_at", w.ResetsAt,
		)
	}
}