package metrics

import (
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

func mustTime(t *testing.T, s string) *time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("解析测试时间失败: %v", err)
	}
	return &tm
}

func TestConvertArk(t *testing.T) {
	t.Run("全4窗口按固定顺序排序", func(t *testing.T) {
		// 故意乱序输入，验证输出顺序 five_hour→daily→weekly→monthly
		quotas := []store.ArkLatestQuota{
			{Name: "monthly", Used: 40, Limit: 1000, Utilization: 4},
			{Name: "five_hour", Used: 5, Limit: 100, Utilization: 5},
			{Name: "weekly", Used: 30, Limit: 500, Utilization: 6},
			{Name: "daily", Used: 10, Limit: 200, Utilization: 5},
		}
		m := ConvertArk(quotas)
		if m == nil {
			t.Fatal("ConvertArk 返回 nil")
		}
		want := []string{"five_hour", "daily", "weekly", "monthly"}
		if len(m.Quota) != len(want) {
			t.Fatalf("窗口数 = %d, 期望 %d", len(m.Quota), len(want))
		}
		for i, w := range want {
			if m.Quota[i].Window != w {
				t.Errorf("Quota[%d].Window = %q, 期望 %q", i, m.Quota[i].Window, w)
			}
		}
	})

	t.Run("部分窗口缺失且未知窗口排最后", func(t *testing.T) {
		quotas := []store.ArkLatestQuota{
			{Name: "unknown_win", Used: 1, Limit: 10, Utilization: 10},
			{Name: "daily", Used: 10, Limit: 200, Utilization: 5},
		}
		m := ConvertArk(quotas)
		if len(m.Quota) != 2 {
			t.Fatalf("窗口数 = %d, 期望 2", len(m.Quota))
		}
		if m.Quota[0].Window != "daily" || m.Quota[1].Window != "unknown_win" {
			t.Errorf("顺序 = [%s, %s], 期望 [daily, unknown_win]", m.Quota[0].Window, m.Quota[1].Window)
		}
	})

	t.Run("字段映射与nil重置时间", func(t *testing.T) {
		quotas := []store.ArkLatestQuota{{
			Name:   "five_hour",
			Used:   25,
			Limit:  100,
			ResetsAt: mustTime(t, "2026-09-01T12:00:00Z"),
		}}
		m := ConvertArk(quotas)
		q := m.Quota[0]
		if q.Used != 25 || q.Total != 100 {
			t.Errorf("Used/Total = %v/%v, 期望 25/100", q.Used, q.Total)
		}
		// Utilization 未填时兜底计算 25/100*100
		if q.Percent != 25 {
			t.Errorf("Percent = %v, 期望 25（兜底）", q.Percent)
		}
		if q.ResetAt != "2026-09-01T12:00:00Z" {
			t.Errorf("ResetAt = %q, 期望 RFC3339 UTC 透传", q.ResetAt)
		}
		if q.Unit != "" {
			t.Errorf("Unit = %q, 期望空（官方单位未知不猜测）", q.Unit)
		}
	})

	t.Run("nil重置时间输出空串", func(t *testing.T) {
		m := ConvertArk([]store.ArkLatestQuota{{Name: "daily", Used: 1, Limit: 10, Utilization: 10}})
		if m.Quota[0].ResetAt != "" {
			t.Errorf("ResetAt = %q, 期望空串", m.Quota[0].ResetAt)
		}
	})

	t.Run("空输入返回空Quota切片", func(t *testing.T) {
		m := ConvertArk(nil)
		if m == nil {
			t.Fatal("ConvertArk(nil) 返回 nil")
		}
		if m.Quota == nil || len(m.Quota) != 0 {
			t.Errorf("Quota 应为空 slice（非 nil）, got %#v", m.Quota)
		}
		if m.Balance != nil || m.Cost != nil || m.Tokens != nil {
			t.Errorf("Balance/Cost/Tokens 应为 nil, got %v/%v/%v", m.Balance, m.Cost, m.Tokens)
		}
	})
}

