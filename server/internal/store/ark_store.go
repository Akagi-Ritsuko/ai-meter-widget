package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
)

// ArkResetCycle tracks usage between two quota resets of a single Ark window.
type ArkResetCycle struct {
	ID              int64
	QuotaName       string
	CycleStart      time.Time
	CycleEnd        *time.Time
	ResetsAt        *time.Time
	PeakUtilization float64
	TotalDelta      float64
}

// ArkLatestQuota is the most recent quota value for one Ark window.
type ArkLatestQuota struct {
	Name        string
	Used        float64
	Limit       float64
	Utilization float64
	ResetsAt    *time.Time
	SubscribeAt *time.Time
	CapturedAt  time.Time
	PlanType    string
}

func (s *Store) InsertArkSnapshot(snapshot *api.ArkSnapshot) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`INSERT INTO ark_snapshots (captured_at, raw_json, plan_type) VALUES (?, ?, ?)`,
		snapshot.CapturedAt.Format(time.RFC3339Nano),
		snapshot.RawJSON,
		snapshot.PlanType,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert ark snapshot: %w", err)
	}

	snapshotID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get snapshot ID: %w", err)
	}

	for _, w := range snapshot.Windows {
		var resetsAt, subscribeAt interface{}
		if w.ResetsAt != nil {
			resetsAt = w.ResetsAt.Format(time.RFC3339Nano)
		}
		if w.SubscribeAt != nil {
			subscribeAt = w.SubscribeAt.Format(time.RFC3339Nano)
		}
		_, err := tx.Exec(
			`INSERT INTO ark_quota_values (snapshot_id, quota_name, quota, used, used_percent, resets_at, subscribe_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			snapshotID, w.Name, w.Quota, w.Used, w.Percent, resetsAt, subscribeAt,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to insert quota value %s: %w", w.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit: %w", err)
	}

	return snapshotID, nil
}

func (s *Store) QueryLatestArk() (*api.ArkSnapshot, error) {
	var snapshot api.ArkSnapshot
	var capturedAt, planType string

	err := s.db.QueryRow(
		`SELECT id, captured_at, plan_type FROM ark_snapshots ORDER BY captured_at DESC LIMIT 1`,
	).Scan(&snapshot.ID, &capturedAt, &planType)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query latest ark: %w", err)
	}

	snapshot.CapturedAt, _ = time.Parse(time.RFC3339Nano, capturedAt)
	snapshot.PlanType = planType

	rows, err := s.db.Query(
		`SELECT quota_name, quota, used, used_percent, resets_at, subscribe_at FROM ark_quota_values WHERE snapshot_id = ? ORDER BY id`,
		snapshot.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query quota values: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var w api.ArkWindowSnapshot
		var resetsAt, subscribeAt sql.NullString
		if err := rows.Scan(&w.Name, &w.Quota, &w.Used, &w.Percent, &resetsAt, &subscribeAt); err != nil {
			return nil, fmt.Errorf("failed to scan quota value: %w", err)
		}
		if resetsAt.Valid && resetsAt.String != "" {
			t, _ := time.Parse(time.RFC3339Nano, resetsAt.String)
			w.ResetsAt = &t
		}
		if subscribeAt.Valid && subscribeAt.String != "" {
			t, _ := time.Parse(time.RFC3339Nano, subscribeAt.String)
			w.SubscribeAt = &t
		}
		snapshot.Windows = append(snapshot.Windows, w)
	}

	return &snapshot, rows.Err()
}

func (s *Store) QueryArkRange(start, end time.Time, limit ...int) ([]*api.ArkSnapshot, error) {
	query := `SELECT id, captured_at, plan_type FROM ark_snapshots
		WHERE captured_at BETWEEN ? AND ? ORDER BY captured_at ASC`
	args := []interface{}{start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano)}
	if len(limit) > 0 && limit[0] > 0 {
		query = `SELECT id, captured_at, plan_type
			FROM (
				SELECT id, captured_at, plan_type
				FROM ark_snapshots
				WHERE captured_at BETWEEN ? AND ?
				ORDER BY captured_at DESC
				LIMIT ?
			) recent
			ORDER BY captured_at ASC`
		args = append(args, limit[0])
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query ark range: %w", err)
	}
	defer rows.Close()

	var snapshots []*api.ArkSnapshot
	for rows.Next() {
		var snap api.ArkSnapshot
		var capturedAt, planType string
		if err := rows.Scan(&snap.ID, &capturedAt, &planType); err != nil {
			return nil, fmt.Errorf("failed to scan ark snapshot: %w", err)
		}
		snap.CapturedAt, _ = time.Parse(time.RFC3339Nano, capturedAt)
		snap.PlanType = planType
		snapshots = append(snapshots, &snap)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, snap := range snapshots {
		qRows, err := s.db.Query(
			`SELECT quota_name, quota, used, used_percent, resets_at, subscribe_at FROM ark_quota_values WHERE snapshot_id = ? ORDER BY id`,
			snap.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to query quota values for snapshot %d: %w", snap.ID, err)
		}
		for qRows.Next() {
			var w api.ArkWindowSnapshot
			var resetsAt, subscribeAt sql.NullString
			if err := qRows.Scan(&w.Name, &w.Quota, &w.Used, &w.Percent, &resetsAt, &subscribeAt); err != nil {
				qRows.Close()
				return nil, fmt.Errorf("failed to scan quota value: %w", err)
			}
			if resetsAt.Valid && resetsAt.String != "" {
				t, _ := time.Parse(time.RFC3339Nano, resetsAt.String)
				w.ResetsAt = &t
			}
			if subscribeAt.Valid && subscribeAt.String != "" {
				t, _ := time.Parse(time.RFC3339Nano, subscribeAt.String)
				w.SubscribeAt = &t
			}
			snap.Windows = append(snap.Windows, w)
		}
		qRows.Close()
	}

	return snapshots, nil
}

func (s *Store) CreateArkCycle(quotaName string, cycleStart time.Time, resetsAt *time.Time) (int64, error) {
	var resetsAtVal interface{}
	if resetsAt != nil {
		resetsAtVal = resetsAt.Format(time.RFC3339Nano)
	}

	result, err := s.db.Exec(
		`INSERT INTO ark_reset_cycles (quota_name, cycle_start, reset_at) VALUES (?, ?, ?)`,
		quotaName, cycleStart.Format(time.RFC3339Nano), resetsAtVal,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create ark cycle: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get cycle ID: %w", err)
	}
	return id, nil
}

func (s *Store) CloseArkCycle(quotaName string, cycleEnd time.Time, peak, delta float64) error {
	_, err := s.db.Exec(
		`UPDATE ark_reset_cycles SET cycle_end = ?, peak_utilization = ?, total_delta = ?
		WHERE quota_name = ? AND cycle_end IS NULL`,
		cycleEnd.Format(time.RFC3339Nano), peak, delta, quotaName,
	)
	if err != nil {
		return fmt.Errorf("failed to close ark cycle: %w", err)
	}
	return nil
}

func (s *Store) UpdateArkCycle(quotaName string, peak, delta float64) error {
	_, err := s.db.Exec(
		`UPDATE ark_reset_cycles SET peak_utilization = ?, total_delta = ?
		WHERE quota_name = ? AND cycle_end IS NULL`,
		peak, delta, quotaName,
	)
	if err != nil {
		return fmt.Errorf("failed to update ark cycle: %w", err)
	}
	return nil
}

func (s *Store) QueryActiveArkCycle(quotaName string) (*ArkResetCycle, error) {
	var cycle ArkResetCycle
	var cycleStart string
	var cycleEnd, resetsAt sql.NullString

	err := s.db.QueryRow(
		`SELECT id, quota_name, cycle_start, cycle_end, reset_at, peak_utilization, total_delta
		FROM ark_reset_cycles WHERE quota_name = ? AND cycle_end IS NULL`,
		quotaName,
	).Scan(
		&cycle.ID, &cycle.QuotaName, &cycleStart, &cycleEnd, &resetsAt,
		&cycle.PeakUtilization, &cycle.TotalDelta,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query active ark cycle: %w", err)
	}

	cycle.CycleStart, _ = time.Parse(time.RFC3339Nano, cycleStart)
	if cycleEnd.Valid {
		t, _ := time.Parse(time.RFC3339Nano, cycleEnd.String)
		cycle.CycleEnd = &t
	}
	if resetsAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, resetsAt.String)
		cycle.ResetsAt = &t
	}

	return &cycle, nil
}

func (s *Store) QueryArkCycleHistory(quotaName string, limit ...int) ([]*ArkResetCycle, error) {
	query := `SELECT id, quota_name, cycle_start, cycle_end, reset_at, peak_utilization, total_delta
		FROM ark_reset_cycles WHERE quota_name = ? AND cycle_end IS NOT NULL ORDER BY cycle_start DESC`
	args := []interface{}{quotaName}
	if len(limit) > 0 && limit[0] > 0 {
		query += ` LIMIT ?`
		args = append(args, limit[0])
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query ark cycles: %w", err)
	}
	defer rows.Close()

	var cycles []*ArkResetCycle
	for rows.Next() {
		var cycle ArkResetCycle
		var cycleStart, cycleEnd string
		var resetsAt sql.NullString

		if err := rows.Scan(&cycle.ID, &cycle.QuotaName, &cycleStart, &cycleEnd, &resetsAt,
			&cycle.PeakUtilization, &cycle.TotalDelta); err != nil {
			return nil, fmt.Errorf("failed to scan ark cycle: %w", err)
		}

		cycle.CycleStart, _ = time.Parse(time.RFC3339Nano, cycleStart)
		t, _ := time.Parse(time.RFC3339Nano, cycleEnd)
		cycle.CycleEnd = &t
		if resetsAt.Valid {
			rt, _ := time.Parse(time.RFC3339Nano, resetsAt.String)
			cycle.ResetsAt = &rt
		}

		cycles = append(cycles, &cycle)
	}

	return cycles, rows.Err()
}

func (s *Store) QueryArkCyclesSince(quotaName string, since time.Time) ([]*ArkResetCycle, error) {
	rows, err := s.db.Query(
		`SELECT id, quota_name, cycle_start, cycle_end, reset_at, peak_utilization, total_delta
		FROM ark_reset_cycles WHERE quota_name = ? AND cycle_end IS NOT NULL AND cycle_start >= ?
		ORDER BY cycle_start DESC`,
		quotaName, since.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query ark cycles since: %w", err)
	}
	defer rows.Close()

	var cycles []*ArkResetCycle
	for rows.Next() {
		var cycle ArkResetCycle
		var cycleStart, cycleEnd string
		var resetsAt sql.NullString

		if err := rows.Scan(&cycle.ID, &cycle.QuotaName, &cycleStart, &cycleEnd, &resetsAt,
			&cycle.PeakUtilization, &cycle.TotalDelta); err != nil {
			return nil, fmt.Errorf("failed to scan ark cycle: %w", err)
		}

		cycle.CycleStart, _ = time.Parse(time.RFC3339Nano, cycleStart)
		t, _ := time.Parse(time.RFC3339Nano, cycleEnd)
		cycle.CycleEnd = &t
		if resetsAt.Valid {
			rt, _ := time.Parse(time.RFC3339Nano, resetsAt.String)
			cycle.ResetsAt = &rt
		}

		cycles = append(cycles, &cycle)
	}

	return cycles, rows.Err()
}

func (s *Store) QueryArkUtilizationSeries(quotaName string, since time.Time) ([]UtilizationPoint, error) {
	rows, err := s.db.Query(
		`SELECT s.captured_at, qv.used_percent
		FROM ark_quota_values qv
		JOIN ark_snapshots s ON s.id = qv.snapshot_id
		WHERE qv.quota_name = ? AND s.captured_at >= ?
		ORDER BY s.captured_at ASC`,
		quotaName, since.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query utilization series: %w", err)
	}
	defer rows.Close()

	var points []UtilizationPoint
	for rows.Next() {
		var capturedAt string
		var util float64
		if err := rows.Scan(&capturedAt, &util); err != nil {
			return nil, fmt.Errorf("failed to scan utilization point: %w", err)
		}
		t, _ := time.Parse(time.RFC3339Nano, capturedAt)
		points = append(points, UtilizationPoint{CapturedAt: t, Utilization: util})
	}

	return points, rows.Err()
}

func (s *Store) QueryArkLatestPerQuota() ([]ArkLatestQuota, error) {
	rows, err := s.db.Query(`
		SELECT qv.quota_name, qv.quota, qv.used, qv.used_percent, qv.resets_at, qv.subscribe_at,
		       s.captured_at, s.plan_type
		FROM ark_quota_values qv
		JOIN ark_snapshots s ON s.id = qv.snapshot_id
		WHERE s.id = (SELECT MAX(id) FROM ark_snapshots)
		ORDER BY qv.quota_name ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query latest per-quota: %w", err)
	}
	defer rows.Close()

	var results []ArkLatestQuota
	for rows.Next() {
		var name string
		var limitValue, used, utilization float64
		var resetsAt, subscribeAt sql.NullString
		var capturedAt, planType string

		if err := rows.Scan(&name, &limitValue, &used, &utilization, &resetsAt, &subscribeAt, &capturedAt, &planType); err != nil {
			return nil, fmt.Errorf("failed to scan latest quota: %w", err)
		}

		q := ArkLatestQuota{
			Name:        name,
			Used:        used,
			Limit:       limitValue,
			Utilization: utilization,
			PlanType:    planType,
		}
		q.CapturedAt, _ = time.Parse(time.RFC3339Nano, capturedAt)
		if resetsAt.Valid && resetsAt.String != "" {
			t, _ := time.Parse(time.RFC3339Nano, resetsAt.String)
			q.ResetsAt = &t
		}
		if subscribeAt.Valid && subscribeAt.String != "" {
			t, _ := time.Parse(time.RFC3339Nano, subscribeAt.String)
			q.SubscribeAt = &t
		}
		results = append(results, q)
	}
	return results, rows.Err()
}

