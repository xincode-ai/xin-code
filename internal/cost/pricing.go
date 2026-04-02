package cost

import "strings"

// 模型价格表（美元/百万 tokens）
// 数据来源：各厂商官方定价页面

// ModelPricing 单个模型的价格信息
type ModelPricing struct {
	InputPerMillion        float64 // 输入 token 单价（$/M tokens）
	OutputPerMillion       float64 // 输出 token 单价（$/M tokens）
	CacheWritePerMillion   float64 // Cache 写入单价（$/M tokens）
	CacheReadPerMillion    float64 // Cache 读取单价（$/M tokens）
}

// 内嵌价格表
var pricingTable = map[string]ModelPricing{
	// Anthropic Claude
	"claude-sonnet-4-6-20250514": {InputPerMillion: 3.0, OutputPerMillion: 15.0, CacheWritePerMillion: 3.75, CacheReadPerMillion: 0.30},
	"claude-sonnet-4-20250514":   {InputPerMillion: 3.0, OutputPerMillion: 15.0, CacheWritePerMillion: 3.75, CacheReadPerMillion: 0.30},
	"claude-opus-4-20250514":     {InputPerMillion: 15.0, OutputPerMillion: 75.0, CacheWritePerMillion: 18.75, CacheReadPerMillion: 1.50},
	"claude-haiku-3-5-20241022":  {InputPerMillion: 0.80, OutputPerMillion: 4.0, CacheWritePerMillion: 1.0, CacheReadPerMillion: 0.08},

	// OpenAI GPT
	"gpt-4o":      {InputPerMillion: 2.50, OutputPerMillion: 10.0},
	"gpt-4o-mini": {InputPerMillion: 0.15, OutputPerMillion: 0.60},
	"o3":          {InputPerMillion: 10.0, OutputPerMillion: 40.0},
	"o4-mini":     {InputPerMillion: 1.10, OutputPerMillion: 4.40},
}

// CNY/USD 汇率（近似值，定期更新）
const cnyPerUSD = 7.25

// GetPricing 获取模型价格，先精确匹配，再尝试子串匹配，未知模型返回零值
func GetPricing(model string) ModelPricing {
	// 精确匹配
	if p, ok := pricingTable[model]; ok {
		return p
	}
	// 子串匹配：model 包含价格表中的 key，或 key 包含 model
	for key, p := range pricingTable {
		if strings.Contains(model, key) || strings.Contains(key, model) {
			return p
		}
	}
	return ModelPricing{}
}

// HasPricing 检查是否有该模型的价格数据
func HasPricing(model string) bool {
	if _, ok := pricingTable[model]; ok {
		return true
	}
	for key := range pricingTable {
		if strings.Contains(model, key) || strings.Contains(key, model) {
			return true
		}
	}
	return false
}
