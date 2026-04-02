// internal/auth/auth.go
// 认证链：按优先级依次尝试获取 API Key
package auth

import (
	"os"
	"path/filepath"
)

// ResolveAPIKey 按优先级解析 API Key
// 优先级：环境变量 > 配置文件 > CC OAuth Token
func ResolveAPIKey(configDir string) (apiKey string, source string) {
	// 1. 环境变量（最高优先级）
	if v := os.Getenv("XINCODE_API_KEY"); v != "" {
		return v, "env:XINCODE_API_KEY"
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		return v, "env:ANTHROPIC_API_KEY"
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		return v, "env:OPENAI_API_KEY"
	}

	// 2. 配置文件
	if key := loadAPIKeyFromConfig(configDir); key != "" {
		return key, "config"
	}

	// 3. CC OAuth Token（平台相关，见 keychain_*.go）
	if token, err := LoadCCOAuthToken(); err == nil && token != "" {
		return token, "cc-oauth"
	}

	return "", "none"
}

// loadAPIKeyFromConfig 从配置文件读取 API Key
func loadAPIKeyFromConfig(configDir string) string {
	credPath := filepath.Join(configDir, "auth", "credentials.json")
	return readAPIKeyFromFile(credPath)
}