func (s *Store) QueryAllArkQuotaNames() ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT quota_name FROM ark_reset_cycles ORDER BY quota_name`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query ark quota names: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan quota name: %w", err)
		}
		names = append(names, name)
	}

	return names, rows.Err()
}

func (s *Store) QueryArkCycleOverview(groupBy string, limit int) ([]CycleOverviewRow, error) {
	if limit <= 0 {
		limit = 50
	}

	var cycles []*ArkResetCycle
	activeCycle, err := s.QueryActiveArkCycle(groupBy)
	if err != nil {
		return nil, fmt.Errorf("store.QueryArkCycleOverview: active: %w", err)
	}
	if activeCycle != nil {
		cycles = append(cycles, activeCycle)
		limit--
	}

	completedCycles, err := s.QueryArkCycleHistory(groupBy, limit)
	if err != nil {
		return nil, fmt.Errorf("store.QueryArkCycleOverview: %w", err)
	}
	cycles = append(cycles, completedCycles...)

	var overviewRows []CycleOverviewRow
	for _, c := range cycles {
		row := CycleOverviewRow{
			CycleID:    c.ID,
			QuotaType:  c.QuotaName,
			CycleStart: c.CycleStart,
			CycleEnd:   c.CycleEnd,
			PeakValue:  c.PeakUtilization,
			TotalDelta: c.TotalDelta,
		}

		var endBoundary time.Time
		if c.CycleEnd != nil {
			endBoundary = *c.CycleEnd
		} else {
			endBoundary = time.Now().Add(time.Minute)
		}

		var snapshotID int64
		var capturedAt string
		err := s.db.QueryRow(
			`SELECT s.id, s.captured_at FROM ark_snapshots s
			JOIN ark_quota_values qv ON qv.snapshot_id = s.id
			WHERE qv.quota_name = ? AND s.captured_at >= ? AND s.captured_at < ?
			ORDER BY qv.used_percent DESC LIMIT 1`,
			groupBy,
			c.CycleStart.Format(time.RFC3339Nano),
			endBoundary.Format(time.RFC3339Nano),
		).Scan(&snapshotID, &capturedAt)

		if err == sql.ErrNoRows {
			overviewRows = append(overviewRows, row)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("store.QueryArkCycleOverview: peak snapshot: %w", err)
		}

		row.PeakTime, _ = time.Parse(time.RFC3339Nano, capturedAt)

		qRows, err := s.db.Query(
			`SELECT quota_name, used_percent, used, quota FROM ark_quota_values WHERE snapshot_id = ? ORDER BY id`,
			snapshotID,
		)
		if err != nil {
			return nil, fmt.Errorf("store.QueryArkCycleOverview: quota values: %w", err)
		}
		for qRows.Next() {
			var entry CrossQuotaEntry
			if err := qRows.Scan(&entry.Name, &entry.Percent, &entry.Value, new(float64)); err != nil {
				qRows.Close()
				return nil, fmt.Errorf("store.QueryArkCycleOverview: scan quota: %w", err)
			}
			row.CrossQuotas = append(row.CrossQuotas, entry)
		}
		qRows.Close()

		overviewRows = append(overviewRows, row)
	}

	return overviewRows, nil
}