package store

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/onllm-dev/onwatch/v2/internal/generic"
)

// SaveGenericSnapshot 保存或更新通用适配器平台指标快照
func (s *Store) SaveGenericSnapshot(snapshot *generic.PlatformSnapshot) error {
	metricsJSON := "{}"
	if snapshot.Metrics != nil {
		data, err := json.Marshal(snapshot.Metrics)
		if err != nil {
			return fmt.Errorf("store: 序列化通用指标失败: %w", err)
		}
		metricsJSON = string(data)
	}

	_, err := s.db.Exec(`
		INSERT INTO generic_metrics (platform, display_name, status, error, metrics, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(platform) DO UPDATE SET
			display_name = excluded.display_name,
			status = excluded.status,
			error = excluded.error,
			metrics = excluded.metrics,
			updated_at = excluded.updated_at`,
		snapshot.Platform, snapshot.DisplayName, snapshot.Status, snapshot.Error, metricsJSON, snapshot.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: 保存通用指标失败: %w", err)
	}
	return nil
}

// GetGenericSnapshot 读取单个平台指标快照
func (s *Store) GetGenericSnapshot(platform string) (*generic.PlatformSnapshot, error) {
	var (
		displayName, status, errMsg, metricsJSON, updatedAt string
	)
	err := s.db.QueryRow(`
		SELECT display_name, status, error, metrics, updated_at
		FROM generic_metrics WHERE platform = ?`, platform).
		Scan(&displayName, &status, &errMsg, &metricsJSON, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: 读取通用指标失败: %w", err)
	}

	snapshot := &generic.PlatformSnapshot{
		Platform:    platform,
		DisplayName: displayName,
		Status:      status,
		Error:       errMsg,
		UpdatedAt:   updatedAt,
	}
	if metricsJSON != "" && metricsJSON != "{}" {
		var m generic.Metrics
		if err := json.Unmarshal([]byte(metricsJSON), &m); err == nil {
			snapshot.Metrics = &m
		}
	}
	return snapshot, nil
}

// GetAllGenericSnapshots 读取全部平台指标快照
func (s *Store) GetAllGenericSnapshots() ([]generic.PlatformSnapshot, error) {
	rows, err := s.db.Query(`
		SELECT platform, display_name, status, error, metrics, updated_at
		FROM generic_metrics ORDER BY platform`)
	if err != nil {
		return nil, fmt.Errorf("store: 查询通用指标失败: %w", err)
	}
	defer rows.Close()

	var result []generic.PlatformSnapshot
	for rows.Next() {
		var (
			platform, displayName, status, errMsg, metricsJSON, updatedAt string
		)
		if err := rows.Scan(&platform, &displayName, &status, &errMsg, &metricsJSON, &updatedAt); err != nil {
			return nil, fmt.Errorf("store: 扫描通用指标失败: %w", err)
		}
		snapshot := generic.PlatformSnapshot{
			Platform:    platform,
			DisplayName: displayName,
			Status:      status,
			Error:       errMsg,
			UpdatedAt:   updatedAt,
		}
		if metricsJSON != "" && metricsJSON != "{}" {
			var m generic.Metrics
			if err := json.Unmarshal([]byte(metricsJSON), &m); err == nil {
				snapshot.Metrics = &m
			}
		}
		result = append(result, snapshot)
	}
	return result, rows.Err()
}