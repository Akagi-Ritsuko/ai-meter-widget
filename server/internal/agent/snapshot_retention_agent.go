package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/store"
)

// snapshotRetentionPruneIntervalDefault 快照保留策略的裁剪周期（每小时一次，复用
// api_integrations 每小时 prune job 的节流模式）。
const snapshotRetentionPruneIntervalDefault = time.Hour

// SnapshotRetentionAgent 周期性删除超过保留期的 provider 快照（P3-07）。
// retention <= 0 表示禁用裁剪，Run 仅阻塞等待退出。
type SnapshotRetentionAgent struct {
	store         *store.Store
	retention     time.Duration
	pruneInterval time.Duration
	logger        *slog.Logger
}

// NewSnapshotRetentionAgent creates a new snapshot retention agent.
func NewSnapshotRetentionAgent(st *store.Store, retention time.Duration, logger *slog.Logger) *SnapshotRetentionAgent {
	if logger == nil {
		logger = slog.Default()
	}
	return &SnapshotRetentionAgent{
		store:         st,
		retention:     retention,
		pruneInterval: snapshotRetentionPruneIntervalDefault,
		logger:        logger,
	}
}

// SetPruneInterval overrides the prune interval. Used in tests.
func (a *SnapshotRetentionAgent) SetPruneInterval(d time.Duration) {
	if d > 0 {
		a.pruneInterval = d
	}
}

// Run starts the periodic prune loop until context cancellation.
func (a *SnapshotRetentionAgent) Run(ctx context.Context) error {
	if a.retention <= 0 {
		a.logger.Info("Snapshot retention pruning disabled", "retention", a.retention)
		<-ctx.Done()
		return nil
	}
	a.logger.Info("Snapshot retention agent started", "retention", a.retention, "interval", a.pruneInterval)
	defer a.logger.Info("Snapshot retention agent stopped")

	a.prune()

	ticker := time.NewTicker(a.pruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.prune()
		case <-ctx.Done():
			return nil
		}
	}
}

func (a *SnapshotRetentionAgent) prune() {
	cutoff := time.Now().UTC().Add(-a.retention)
	deleted, err := a.store.PruneSnapshotsOlderThan(cutoff)
	if err != nil {
		a.logger.Error("Snapshot retention prune failed", "error", err)
		return
	}
	total := int64(0)
	for _, n := range deleted {
		total += n
	}
	if total > 0 {
		a.logger.Info("Snapshot retention pruned snapshots",
			"deleted", total, "tables", len(deleted), "cutoff", cutoff.Format(time.RFC3339))
	}
}
