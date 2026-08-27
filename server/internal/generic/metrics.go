package generic

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/PaesslerAG/jsonpath"
)

// Metrics 统一指标模型（所有平台归一化后的输出契约）
type Metrics struct {
	Quota   []QuotaMetric  `json:"quota"`
	Balance *BalanceMetric `json:"balance"`
	Cost    *CostMetric    `json:"cost"`
	Tokens  *TokensMetric  `json:"tokens"`
}

// QuotaMetric 配额窗口
type QuotaMetric struct {
	Window  string  `json:"window"`   // 窗口名（如 5h、7d）
	Used    float64 `json:"used"`     // 已用
	Total   float64 `json:"total"`    // 总量
	Percent float64 `json:"percent"`  // 百分比（0-100）
	ResetAt string  `json:"reset_at"` // 重置时间（RFC3339）
	Unit    string  `json:"unit"`     // 单位
}

// BalanceMetric 余额
type BalanceMetric struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

// CostMetric 费用
type CostMetric struct {
	Today    float64 `json:"today"`
	Month    float64 `json:"month"`
	Currency string  `json:"currency"`
}

// TokensMetric token 消耗
type TokensMetric struct {
	Today float64 `json:"today"`
	Month float64 `json:"month"`
}

// PlatformSnapshot 平台指标快照（含状态）
type PlatformSnapshot struct {
	Platform    string   `json:"platform"`
	DisplayName string   `json:"display_name"`
	Status      string   `json:"status"` // ok | error | auth_failed | unconfigured
	Error       string   `json:"error,omitempty"`
	UpdatedAt   string   `json:"updated_at"`
	Metrics     *Metrics `json:"metrics"`
}

// 状态常量
const (
	StatusOK           = "ok"
	StatusError        = "error"
	StatusAuthFailed   = "auth_failed"
	StatusUnconfigured = "unconfigured"
)

// mapSource 将单个数据源的接口响应映射为统一指标
// 返回该 source 对应的指标片段（quota 返回 QuotaMetric，其余返回字段值）
func mapSource(source SourceConfig, body []byte) (map[string]interface{}, error) {
	var doc interface{}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("generic: 解析接口响应失败: %w", err)
	}

	result := make(map[string]interface{})

	// 先应用静态值
	for field, val := range source.Static {
		result[field] = val
	}

	// 再应用 JSONPath 映射
	for field, expr := range source.Mapping {
		val, err := jsonpath.Get(expr, doc)
		if err != nil {
			// 字段缺失时跳过（不视为错误，避免单个字段缺失导致整个 source 失败）
			continue
		}
		result[field] = val
	}

	return result, nil
}

// buildMetrics 将 source 映射结果组装为统一指标模型
func buildMetrics(source SourceConfig, mapped map[string]interface{}) (*Metrics, error) {
	m := &Metrics{}

	switch source.Name {
	case SourceQuota:
		q := QuotaMetric{}
		if v, ok := mapped["quota.window"]; ok {
			q.Window = toString(v)
		}
		if v, ok := mapped["quota.used"]; ok {
			q.Used = toFloat(v)
		}
		if v, ok := mapped["quota.total"]; ok {
			q.Total = toFloat(v)
		}
		if v, ok := mapped["quota.percent"]; ok {
			q.Percent = toFloat(v)
		} else if q.Total > 0 {
			q.Percent = q.Used / q.Total * 100
		}
		if v, ok := mapped["quota.reset_at"]; ok {
			q.ResetAt = toString(v)
		}
		if v, ok := mapped["quota.unit"]; ok {
			q.Unit = toString(v)
		}
		m.Quota = append(m.Quota, q)

	case SourceBalance:
		b := &BalanceMetric{}
		if v, ok := mapped["balance.amount"]; ok {
			b.Amount = toFloat(v)
		}
		if v, ok := mapped["balance.currency"]; ok {
			b.Currency = toString(v)
		}
		m.Balance = b

	case SourceCost:
		c := &CostMetric{}
		if v, ok := mapped["cost.today"]; ok {
			c.Today = toFloat(v)
		}
		if v, ok := mapped["cost.month"]; ok {
			c.Month = toFloat(v)
		}
		if v, ok := mapped["cost.currency"]; ok {
			c.Currency = toString(v)
		}
		m.Cost = c

	case SourceTokens:
		t := &TokensMetric{}
		if v, ok := mapped["tokens.today"]; ok {
			t.Today = toFloat(v)
		}
		if v, ok := mapped["tokens.month"]; ok {
			t.Month = toFloat(v)
		}
		m.Tokens = t
	}

	return m, nil
}

// mergeMetrics 合并多个 source 的指标
func mergeMetrics(target *Metrics, src *Metrics) {
	if src == nil {
		return
	}
	if len(src.Quota) > 0 {
		target.Quota = append(target.Quota, src.Quota...)
	}
	if src.Balance != nil {
		target.Balance = src.Balance
	}
	if src.Cost != nil {
		target.Cost = src.Cost
	}
	if src.Tokens != nil {
		target.Tokens = src.Tokens
	}
}

// toFloat 将任意 JSON 值转为 float64
func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case json.Number:
		f, _ := val.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(val), 64)
		return f
	case bool:
		if val {
			return 1
		}
	}
	return 0
}

// toString 将任意 JSON 值转为字符串
func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case json.Number:
		return val.String()
	case bool:
		return strconv.FormatBool(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}