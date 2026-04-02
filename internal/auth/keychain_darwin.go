//go:build darwin

// internal/auth/keychain_darwin.go
// macOS Keychain 读取 Claude Code 的 OAuth token
package auth

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CC 在 macOS Keychain 中的 service name
const ccKeychainService = "Claude Code-credentials"

// ccOAuthData Claude Code 存储在 Keychain 中的完整数据结构
type ccOAuthData struct {
	ClaudeAIOAuth *ccOAuthToken `json:"claudeAiOauth"`
}

// ccOAuthToken OAuth token 详情
type ccOAuthToken struct {
	AccessToken      string `json:"accessToken"`
	RefreshToken     string `json:"refreshToken"`
	ExpiresAt        int64  `json:"expiresAt"` // Unix 毫秒时间戳
	SubscriptionType string `json:"subscriptionType"`
}

// LoadCCOAuthToken 从 macOS Keychain 读取 Claude Code 的 OAuth access token
func LoadCCOAuthToken() (string, error) {
	// 使用 security 命令读取 Keychain
	cmd := exec.Command("security", "find-generic-password", "-s", ccKeychainService, "-w")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("keychain read failed: %w", err)
	}

	raw := strings.TrimSpace(string(output))
	if raw == "" {
		return "", fmt.Errorf("empty keychain data")
	}

	// 解析 JSON
	var data ccOAuthData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return "", fmt.Errorf("keychain data parse failed: %w", err)
	}

	if data.ClaudeAIOAuth == nil {
		return "", fmt.Errorf("no claudeAiOauth in keychain data")
	}

	// 检查 token 是否过期
	if data.ClaudeAIOAuth.ExpiresAt > 0 {
		expiresAt := time.UnixMilli(data.ClaudeAIOAuth.ExpiresAt)
		if time.Now().After(expiresAt) {
			return "", fmt.Errorf("cc oauth token expired at %s", expiresAt.Format(time.RFC3339))
		}
	}

	if data.ClaudeAIOAuth.AccessToken == "" {
		return "", fmt.Errorf("empty access token")
	}

	return data.ClaudeAIOAuth.AccessToken, nil
}
