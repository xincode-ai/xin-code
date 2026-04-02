package provider

import (
	"fmt"
	"strings"
)

// ResolveProviderName 根据模型名推断 Provider
func ResolveProviderName(model string) string {
	model = strings.ToLower(model)
	switch {
	case strings.HasPrefix(model, "claude"),
		strings.HasPrefix(model, "sonnet"),
		strings.HasPrefix(model, "opus"),
		strings.HasPrefix(model, "haiku"):
		return "anthropic"
	case strings.HasPrefix(model, "gpt"),
		strings.HasPrefix(model, "o1"),
		strings.HasPrefix(model, "o3"),
		strings.HasPrefix(model, "o4"):
		return "openai"
	default:
		return "openai" // 兜底：走 OpenAI 兼容协议
	}
}

// NewProvider 根据名称创建 Provider 实例
func NewProvider(name, apiKey, model, baseURL string) (Provider, error) {
	switch name {
	case "anthropic":
		return NewAnthropicProvider(apiKey, model, baseURL)
	// case "openai": Phase 4 实现
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}