func TestConvertZai(t *testing.T) {
	t.Run("双窗口完整映射", func(t *testing.T) {
		snap := &api.ZaiSnapshot{
			TimeLimit:      6000,
			TimeUsage:      1500,
			TimePercentage: 25,
			TokensLimit:         70000,
			TokensUsage:         21000,
			TokensPercentage:    30,
			TokensNextResetTime: mustTime(t, "2026-09-07T16:00:00Z"),
		}
		m := ConvertZai(snap)
		if len(m.Quota) != 2 {
			t.Fatalf("窗口数 = %d, 期望 2", len(m.Quota))
		}
		fiveHour := m.Quota[0]
		if fiveHour.Window != "5h" {
			t.Errorf("Quota[0].Window = %q, 期望 \"5h\"", fiveHour.Window)
		}
		if fiveHour.Used != 1500 || fiveHour.Total != 6000 || fiveHour.Percent != 25 {
			t.Errorf("5h 窗口 Used/Total/Percent = %v/%v/%v, 期望 1500/6000/25",
				fiveHour.Used, fiveHour.Total, fiveHour.Percent)
		}
		if fiveHour.ResetAt != "" {
			t.Errorf("5h ResetAt = %q, 期望空（ADR-015 动态刷新）", fiveHour.ResetAt)
		}
		weekly := m.Quota[1]
		if weekly.Window != "weekly" {
			t.Errorf("Quota[1].Window = %q, 期望 \"weekly\"", weekly.Window)
		}
		if weekly.Used != 21000 || weekly.Total != 70000 || weekly.Percent != 30 {
			t.Errorf("weekly 窗口 Used/Total/Percent = %v/%v/%v, 期望 21000/70000/30",
				weekly.Used, weekly.Total, weekly.Percent)
		}
		if weekly.ResetAt != "2026-09-07T16:00:00Z" {
			t.Errorf("weekly ResetAt = %q, 期望 TokensNextResetTime 透传", weekly.ResetAt)
		}
		if m.Balance != nil || m.Cost != nil || m.Tokens != nil {
			t.Errorf("Balance/Cost/Tokens 应为 nil, got %v/%v/%v", m.Balance, m.Cost, m.Tokens)
		}
	})

	t.Run("国内版仅有5h窗口", func(t *testing.T) {
		snap := &api.ZaiSnapshot{TimeLimit: 6000, TimeUsage: 0, TimePercentage: 0}
		m := ConvertZai(snap)
		if len(m.Quota) != 1 {
			t.Fatalf("窗口数 = %d, 期望 1", len(m.Quota))
		}
		if m.Quota[0].Window != "5h" {
			t.Errorf("Window = %q, 期望 \"5h\"", m.Quota[0].Window)
		}
	})

	t.Run("weekly重置时间为nil时输出空串", func(t *testing.T) {
		snap := &api.ZaiSnapshot{
			TimeLimit:   6000,
			TokensLimit: 70000,
			TokensUsage: 100,
		}
		m := ConvertZai(snap)
		weekly := m.Quota[1]
		if weekly.ResetAt != "" {
			t.Errorf("weekly ResetAt = %q, 期望空串", weekly.ResetAt)
		}
		// Percent=0 但 Total>0 → 兜底 100/70000*100
		if weekly.Percent == 0 {
			t.Errorf("weekly Percent = 0, 期望兜底计算结果")
		}
	})

	t.Run("空快照返回空Quota切片", func(t *testing.T) {
		m := ConvertZai(&api.ZaiSnapshot{})
		if m == nil || m.Quota == nil || len(m.Quota) != 0 {
			t.Errorf("空快照应返回空 slice, got %#v", m)
		}
	})
}

func TestConvertDeepSeek(t *testing.T) {
	t.Run("余额与币种透传", func(t *testing.T) {
		snap := &api.DeepSeekSnapshot{Currency: "CNY", TotalBalance: 110.55}
		m := ConvertDeepSeek(snap)
		if m.Balance == nil {
			t.Fatal("Balance 为 nil")
		}
		if m.Balance.Amount != 110.55 || m.Balance.Currency != "CNY" {
			t.Errorf("Balance = %v %s, 期望 110.55 CNY", m.Balance.Amount, m.Balance.Currency)
		}
		if m.Quota == nil || len(m.Quota) != 0 {
			t.Errorf("Quota 应为空 slice, got %#v", m.Quota)
		}
		if m.Cost != nil || m.Tokens != nil {
			t.Errorf("Cost/Tokens 应为 nil, got %v/%v", m.Cost, m.Tokens)
		}
	})

	t.Run("空快照不输出假余额", func(t *testing.T) {
		m := ConvertDeepSeek(&api.DeepSeekSnapshot{})
		if m.Balance == nil {
			t.Fatal("有快照时应输出 Balance（零值余额也是真实数据）")
		}
		if m.Balance.Amount != 0 || m.Balance.Currency != "" {
			t.Errorf("Balance = %v %q, 期望零值", m.Balance.Amount, m.Balance.Currency)
		}
	})
}

