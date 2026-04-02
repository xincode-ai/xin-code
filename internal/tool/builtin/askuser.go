package builtin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xincode-ai/xin-code/internal/tool"
)

// AskUserTool 向用户提问工具
type AskUserTool struct {
	// AskFunc 由 TUI 注入的提问回调
	// 参数：问题文本，返回：用户回答
	AskFunc func(ctx context.Context, question string) (string, error)
}

type askUserInput struct {
	Question string `json:"question"`
}

func (t *AskUserTool) Name() string        { return "AskUser" }
func (t *AskUserTool) Description() string { return "向用户提问并等待回答。用于需要用户确认或提供信息时。" }
func (t *AskUserTool) IsReadOnly() bool    { return true }
func (t *AskUserTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{"type": "string", "description": "要向用户提出的问题"},
		},
		"required": []string{"question"},
	}
}

func (t *AskUserTool) Execute(ctx context.Context, input json.RawMessage) (*tool.Result, error) {
	var in askUserInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if t.AskFunc == nil {
		return &tool.Result{Content: "AskUser: no UI handler configured", IsError: true}, nil
	}

	answer, err := t.AskFunc(ctx, in.Question)
	if err != nil {
		return &tool.Result{Content: fmt.Sprintf("AskUser error: %s", err), IsError: true}, nil
	}

	return &tool.Result{Content: answer}, nil
}
