package agent

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

func newRetentionTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// insertOldAnthropic 插入一行 200 天前的 anthropic 快照，返回插入是否成功。
func insertOldAnthropic(t *testing.T, st *store.Store) {
	t.Helper()
	if _, err := st.InsertAnthropicSnapshot(&api.AnthropicSnapshot{
		CapturedAt: time.Now().UTC().Add(-200 * 24 * time.Hour),
		RawJSON:    "{}",
	}); err != nil {
		t.Fatalf("InsertAnthropicSnapshot: %v", err)
	}
}

func TestSnapshotRetentionAgent_PrunesExpired(t *testing.T) {
	st := newRetentionTestStore(t)
	insertOldAnthropic(t, st)

	ag := NewSnapshotRetentionAgent(st, 24*time.Hour, slog.Default())
	ag.SetPruneInterval(10 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		_ = ag.Run(ctx)
		close(done)
	}()

	// 轮询等待过期快照被删除（首跑立即执行 prune，正常应秒级完成）
	deadline := time.Now().Add(2 * time.Second)
	for {
		snap, err := st.QueryLatestAnthropic()
		if err != nil {
			t.Fatalf("QueryLatestAnthropic: %v", err)
		}
		if snap == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("过期快照未被 prune 删除")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run 未随 ctx 退出")
	}
}

func TestSnapshotRetentionAgent_Disabled(t *testing.T) {
	st := newRetentionTestStore(t)
	insertOldAnthropic(t, st)

	// retention=0：与现状一致，不做任何裁剪
	ag := NewSnapshotRetentionAgent(st, 0, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = ag.Run(ctx)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run 未随 ctx 退出")
	}

	snap, err := st.QueryLatestAnthropic()
	if err != nil {
		t.Fatalf("QueryLatestAnthropic: %v", err)
	}
	if snap == nil {
		t.Fatal("retention=0 时过期快照不应被删除")
	}
}
