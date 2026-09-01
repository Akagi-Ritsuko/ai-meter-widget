package store

import (
	"testing"
	"time"
)

// snapshotInsertFixtures 各快照表的最小插入 SQL（唯一参数为 captured_at；
// 其余 NOT NULL 无默认值列已按 schema 补零值）。
// 新增 *_snapshots 表时必须同步补充 snapshotPruneTables 与本清单（漏一即测试失败）。
var snapshotInsertFixtures = map[string]string{
	"quota_snapshots":       `INSERT INTO quota_snapshots (captured_at, sub_limit, sub_requests, sub_renews_at, search_limit, search_requests, search_renews_at, tool_limit, tool_requests, tool_renews_at) VALUES (?, 0, 0, '', 0, 0, '', 0, 0, '')`,
	"zai_snapshots":         `INSERT INTO zai_snapshots (captured_at, time_limit, time_unit, time_number, time_usage, time_current_value, time_remaining, time_percentage, tokens_limit, tokens_unit, tokens_number, tokens_usage, tokens_current_value, tokens_remaining, tokens_percentage) VALUES (?, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)`,
	"anthropic_snapshots":   `INSERT INTO anthropic_snapshots (captured_at) VALUES (?)`,
	"copilot_snapshots":     `INSERT INTO copilot_snapshots (captured_at) VALUES (?)`,
	"codex_snapshots":       `INSERT INTO codex_snapshots (captured_at) VALUES (?)`,
	"antigravity_snapshots": `INSERT INTO antigravity_snapshots (captured_at) VALUES (?)`,
	"moonshot_snapshots":    `INSERT INTO moonshot_snapshots (captured_at) VALUES (?)`,
	"deepseek_snapshots":    `INSERT INTO deepseek_snapshots (captured_at) VALUES (?)`,
	"minimax_snapshots":     `INSERT INTO minimax_snapshots (captured_at) VALUES (?)`,
	"gemini_snapshots":      `INSERT INTO gemini_snapshots (captured_at) VALUES (?)`,
	"openrouter_snapshots":  `INSERT INTO openrouter_snapshots (captured_at) VALUES (?)`,
	"cursor_snapshots":      `INSERT INTO cursor_snapshots (captured_at) VALUES (?)`,
	"grok_snapshots":        `INSERT INTO grok_snapshots (captured_at) VALUES (?)`,
	"kimi_snapshots":        `INSERT INTO kimi_snapshots (captured_at) VALUES (?)`,
	"opencode_snapshots":    `INSERT INTO opencode_snapshots (captured_at) VALUES (?)`,
	"ark_snapshots":         `INSERT INTO ark_snapshots (captured_at) VALUES (?)`,
}

func TestSnapshotPruneListCoversAllTables(t *testing.T) {
	// 清单化校验：prune 清单与插入夹具必须一一对应，防止新增快照表后漏裁剪
	if len(snapshotPruneTables) != len(snapshotInsertFixtures) {
		t.Fatalf("prune 清单 %d 张表, 夹具 %d 张表, 数量不一致", len(snapshotPruneTables), len(snapshotInsertFixtures))
	}
	seen := make(map[string]bool, len(snapshotPruneTables))
	for _, table := range snapshotPruneTables {
		seen[table] = true
		if _, ok := snapshotInsertFixtures[table]; !ok {
			t.Errorf("表 %q 在 prune 清单中但缺少插入夹具", table)
		}
	}
	for table := range snapshotInsertFixtures {
		if !seen[table] {
			t.Errorf("表 %q 有插入夹具但不在 prune 清单中", table)
		}
	}
}

func insertSnapshotRow(t *testing.T, s *Store, table string, capturedAt time.Time) int64 {
	t.Helper()
	res, err := s.db.Exec(snapshotInsertFixtures[table], capturedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert %s: %v", table, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("insert %s LastInsertId: %v", table, err)
	}
	return id
}

