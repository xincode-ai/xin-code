//go:build !darwin

// internal/auth/keychain_other.go
// 非 macOS 平台的 CC OAuth 占位实现
package auth

import "fmt"

// LoadCCOAuthToken 非 macOS 平台暂不支持 CC OAuth
// TODO: Linux 支持通过 libsecret / secret-tool 读取
func LoadCCOAuthToken() (string, error) {
	return "", fmt.Errorf("cc oauth not supported on this platform")
}
