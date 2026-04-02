package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/xincode-ai/xin-code/internal/tool"
)

// EditTool 文件编辑工具（字符串替换）
type EditTool struct {
	// ConfirmFunc Diff 确认回调，返回 true 则写入，false 则取消
	// 为 nil 时直接写入（向后兼容）
	ConfirmFunc func(ctx context.Context, path string, diffText string) (bool, error)
}

type editInput struct {
	Path      string `json:"path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

func (t *EditTool) Name() string        { return "Edit" }
func (t *EditTool) Description() string {
	return "编辑文件：将 old_string 替换为 new_string。old_string 必须在文件中唯一匹配。"
}
func (t *EditTool) IsReadOnly() bool    { return false }
func (t *EditTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":       map[string]any{"type": "string", "description": "文件的绝对路径"},
			"old_string": map[string]any{"type": "string", "description": "要替换的原始文本（必须唯一匹配）"},
			"new_string": map[string]any{"type": "string", "description": "替换后的文本"},
		},
		"required": []string{"path", "old_string", "new_string"},
	}
}

func (t *EditTool) Execute(ctx context.Context, input json.RawMessage) (*tool.Result, error) {
	var in editInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	absPath, err := filepath.Abs(in.Path)
	if err != nil {
		return &tool.Result{Content: fmt.Sprintf("invalid path: %s", err), IsError: true}, nil
	}

	// 读取原文件
	data, err := os.ReadFile(absPath)
	if err != nil {
		return &tool.Result{Content: fmt.Sprintf("read error: %s", err), IsError: true}, nil
	}
	content := string(data)

	// 检查 old_string 唯一性
	count := strings.Count(content, in.OldString)
	if count == 0 {
		return &tool.Result{
			Content: "old_string not found in file",
			IsError: true,
		}, nil
	}
	if count > 1 {
		return &tool.Result{
			Content: fmt.Sprintf("old_string found %d times, must be unique", count),
			IsError: true,
		}, nil
	}

	// 执行替换
	newContent := strings.Replace(content, in.OldString, in.NewString, 1)

	// 计算 diff
	diffText := computeDiff(content, newContent)

	// Diff 确认
	if t.ConfirmFunc != nil {
		confirmed, err := t.ConfirmFunc(ctx, absPath, diffText)
		if err != nil {
			return &tool.Result{Content: fmt.Sprintf("confirm error: %s", err), IsError: true}, nil
		}
		if !confirmed {
			return &tool.Result{Content: "edit cancelled by user"}, nil
		}
	}

	// 写入文件
	if err := os.WriteFile(absPath, []byte(newContent), 0o644); err != nil {
		return &tool.Result{Content: fmt.Sprintf("write error: %s", err), IsError: true}, nil
	}

	return &tool.Result{Content: fmt.Sprintf("edited %s\n\n%s", absPath, diffText)}, nil
}

// computeDiff 计算并格式化 unified diff
func computeDiff(oldText, newText string) string {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(oldText, newText, true)

	var sb strings.Builder
	for _, d := range diffs {
		lines := strings.Split(d.Text, "\n")
		for i, line := range lines {
			if i == len(lines)-1 && line == "" {
				continue // 跳过末尾空行
			}
			switch d.Type {
			case diffmatchpatch.DiffInsert:
				sb.WriteString("+ " + line + "\n")
			case diffmatchpatch.DiffDelete:
				sb.WriteString("- " + line + "\n")
			case diffmatchpatch.DiffEqual:
				// 只显示上下文各 3 行
				if len(lines) > 7 && i >= 3 && i < len(lines)-3 {
					if i == 3 {
						sb.WriteString("  ...\n")
					}
					continue
				}
				sb.WriteString("  " + line + "\n")
			}
		}
	}
	return sb.String()
}
