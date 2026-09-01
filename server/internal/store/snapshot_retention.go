package store

import (
	"context"
	"fmt"
	"time"
)

// snapshotPruneTables 参与保留策略裁剪的 provider 快照表清单（P3-07）。
// 各表 captured_at 均存储 RFC3339Nano UTC 字符串（TEXT/DATETIME 皆同），字符串比较语义正确。
// 不在裁剪范围：generic_metrics（仅保留最新快照）、*_reset_cycles（无过期语义）。
var snapshotPruneTables = []string{
	"quota_snapshots",
	"zai_snapshots",
	"anthropic_snapshots",
	"copilot_snapshots",
	"codex_snapshots",
	"antigravity_snapshots",
	"moonshot_snapshots",
	"deepseek_snapshots",
	"minimax_snapshots",
	"gemini_snapshots",
	"openrouter_snapshots",
	"cursor_snapshots",
	"grok_snapshots",
	"kimi_snapshots",
	"opencode_snapshots",
	"ark_snapshots",
}

// PruneSnapshotsOlderThan deletes all provider snapshots whose captured_at is
// before cutoff (UTC), across every *_snapshots table (P3-07 retention policy).
// ark_quota_values rows are removed together with their ark_snapshots parents.
// Returns per-table deleted row counts (only tables with deletions are present).
func (s *Store) PruneSnapshotsOlderThan(cutoff time.Time) (map[string]int64, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	deleted := make(map[string]int64, len(snapshotPruneTables)+1)
	param := cutoff.UTC().Format(time.RFC3339Nano)

	// ark_quota_values 无自身 captured_at，按 snapshot_id 联动删除（先删子表再删主表）
	res, err := tx.ExecContext(ctx,
		`DELETE FROM ark_quota_values WHERE snapshot_id IN (SELECT id FROM ark_snapshots WHERE captured_at < ?)`,
		param)
	if err != nil {
		return nil, fmt.Errorf("prune ark_quota_values: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		deleted["ark_quota_values"] = n
	}

	for _, table := range snapshotPruneTables {
		res, err := tx.ExecContext(ctx,
			fmt.Sprintf(`DELETE FROM %s WHERE captured_at < ?`, table), param)
		if err != nil {
			return nil, fmt.Errorf("prune %s: %w", table, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			deleted[table] = n
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return deleted, nil
}