func TestConvertOpenCode(t *testing.T) {
	t.Run("字段映射与Format透传", func(t *testing.T) {
		quotas := []store.OpenCodeLatestQuota{
			{
				Name:   "five_hour",
				Used:   3.5,
				Limit:  10,
				Format: "hours",
				ResetsAt: mustTime(t, "2026-09-01T18:00:00Z"),
			},
			{Name: "monthly", Used: 20, Limit: 100, Utilization: 20, Format: "requests"},
		}
		m := ConvertOpenCode(quotas)
		if len(m.Quota) != 2 {
			t.Fatalf("窗口数 = %d, 期望 2", len(m.Quota))
		}
		q := m.Quota[0]
		if q.Window != "five_hour" || q.Used != 3.5 || q.Total != 10 {
			t.Errorf("字段映射错误: %+v", q)
		}
		// Utilization 未填时兜底 3.5/10*100
		if q.Percent != 35 {
			t.Errorf("Percent = %v, 期望 35（兜底）", q.Percent)
		}
		if q.Unit != "hours" {
			t.Errorf("Unit = %q, 期望 \"hours\"（Format 透传）", q.Unit)
		}
		if q.ResetAt != "2026-09-01T18:00:00Z" {
			t.Errorf("ResetAt = %q, 期望 RFC3339 透传", q.ResetAt)
		}
		if m.Quota[1].Unit != "requests" || m.Quota[1].Percent != 20 {
			t.Errorf("第二个窗口字段错误: %+v", m.Quota[1])
		}
		if m.Balance != nil || m.Cost != nil || m.Tokens != nil {
			t.Errorf("Balance/Cost/Tokens 应为 nil, got %v/%v/%v", m.Balance, m.Cost, m.Tokens)
		}
	})

	t.Run("保持store返回顺序", func(t *testing.T) {
		quotas := []store.OpenCodeLatestQuota{
			{Name: "weekly", Limit: 1},
			{Name: "five_hour", Limit: 1}, // 故意与 ark 顺序相反，OpenCode 不排序
			{Name: "daily", Limit: 1},
		}
		m := ConvertOpenCode(quotas)
		want := []string{"weekly", "five_hour", "daily"}
		for i, w := range want {
			if m.Quota[i].Window != w {
				t.Errorf("Quota[%d].Window = %q, 期望 %q（保持原顺序）", i, m.Quota[i].Window, w)
			}
		}
	})

	t.Run("空Format不填Unit", func(t *testing.T) {
		m := ConvertOpenCode([]store.OpenCodeLatestQuota{{Name: "daily", Limit: 10, Used: 1}})
		if m.Quota[0].Unit != "" {
			t.Errorf("Unit = %q, 期望空", m.Quota[0].Unit)
		}
	})

	t.Run("空输入返回空Quota切片", func(t *testing.T) {
		m := ConvertOpenCode(nil)
		if m == nil || m.Quota == nil || len(m.Quota) != 0 {
			t.Errorf("空输入应返回空 slice, got %#v", m)
		}
	})
}

func TestPercentOnlyQuota(t *testing.T) {
	q := percentOnlyQuota("5h", 42.5)
	if q.Used != 0 || q.Total != 0 {
		t.Errorf("Used/Total = %v/%v, 期望 0/0（不编造数据）", q.Used, q.Total)
	}
	if q.Percent != 42.5 {
		t.Errorf("Percent = %v, 期望 42.5", q.Percent)
	}
	if q.Unit != "percent" {
		t.Errorf("Unit = %q, 期望 \"percent\"", q.Unit)
	}
}

func TestRfc3339(t *testing.T) {
	t.Run("nil返回空串", func(t *testing.T) {
		if got := rfc3339(nil); got != "" {
			t.Errorf("rfc3339(nil) = %q, 期望空串", got)
		}
	})
	t.Run("零值返回空串", func(t *testing.T) {
		if got := rfc3339(&time.Time{}); got != "" {
			t.Errorf("零值时间 = %q, 期望空串", got)
		}
	})
	t.Run("转为UTC输出", func(t *testing.T) {
		tm := time.Date(2026, 9, 1, 20, 0, 0, 0, time.FixedZone("CST", 8*3600))
		got := rfc3339(&tm)
		if got != "2026-09-01T12:00:00Z" {
			t.Errorf("rfc3339 = %q, 期望 UTC 2026-09-01T12:00:00Z", got)
		}
	})
}

func TestConvertOpenRouter(t *testing.T) {
	t.Run("美元消费映射为cost", func(t *testing.T) {
		snap := &api.OpenRouterSnapshot{
			CapturedAt:   time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
			UsageDaily:   1.25,
			UsageMonthly: 30.5,
		}
		m := ConvertOpenRouter(snap)
		if m.Cost == nil {
			t.Fatal("Cost 为 nil")
		}
		if m.Cost.Today != 1.25 || m.Cost.Month != 30.5 || m.Cost.Currency != "USD" {
			t.Errorf("Cost = %v/%v %s, 期望 1.25/30.5 USD", m.Cost.Today, m.Cost.Month, m.Cost.Currency)
		}
		if m.Quota == nil || len(m.Quota) != 0 {
			t.Errorf("Quota 应为空 slice, got %#v", m.Quota)
		}
		// 平台侧无 token 数据源 → tokens 保持 null（避免误读，P3-04）
		if m.Tokens != nil {
			t.Errorf("Tokens 应为 nil, got %v", m.Tokens)
		}
	})

	t.Run("零消费也是真实数据", func(t *testing.T) {
		m := ConvertOpenRouter(&api.OpenRouterSnapshot{})
		if m.Cost == nil {
			t.Fatal("有快照时应输出 Cost（0 为真实用量）")
		}
		if m.Cost.Today != 0 || m.Cost.Month != 0 || m.Cost.Currency != "USD" {
			t.Errorf("Cost = %v/%v %s, 期望零值 USD", m.Cost.Today, m.Cost.Month, m.Cost.Currency)
		}
	})

	t.Run("空快照不输出假cost", func(t *testing.T) {
		m := ConvertOpenRouter(nil)
		if m == nil || m.Cost != nil || m.Quota == nil || len(m.Quota) != 0 {
			t.Errorf("nil 快照应返回空指标, got %#v", m)
		}
	})
}
