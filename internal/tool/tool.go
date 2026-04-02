package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/xincode-ai/xin-code/internal/provider"
)

// Tool 工具接口
type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any
	IsReadOnly() bool
	Execute(ctx context.Context, input json.RawMessage) (*Result, error)
}

// Result 工具执行结果
type Result struct {
	Content string
	IsError bool
}

// Registry 工具注册表
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry 创建工具注册表
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register 注册工具
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get 获取工具
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// All 返回所有工具
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// ToolDefs 返回所有工具的 Provider ToolDef 格式（传给 API）
func (r *Registry) ToolDefs() []provider.ToolDef {
	tools := r.All()
	defs := make([]provider.ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = provider.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.InputSchema(),
		}
	}
	return defs
}

// ExecuteTool 执行单个工具调用
func (r *Registry) ExecuteTool(ctx context.Context, call *provider.ToolCall) *Result {
	t, ok := r.Get(call.Name)
	if !ok {
		return &Result{
			Content: fmt.Sprintf("unknown tool: %s", call.Name),
			IsError: true,
		}
	}
	result, err := t.Execute(ctx, json.RawMessage(call.Input))
	if err != nil {
		return &Result{
			Content: fmt.Sprintf("tool error: %s", err),
			IsError: true,
		}
	}
	return result
}
