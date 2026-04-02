package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xincode-ai/xin-code/internal/tool"
)

// WriteTool 文件写入工具
type WriteTool struct{}

type writeInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (t *WriteTool) Name() string        { return "Write" }
func (t *WriteTool) Description() string { return "创建或覆写文件。自动创建不存在的目录。" }
func (t *WriteTool) IsReadOnly() bool    { return false }
func (t *WriteTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "文件的绝对路径"},
			"content": map[string]any{"type": "string", "description": "文件内容"},
		},
		"required": []string{"path", "content"},
	}
}

func (t *WriteTool) Execute(_ context.Context, input json.RawMessage) (*tool.Result, error) {
	var in writeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	absPath, err := filepath.Abs(in.Path)
	if err != nil {
		return &tool.Result{Content: fmt.Sprintf("invalid path: %s", err), IsError: true}, nil
	}

	// 路径安全校验：只允许工作目录内 + home 下的 .xincode/ 配置
	if resolved, err := filepath.EvalSymlinks(filepath.Dir(absPath)); err == nil {
		absPath = filepath.Join(resolved, filepath.Base(absPath))
	}
	cwd, _ := os.Getwd()
	if resolvedCwd, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolvedCwd
	}
	homeDir, _ := os.UserHomeDir()
	if resolvedHome, err := filepath.EvalSymlinks(homeDir); err == nil {
		homeDir = resolvedHome
	}
	xincodeDir := filepath.Join(homeDir, ".xincode") + string(filepath.Separator)
	if !strings.HasPrefix(absPath, cwd+string(filepath.Separator)) && absPath != cwd &&
		!strings.HasPrefix(absPath, xincodeDir) {
		return &tool.Result{
			Content: fmt.Sprintf("access denied: %s is outside working directory", in.Path),
			IsError: true,
		}, nil
	}

	// 自动创建目录
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &tool.Result{Content: fmt.Sprintf("mkdir error: %s", err), IsError: true}, nil
	}

	// 原子写入：先写临时文件再 rename
	tmpFile, err := os.CreateTemp(dir, ".xincode-write-*.tmp")
	if err != nil {
		return &tool.Result{Content: fmt.Sprintf("create temp file error: %s", err), IsError: true}, nil
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(in.Content); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return &tool.Result{Content: fmt.Sprintf("write error: %s", err), IsError: true}, nil
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return &tool.Result{Content: fmt.Sprintf("close error: %s", err), IsError: true}, nil
	}

	// 设置权限（如果目标文件已存在，继承权限；否则 0o644）
	perm := os.FileMode(0o644)
	if info, err := os.Stat(absPath); err == nil {
		perm = info.Mode().Perm()
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		os.Remove(tmpPath)
		return &tool.Result{Content: fmt.Sprintf("chmod error: %s", err), IsError: true}, nil
	}

	if err := os.Rename(tmpPath, absPath); err != nil {
		os.Remove(tmpPath)
		return &tool.Result{Content: fmt.Sprintf("rename error: %s", err), IsError: true}, nil
	}

	return &tool.Result{Content: fmt.Sprintf("wrote %d bytes to %s", len(in.Content), absPath)}, nil
}
