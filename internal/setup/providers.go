package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ProviderInfo 预置 Provider 信息
type ProviderInfo struct {
	ID          string   // "anthropic", "openai", "openrouter", "custom"
	Name        string   // 显示名
	Description string   // 一句话描述
	BaseURL     string   // 默认 API 端点（custom 为空）
	EnvKey      string   // 对应的环境变量名
	Models      []string // 推荐模型列表（首个为默认）
}

// BuiltinProviders 预置 Provider 列表
var BuiltinProviders = []ProviderInfo{
	{
		ID:          "anthropic",
		Name:        "Anthropic (Claude)",
		Description: "Claude 系列模型，推荐",
		BaseURL:     "", // 使用 SDK 默认
		EnvKey:      "ANTHROPIC_API_KEY",
		Models: []string{
			"claude-sonnet-4-6-20250514",
			"claude-opus-4-6-20250610",
			"claude-haiku-4-5-20251001",
		},
	},
	{
		ID:          "openai",
		Name:        "OpenAI",
		Description: "GPT / o 系列模型",
		BaseURL:     "",
		EnvKey:      "OPENAI_API_KEY",
		Models: []string{
			"gpt-4.1",
			"o4-mini",
			"o3",
		},
	},
	{
		ID:          "openrouter",
		Name:        "OpenRouter",
		Description: "多模型聚合，支持数百个模型",
		BaseURL:     "https://openrouter.ai/api/v1",
		EnvKey:      "OPENROUTER_API_KEY",
		Models: []string{
			"anthropic/claude-sonnet-4-6",
			"openai/gpt-4.1",
			"google/gemini-2.5-pro",
		},
	},
	{
		ID:          "custom",
		Name:        "自定义端点",
		Description: "兼容 OpenAI API 的自定义服务",
		BaseURL:     "",
		EnvKey:      "",
		Models:      []string{},
	},
}

// SaveGlobalSettings 写入/合并全局设置到 settings.json
// fields 是需要更新的 key-value 对
func SaveGlobalSettings(path string, fields map[string]string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 读取现有配置
	existing := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &existing)
	}

	// 合并新字段
	for k, v := range fields {
		existing[k] = v
	}

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
