package metrics

import (
	"sort"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/generic"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

// Package metrics 将内置 provider 的平台快照转换为 generic 统一指标模型。
// 全部转换函数均为纯函数：不访问 store/config/网络，便于单测与后续 provider 复用。

// rfc3339 将 *time.Time 格式化为 RFC3339 UTC 字符串；nil 或零值返回空串
func rfc3339(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// fillPercent 百分比兜底：Percent == 0 且 Total > 0 时按 Used/Total*100 计算
// （镜像 generic.buildMetrics 行为）
func fillPercent(used, total, percent float64) float64 {
	if percent == 0 && total > 0 {
		return used / total * 100
	}
	return percent
}

// percentOnlyQuota 构造 percent-only 窗口：Used/Total 置 0，仅填 Percent + Unit="percent"，
// 不编造数据。本批四家未涉及，供后续 percent-only 范式 provider（anthropic/codex/grok/kimi）复用。
func percentOnlyQuota(window string, percent float64) generic.QuotaMetric {
	return generic.QuotaMetric{
		Window:  window,
		Percent: percent,
		Unit:    "percent",
	}
}

// arkWindowOrder 镜像 web/ark_handlers.go arkQuotaDisplayOrder 的固定展示顺序
var arkWindowOrder = map[string]int{
	"five_hour":  1,
	"daily":      2,
	"weekly":     3,
	"monthly":    4,
	"cp_session": 5,
	"cp_weekly":  6,
	"cp_monthly": 7,
}

func arkWindowRank(window string) int {
	if order, ok := arkWindowOrder[window]; ok {
		return order
	}
	return 99 // 未知窗口排最后
}

func sortArkQuotas(quotas []generic.QuotaMetric) {
	sort.SliceStable(quotas, func(i, j int) bool {
		return arkWindowRank(quotas[i].Window) < arkWindowRank(quotas[j].Window)
	})
}

// ConvertArk 将火山方舟最新配额记录转换为统一指标模型。
// 窗口名原样透传并按 five_hour→daily→weekly→monthly 固定顺序排序（未知窗口排最后）；
// 官方用量单位未知，Unit 留空。
func ConvertArk(quotas []store.ArkLatestQuota) *generic.Metrics {
	m := &generic.Metrics{Quota: []generic.QuotaMetric{}}
	for _, q := range quotas {
		m.Quota = append(m.Quota, generic.QuotaMetric{
			Window:  q.Name,
			Used:    q.Used,
			Total:   q.Limit,
			Percent: fillPercent(q.Used, q.Limit, q.Utilization),
			ResetAt: rfc3339(q.ResetsAt),
		})
	}
	sortArkQuotas(m.Quota)
	return m
}

// ConvertZai 将智谱快照转换为统一指标模型。
// TIME_LIMIT 为五小时窗口（ADR-015：动态刷新无固定重置时间，ResetAt 留空）；
// TOKENS_LIMIT 为周窗口；窗口仅在对应 limit > 0 时输出（国内版可能只有部分窗口）。
// zai 的 tokens 字段是配额窗口而非用量统计，故 Cost/Tokens 维度为 nil。
func ConvertZai(snap *api.ZaiSnapshot) *generic.Metrics {
	m := &generic.Metrics{Quota: []generic.QuotaMetric{}}
	if snap == nil {
		return m
	}
	if snap.TimeLimit > 0 {
		m.Quota = append(m.Quota, generic.QuotaMetric{
			Window:  "5h",
			Used:    snap.TimeUsage,
			Total:   float64(snap.TimeLimit),
			Percent: fillPercent(snap.TimeUsage, float64(snap.TimeLimit), float64(snap.TimePercentage)),
		})
	}
	if snap.TokensLimit > 0 {
		m.Quota = append(m.Quota, generic.QuotaMetric{
			Window:  "weekly",
			Used:    snap.TokensUsage,
			Total:   float64(snap.TokensLimit),
			Percent: fillPercent(snap.TokensUsage, float64(snap.TokensLimit), float64(snap.TokensPercentage)),
			ResetAt: rfc3339(snap.TokensNextResetTime),
		})
	}
	return m
}

// ConvertDeepSeek 将 DeepSeek 快照转换为统一指标模型（纯余额平台，无配额窗口）。
func ConvertDeepSeek(snap *api.DeepSeekSnapshot) *generic.Metrics {
	m := &generic.Metrics{Quota: []generic.QuotaMetric{}}
	if snap == nil {
		return m
	}
	m.Balance = &generic.BalanceMetric{
		Amount:   snap.TotalBalance,
		Currency: snap.Currency,
	}
	return m
}

// ConvertOpenRouter 将 OpenRouter 快照转换为统一指标模型（P3-04）。
// UsageDaily/UsageMonthly 为美元消费（GET /api/v1/auth/key），映射为 cost 维度
// （Today=usage_daily、Month=usage_monthly、Currency=USD）；快照存在即输出，0 为真实用量。
// tokens 维度平台侧无数据源（遥测级数据走 API Integrations，见 docs/token-cost-data-sources.md）。
func ConvertOpenRouter(snap *api.OpenRouterSnapshot) *generic.Metrics {
	m := &generic.Metrics{Quota: []generic.QuotaMetric{}}
	if snap == nil {
		return m
	}
	m.Cost = &generic.CostMetric{
		Today:    snap.UsageDaily,
		Month:    snap.UsageMonthly,
		Currency: "USD",
	}
	return m
}

// ConvertOpenCode 将 OpenCode 最新配额记录转换为统一指标模型（保持 store 返回顺序）。
// Format 字段透传为 Unit（非空才填）。
func ConvertOpenCode(quotas []store.OpenCodeLatestQuota) *generic.Metrics {
	m := &generic.Metrics{Quota: []generic.QuotaMetric{}}
	for _, q := range quotas {
		m.Quota = append(m.Quota, generic.QuotaMetric{
			Window:  q.Name,
			Used:    q.Used,
			Total:   q.Limit,
			Percent: fillPercent(q.Used, q.Limit, q.Utilization),
			ResetAt: rfc3339(q.ResetsAt),
			Unit:    q.Format,
		})
	}
	return m
}
