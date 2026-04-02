// internal/auth/apikey.go
// 从配置文件读取 API Key
package auth

import (
	"encoding/json"
	"os"
)

// credentialsFile 凭据文件结构
type credentialsFile struct {
	APIKey string `json:"api_key"`
}

// readAPIKeyFromFile 从 JSON 文件读取 API Key
func readAPIKeyFromFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var creds credentialsFile
	if err := json.Unmarshal(data, &creds); err != nil {
		return ""
	}
	return creds.APIKey
}
