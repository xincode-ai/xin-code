package cost

import (
	"fmt"
	"sync"
)

// Tracker 实时费用追踪器
type Tracker struct {
	mu       sync.RWMutex
	model    string
	currency string // "CNY" 或 "USD"

	// 累计 token 数
	totalInput              int
	totalOutput             int
	totalCacheCreation      int
	totalCacheRead          int

	// 累计费用（美元）
	totalCostUSD float64
}

// NewTracker 创建费用追踪器
func NewTracker(model, currency string) *Tracker {
	if currency == "" {
		currency = "CNY"
	}
	return &Tracker{
		model:    model,
		currency: currency,
	}
}

// AddUsage 记录一次 API 调用的 token 使用量
func (t *Tracker) AddUsage(inputTokens, outputTokens, cacheCreation, cacheRead int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.totalInput += inputTokens
	t.totalOutput += outputTokens
	t.totalCacheCreation += cacheCreation
	t.totalCacheRead += cacheRead

	// 计算本次费用
	pricing := GetPricing(t.model)
	cost := float64(inputTokens)*pricing.InputPerMillion/1_000_000 +
		float64(outputTokens)*pricing.OutputPerMillion/1_000_000 +
		float64(cacheCreation)*pricing.CacheWritePerMillion/1_000_000 +
		float64(cacheRead)*pricing.CacheReadPerMillion/1_000_000
	t.totalCostUSD += cost
}

// TotalCost 返回总费用（按当前货币）
func (t *Tracker) TotalCost() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.currency == "USD" {
		return t.totalCostUSD
	}
	return t.totalCostUSD * cnyPerUSD
}

// CostString 返回格式化的费用字符串
func (t *Tracker) CostString() string {
	cost := t.TotalCost()
	t.mu.RLock()
	currency := t.currency
	t.mu.RUnlock()

	if currency == "USD" {
		return fmt.Sprintf("$%.4f", cost)
	}
	return fmt.Sprintf("¥%.4f", cost)
}

// TotalTokens 返回总 token 数（输入+输出）
func (t *Tracker) TotalTokens() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.totalInput + t.totalOutput
}

// InputTokens 返回总输入 token 数
func (t *Tracker) InputTokens() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.totalInput
}

// OutputTokens 返回总输出 token 数
func (t *Tracker) OutputTokens() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.totalOutput
}

// SetCurrency 切换货币
func (t *Tracker) SetCurrency(currency string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.currency = currency
}
