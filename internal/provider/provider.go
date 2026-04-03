// internal/provider/provider.go
package provider

import "context"

// EventType 流式事件类型
type EventType int

const (
	EventTextDelta EventType = iota
	EventThinking
	EventToolUse
	EventUsage
	EventDone
	EventError
)

// Provider 多模型提供者接口
type Provider interface {
	// Name 返回 Provider 标识符，如 "anthropic"、"openai"
	Name() string
	// Stream 发起流式对话请求，返回事件 channel
	Stream(ctx context.Context, req *Request) (<-chan Event, error)
	// Capabilities 返回该 Provider 支持的能力
	Capabilities() Capabilities
}

// SystemBlock 系统提示词块（支持 cache_control）
type SystemBlock struct {
	Text         string `json:"text"`
	CacheControl string `json:"cache_control,omitempty"` // "ephemeral" 表示启用缓存
}

// Request 统一的 API 请求
type Request struct {
	Model        string
	System       string         // 兼容：单字符串 system prompt
	SystemBlocks []SystemBlock  // 优先：分块 system prompt（支持 cache_control）
	Messages     []Message
	Tools        []ToolDef
	MaxTokens    int
	Temperature  float64
}

// EffectiveSystemBlocks 返回生效的 SystemBlock 列表
// 如果 SystemBlocks 非空则使用它，否则将 System string 转为单个 block
func (r *Request) EffectiveSystemBlocks() []SystemBlock {
	if len(r.SystemBlocks) > 0 {
		return r.SystemBlocks
	}
	if r.System != "" {
		return []SystemBlock{{Text: r.System}}
	}
	return nil
}

// ToolDef 工具定义（传给 API 的 schema）
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// ToolCall API 返回的工具调用
type ToolCall struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input string `json:"input"` // JSON string
}

// Usage token 使用量
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// ThinkingBlock Extended Thinking 内容
type ThinkingBlock struct {
	Text string `json:"text"`
}

// Event 流式事件
type Event struct {
	Type     EventType
	Text     string
	Thinking *ThinkingBlock
	ToolCall *ToolCall
	Usage    *Usage
	Error    error
}

// Capabilities Provider 支持的能力
type Capabilities struct {
	Thinking   bool
	Vision     bool
	ToolUse    bool
	Streaming  bool
	MaxContext int
}
