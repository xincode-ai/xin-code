package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config 全局配置
type Config struct {
	Model      string           `json:"model"`
	Provider   string           `json:"provider"`
	APIKey     string           `json:"-"` // 不序列化到文件
	BaseURL    string           `json:"base_url,omitempty"`
	MaxTokens  int              `json:"max_tokens"`
	MaxTurns   int              `json:"max_turns"`
	Permission PermissionConfig `json:"permissions"`
	Cost       CostConfig       `json:"cost"`
	MCP        []MCPConfig      `json:"mcp_servers,omitempty"`
}

// MCPConfig MCP 服务器配置
type MCPConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// PermissionConfig 权限配置
type PermissionConfig struct {
	Mode  string           `json:"mode"` // bypass / acceptEdits / default / plan / interactive
	Rules []PermissionRule `json:"rules,omitempty"`
}

// PermissionRule 权限规则
type PermissionRule struct {
	Tool     string `json:"tool"`
	Behavior string `json:"behavior"` // allow / deny / ask
	Source   string `json:"source"`   // settings / project / session / cli
}

// CostConfig 费用配置
type CostConfig struct {
	Currency     string  `json:"currency"`      // CNY / USD
	Budget       float64 `json:"budget"`         // 预算上限
	BudgetAction string  `json:"budget_action"`  // warn / stop
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Model:     "claude-sonnet-4-6-20250514",
		Provider:  "anthropic",
		MaxTokens: 16384,
		MaxTurns:  100,
		Permission: PermissionConfig{
			Mode: "default",
		},
		Cost: CostConfig{
			Currency:     "CNY",
			Budget:       0, // 0 表示不限制
			BudgetAction: "warn",
		},
	}
}

// XinCodeDir 返回 ~/.xincode 目录路径
func XinCodeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".xincode")
}

// LoadConfig 加载配置（按优先级合并）
func LoadConfig() *Config {
	cfg := DefaultConfig()

	// 1. 全局配置
	globalPath := filepath.Join(XinCodeDir(), "settings.json")
	mergeConfigFile(cfg, globalPath)

	// 2. 项目配置
	projectPath := filepath.Join(".xincode", "settings.json")
	mergeConfigFile(cfg, projectPath)

	// 3. 环境变量
	if v := os.Getenv("XINCODE_API_KEY"); v != "" {
		cfg.APIKey = v
	} else if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("XINCODE_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("XINCODE_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("XINCODE_PERMISSION_MODE"); v != "" {
		cfg.Permission.Mode = v
	}

	return cfg
}

func mergeConfigFile(cfg *Config, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // 文件不存在则跳过
	}
	// 解析到临时结构，非零值覆盖
	var fileCfg Config
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return
	}
	if fileCfg.Model != "" {
		cfg.Model = fileCfg.Model
	}
	if fileCfg.Provider != "" {
		cfg.Provider = fileCfg.Provider
	}
	if fileCfg.BaseURL != "" {
		cfg.BaseURL = fileCfg.BaseURL
	}
	if fileCfg.MaxTokens > 0 {
		cfg.MaxTokens = fileCfg.MaxTokens
	}
	if fileCfg.MaxTurns > 0 {
		cfg.MaxTurns = fileCfg.MaxTurns
	}
	if fileCfg.Permission.Mode != "" {
		cfg.Permission.Mode = fileCfg.Permission.Mode
	}
	if len(fileCfg.Permission.Rules) > 0 {
		cfg.Permission.Rules = append(cfg.Permission.Rules, fileCfg.Permission.Rules...)
	}
	if fileCfg.Cost.Currency != "" {
		cfg.Cost.Currency = fileCfg.Cost.Currency
	}
	if fileCfg.Cost.Budget > 0 {
		cfg.Cost.Budget = fileCfg.Cost.Budget
	}
}