func countRows(t *testing.T, s *Store, query string, args ...interface{}) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return n
}

func TestPruneSnapshotsOlderThan(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	old := now.Add(-200 * 24 * time.Hour)
	recent := now.Add(-24 * time.Hour)

	// 全部 16 张快照表：各注入 1 行过期 + 1 行新数据（清单化覆盖）
	// ark_snapshots 在下方单独注入（需 snapshot_id 做子表联动），此处跳过
	for _, table := range snapshotPruneTables {
		if table == "ark_snapshots" {
			continue
		}
		insertSnapshotRow(t, s, table, old)
		insertSnapshotRow(t, s, table, recent)
	}

	// ark 子表联动：过期快照 2 条子记录，新快照 1 条子记录
	oldArkID := insertSnapshotRow(t, s, "ark_snapshots", old)
	recentArkID := insertSnapshotRow(t, s, "ark_snapshots", recent)
	for i := 0; i < 2; i++ {
		if _, err := s.db.Exec(`INSERT INTO ark_quota_values (snapshot_id, quota_name) VALUES (?, 'q')`, oldArkID); err != nil {
			t.Fatalf("insert ark_quota_values: %v", err)
		}
	}
	if _, err := s.db.Exec(`INSERT INTO ark_quota_values (snapshot_id, quota_name) VALUES (?, 'q')`, recentArkID); err != nil {
		t.Fatalf("insert ark_quota_values: %v", err)
	}

	cutoff := now.Add(-100 * 24 * time.Hour)
	deleted, err := s.PruneSnapshotsOlderThan(cutoff)
	if err != nil {
		t.Fatalf("PruneSnapshotsOlderThan: %v", err)
	}

	// 16 张快照表各删 1 行
	for _, table := range snapshotPruneTables {
		if deleted[table] != 1 {
			t.Errorf("deleted[%s] = %d, 期望 1", table, deleted[table])
		}
	}
	// ark 子表联动删除 2 行
	if deleted["ark_quota_values"] != 2 {
		t.Errorf("deleted[ark_quota_values] = %d, 期望 2", deleted["ark_quota_values"])
	}
	if len(deleted) != len(snapshotPruneTables)+1 {
		t.Errorf("deleted 明细数量 = %d, 期望 %d", len(deleted), len(snapshotPruneTables)+1)
	}

	// 验证：过期行已删、新数据保留、子表仅剩新快照的 1 条
	for _, table := range snapshotPruneTables {
		if n := countRows(t, s, `SELECT COUNT(*) FROM `+table+` WHERE captured_at < ?`, cutoff.UTC().Format(time.RFC3339Nano)); n != 0 {
			t.Errorf("%s 仍有 %d 行早于 cutoff", table, n)
		}
		if n := countRows(t, s, `SELECT COUNT(*) FROM `+table); n != 1 {
			t.Errorf("%s 剩余 %d 行, 期望 1（新数据应保留）", table, n)
		}
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM ark_quota_values`); n != 1 {
		t.Errorf("ark_quota_values 剩余 %d 行, 期望 1", n)
	}

	// 再次执行：无删除，返回空明细
	deleted2, err := s.PruneSnapshotsOlderThan(cutoff)
	if err != nil {
		t.Fatalf("second PruneSnapshotsOlderThan: %v", err)
	}
	if len(deleted2) != 0 {
		t.Errorf("重复 prune 应无删除, got %v", deleted2)
	}
}

func TestPruneSnapshotsOlderThanBoundary(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	// captured_at == cutoff 的行不删除（严格小于语义）
	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if _, err := s.db.Exec(`INSERT INTO anthropic_snapshots (captured_at) VALUES (?)`, cutoff.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := s.PruneSnapshotsOlderThan(cutoff); err != nil {
		t.Fatalf("PruneSnapshotsOlderThan: %v", err)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM anthropic_snapshots`); n != 1 {
		t.Errorf("等于 cutoff 的行应保留, 实际剩余 %d", n)
	}
}
