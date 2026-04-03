package builtin

import (
	"context"
	"encoding/json"
	"fmt"

	agentPkg "github.com/xincode-ai/xin-code/internal/agent"
	"github.com/xincode-ai/xin-code/internal/tool"
)

// SendMessageTool 向子 Agent 发送消息
type SendMessageTool struct {
	SubAgentReg *agentPkg.SubAgentRegistry
}

type sendMessageInput struct {
	To      string `json:"to"`
	Message string `json:"message"`
}

func (t *SendMessageTool) Name() string { return "SendMessage" }
func (t *SendMessageTool) Description() string {
	return "向运行中的子 Agent 发送消息。当前版本子 Agent 为同步执行，请使用 Agent 工具启动新的子任务。"
}
func (t *SendMessageTool) IsReadOnly() bool { return false }
func (t *SendMessageTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"to":      map[string]any{"type": "string", "description": "子 Agent 名称或 ID"},
			"message": map[string]any{"type": "string", "description": "要发送的消息"},
		},
		"required": []string{"to", "message"},
	}
}

func (t *SendMessageTool) Execute(_ context.Context, input json.RawMessage) (*tool.Result, error) {
	var in sendMessageInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if in.To == "" || in.Message == "" {
		return &tool.Result{Content: "to 和 message 都是必填项", IsError: true}, nil
	}

	// 查找目标子 agent
	if t.SubAgentReg == nil {
		return &tool.Result{
			Content: "子 Agent 注册表未初始化",
			IsError: true,
		}, nil
	}

	entry := t.SubAgentReg.Get(in.To)
	if entry == nil {
		// 列出当前可用的子 agent
		agents := t.SubAgentReg.List()
		if len(agents) == 0 {
			return &tool.Result{
				Content: fmt.Sprintf("找不到子 Agent %q，当前没有运行中的子 Agent。请使用 Agent 工具启动新的子任务。", in.To),
				IsError: true,
			}, nil
		}
		var names []string
		for _, a := range agents {
			status := "运行中"
			if a.Done {
				status = "已完成"
			}
			names = append(names, fmt.Sprintf("  - %s (%s) [%s]", a.Name, a.ID, status))
		}
		return &tool.Result{
			Content: fmt.Sprintf("找不到子 Agent %q。当前子 Agent:\n%s", in.To, joinLines(names)),
			IsError: true,
		}, nil
	}

	// 当前版本子 agent 为同步执行，不支持异步消息传递
	if entry.Done {
		return &tool.Result{
			Content: fmt.Sprintf("子 Agent %q 已完成执行。结果: %s\n请使用 Agent 工具启动新的子任务。", in.To, truncate(entry.Result, 200)),
		}, nil
	}

	// 同步模式下子 agent 正在阻塞父 agent，无法接收消息
	return &tool.Result{
		Content: "当前版本子 Agent 为同步执行模式，不支持异步消息传递。子 Agent 完成后请使用 Agent 工具启动新的子任务。",
	}, nil
}

// joinLines 用换行连接字符串
func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}

// truncate 截取字符串
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
