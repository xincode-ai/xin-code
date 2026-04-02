# Xin Code Phase 1: Foundation 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建 Xin Code 的最小可运行版本 — 一个能通过 Anthropic API 对话、读取文件、执行命令的终端 Agent CLI。

**Architecture:** Go 单二进制 CLI。根目录放入口和核心引擎（main.go, agent.go, config.go），internal/ 放 Provider 抽象、工具系统、上下文组装。Phase 1 使用简单 REPL（stdin/stdout），Phase 2 再替换为 Bubbletea TUI。

**Tech Stack:** Go 1.23+, github.com/anthropics/anthropic-sdk-go, github.com/spf13/cobra

**Spec:** `docs/superpowers/specs/2026-04-02-xin-code-design.md`

---

## Phase 总览

```
Phase 1 任务依赖图：

Task 1 (脚手架)
  └─→ Task 2 (Provider 类型)
       ├─→ Task 3 (配置系统)
       │    └─→ Task 4 (Anthropic Provider)
       └─→ Task 5 (Tool 类型)
            └─→ Task 6 (4 个核心工具)
                 └─→ Task 7 (Tool 执行器 + 权限)
                      └─→ Task 8 (Agent 循环)
                           └─→ Task 9 (REPL 集成)
                                └─→ Task 10 (构建 + CI)
```

---

### Task 1: 项目脚手架

**Files:**
- Create: `main.go`
- Create: `version.go`
- Create: `go.mod`
- Create: `.gitignore`
- Create: `LICENSE`

- [ ] **Step 1: 初始化 Go 模块**

```bash
cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code
go mod init github.com/xincode-ai/xin-code
```

- [ ] **Step 2: 创建 .gitignore**

```gitignore
# Binaries
xin-code
*.exe
dist/

# IDE
.idea/
.vscode/
*.swp

# OS
.DS_Store
Thumbs.db

# Go
vendor/

# Config (secrets)
.env
```

- [ ] **Step 3: 创建 LICENSE (MIT)**

```
MIT License

Copyright (c) 2026 Xin Code Contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 4: 创建 version.go**

```go
package main

// 构建时通过 ldflags 注入
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
```

- [ ] **Step 5: 创建最小 main.go**

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("xin-code %s (%s %s)\n", Version, Commit, Date)
		os.Exit(0)
	}
	fmt.Println("xin-code", Version)
}
```

- [ ] **Step 6: 验证构建**

Run: `go build -o xin-code . && ./xin-code --version`
Expected: `xin-code dev (none unknown)`

- [ ] **Step 7: 创建目录结构**

```bash
mkdir -p internal/provider
mkdir -p internal/tool/builtin
mkdir -p internal/context
mkdir -p internal/auth
```

- [ ] **Step 8: 初始化 git 并提交**

```bash
git init
git add .
git commit -m "feat: 项目脚手架初始化"
```

---

### Task 2: Provider 类型定义

**Files:**
- Create: `internal/provider/provider.go`
- Create: `internal/provider/message.go`
- Test: `internal/provider/message_test.go`

- [ ] **Step 1: 创建 Provider 接口和事件类型**

```go
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

// Request 统一的 API 请求
type Request struct {
	Model       string
	System      string
	Messages    []Message
	Tools       []ToolDef
	MaxTokens   int
	Temperature float64
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
```

- [ ] **Step 2: 创建统一消息类型**

```go
// internal/provider/message.go
package provider

// Role 消息角色
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// BlockType 内容块类型
type BlockType int

const (
	BlockText BlockType = iota
	BlockThinking
	BlockImage
	BlockToolUse
	BlockToolResult
)

// ContentBlock 消息内容块
type ContentBlock struct {
	Type       BlockType
	Text       string
	Thinking   string
	ImageURL   string
	ToolCall   *ToolCall
	ToolResult *ToolResult
}

// ToolResult 工具执行结果
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// Message 统一消息格式
type Message struct {
	Role    Role
	Content []ContentBlock
}

// NewTextMessage 创建纯文本消息
func NewTextMessage(role Role, text string) Message {
	return Message{
		Role: role,
		Content: []ContentBlock{
			{Type: BlockText, Text: text},
		},
	}
}

// NewToolResultMessage 创建工具结果消息
func NewToolResultMessage(toolUseID, content string, isError bool) Message {
	return Message{
		Role: RoleUser,
		Content: []ContentBlock{
			{
				Type: BlockToolResult,
				ToolResult: &ToolResult{
					ToolUseID: toolUseID,
					Content:   content,
					IsError:   isError,
				},
			},
		},
	}
}

// TextContent 提取消息中的纯文本内容
func (m Message) TextContent() string {
	var text string
	for _, block := range m.Content {
		if block.Type == BlockText {
			text += block.Text
		}
	}
	return text
}

// ToolCalls 提取消息中的工具调用
func (m Message) ToolCalls() []*ToolCall {
	var calls []*ToolCall
	for _, block := range m.Content {
		if block.Type == BlockToolUse && block.ToolCall != nil {
			calls = append(calls, block.ToolCall)
		}
	}
	return calls
}
```

- [ ] **Step 3: 写测试**

```go
// internal/provider/message_test.go
package provider

import "testing"

func TestNewTextMessage(t *testing.T) {
	msg := NewTextMessage(RoleUser, "hello")
	if msg.Role != RoleUser {
		t.Errorf("expected RoleUser, got %v", msg.Role)
	}
	if msg.TextContent() != "hello" {
		t.Errorf("expected 'hello', got '%s'", msg.TextContent())
	}
}

func TestNewToolResultMessage(t *testing.T) {
	msg := NewToolResultMessage("tool-1", "result text", false)
	if msg.Role != RoleUser {
		t.Errorf("expected RoleUser, got %v", msg.Role)
	}
	if len(msg.Content) != 1 || msg.Content[0].Type != BlockToolResult {
		t.Fatal("expected one ToolResult block")
	}
	if msg.Content[0].ToolResult.ToolUseID != "tool-1" {
		t.Errorf("expected tool-1, got %s", msg.Content[0].ToolResult.ToolUseID)
	}
}

func TestToolCalls(t *testing.T) {
	msg := Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			{Type: BlockText, Text: "Let me read that file"},
			{Type: BlockToolUse, ToolCall: &ToolCall{ID: "tc-1", Name: "Read", Input: `{"path":"main.go"}`}},
		},
	}
	calls := msg.ToolCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	if calls[0].Name != "Read" {
		t.Errorf("expected Read, got %s", calls[0].Name)
	}
}
```

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/provider/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/provider/
git commit -m "feat: Provider 接口和统一消息类型定义"
```

---

### Task 3: 配置系统

**Files:**
- Create: `config.go`
- Test: `config_test.go`

- [ ] **Step 1: 创建配置结构和加载逻辑**

```go
// config.go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config 全局配置
type Config struct {
	Model      string            `json:"model"`
	Provider   string            `json:"provider"`
	APIKey     string            `json:"-"` // 不序列化到文件
	BaseURL    string            `json:"base_url,omitempty"`
	MaxTokens  int               `json:"max_tokens"`
	MaxTurns   int               `json:"max_turns"`
	Permission PermissionConfig  `json:"permissions"`
	Cost       CostConfig        `json:"cost"`
}

// PermissionConfig 权限配置
type PermissionConfig struct {
	Mode  string           `json:"mode"` // bypass / acceptEdits / default / plan / interactive
	Rules []PermissionRule `json:"rules,omitempty"`
}

// PermissionRule 权限规则
type PermissionRule struct {
	Tool     string `json:"tool"`
	Behavior string `json:"behavior"` // allow / deny / ask
	Source   string `json:"source"`   // settings / project / session / cli
}

// CostConfig 费用配置
type CostConfig struct {
	Currency     string  `json:"currency"`      // CNY / USD
	Budget       float64 `json:"budget"`         // 预算上限
	BudgetAction string  `json:"budget_action"`  // warn / stop
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Model:     "claude-sonnet-4-6-20250514",
		Provider:  "anthropic",
		MaxTokens: 16384,
		MaxTurns:  100,
		Permission: PermissionConfig{
			Mode: "default",
		},
		Cost: CostConfig{
			Currency:     "CNY",
			Budget:       0, // 0 表示不限制
			BudgetAction: "warn",
		},
	}
}

// XinCodeDir 返回 ~/.xincode 目录路径
func XinCodeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".xincode")
}

// LoadConfig 加载配置（按优先级合并）
func LoadConfig() *Config {
	cfg := DefaultConfig()

	// 1. 全局配置
	globalPath := filepath.Join(XinCodeDir(), "settings.json")
	mergeConfigFile(cfg, globalPath)

	// 2. 项目配置
	projectPath := filepath.Join(".xincode", "settings.json")
	mergeConfigFile(cfg, projectPath)

	// 3. 环境变量
	if v := os.Getenv("XINCODE_API_KEY"); v != "" {
		cfg.APIKey = v
	} else if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("XINCODE_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("XINCODE_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("XINCODE_PERMISSION_MODE"); v != "" {
		cfg.Permission.Mode = v
	}

	return cfg
}

func mergeConfigFile(cfg *Config, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // 文件不存在则跳过
	}
	// 解析到临时结构，非零值覆盖
	var fileCfg Config
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return
	}
	if fileCfg.Model != "" {
		cfg.Model = fileCfg.Model
	}
	if fileCfg.Provider != "" {
		cfg.Provider = fileCfg.Provider
	}
	if fileCfg.BaseURL != "" {
		cfg.BaseURL = fileCfg.BaseURL
	}
	if fileCfg.MaxTokens > 0 {
		cfg.MaxTokens = fileCfg.MaxTokens
	}
	if fileCfg.MaxTurns > 0 {
		cfg.MaxTurns = fileCfg.MaxTurns
	}
	if fileCfg.Permission.Mode != "" {
		cfg.Permission.Mode = fileCfg.Permission.Mode
	}
	if len(fileCfg.Permission.Rules) > 0 {
		cfg.Permission.Rules = append(cfg.Permission.Rules, fileCfg.Permission.Rules...)
	}
	if fileCfg.Cost.Currency != "" {
		cfg.Cost.Currency = fileCfg.Cost.Currency
	}
	if fileCfg.Cost.Budget > 0 {
		cfg.Cost.Budget = fileCfg.Cost.Budget
	}
}
```

- [ ] **Step 2: 写测试**

```go
// config_test.go
package main

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Model == "" {
		t.Error("Model should not be empty")
	}
	if cfg.Permission.Mode != "default" {
		t.Errorf("expected 'default', got '%s'", cfg.Permission.Mode)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	os.Setenv("XINCODE_MODEL", "gpt-4o")
	os.Setenv("XINCODE_API_KEY", "test-key")
	defer os.Unsetenv("XINCODE_MODEL")
	defer os.Unsetenv("XINCODE_API_KEY")

	cfg := LoadConfig()
	if cfg.Model != "gpt-4o" {
		t.Errorf("expected 'gpt-4o', got '%s'", cfg.Model)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("expected 'test-key', got '%s'", cfg.APIKey)
	}
}
```

- [ ] **Step 3: 运行测试**

Run: `go test -v -run TestDefaultConfig && go test -v -run TestLoadConfigFromEnv`
Expected: PASS

- [ ] **Step 4: 提交**

```bash
git add config.go config_test.go
git commit -m "feat: 配置系统 — 多层级加载 + 环境变量"
```

---

### Task 4: Anthropic Provider

**Files:**
- Create: `internal/provider/anthropic.go`
- Create: `internal/provider/registry.go`
- Test: `internal/provider/registry_test.go`

- [ ] **Step 1: 安装 Anthropic SDK**

```bash
go get github.com/anthropics/anthropic-sdk-go
```

- [ ] **Step 2: 实现 AnthropicProvider**

```go
// internal/provider/anthropic.go
package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicProvider Claude API Provider
type AnthropicProvider struct {
	client *anthropic.Client
	model  string
}

// NewAnthropicProvider 创建 Anthropic Provider
func NewAnthropicProvider(apiKey, model, baseURL string) (*AnthropicProvider, error) {
	opts := []option.RequestOption{
		anthropic.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := anthropic.NewClient(opts...)
	return &AnthropicProvider{
		client: &client,
		model:  model,
	}, nil
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) Capabilities() Capabilities {
	return Capabilities{
		Thinking:   strings.Contains(p.model, "opus") || strings.Contains(p.model, "sonnet"),
		Vision:     true,
		ToolUse:    true,
		Streaming:  true,
		MaxContext:  200000,
	}
}

func (p *AnthropicProvider) Stream(ctx context.Context, req *Request) (<-chan Event, error) {
	ch := make(chan Event, 64)

	// 转换消息格式
	messages := convertToAnthropicMessages(req.Messages)

	// 构建 API 参数
	params := anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: int64(req.MaxTokens),
		Messages:  messages,
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{
			anthropic.NewTextBlock(req.System),
		}
	}

	// 转换工具定义
	if len(req.Tools) > 0 {
		params.Tools = convertToAnthropicTools(req.Tools)
	}

	go func() {
		defer close(ch)

		stream := p.client.Messages.NewStreaming(ctx, params)
		for stream.Next() {
			evt := stream.Current()
			events := convertAnthropicEvent(evt)
			for _, e := range events {
				select {
				case ch <- e:
				case <-ctx.Done():
					return
				}
			}
		}
		if err := stream.Err(); err != nil {
			ch <- Event{Type: EventError, Error: fmt.Errorf("anthropic stream error: %w", err)}
			return
		}

		// 发送最终 usage
		msg := stream.FinalMessage()
		if msg != nil {
			ch <- Event{
				Type: EventUsage,
				Usage: &Usage{
					InputTokens:  int(msg.Usage.InputTokens),
					OutputTokens: int(msg.Usage.OutputTokens),
				},
			}
		}
		ch <- Event{Type: EventDone}
	}()

	return ch, nil
}

func convertToAnthropicMessages(msgs []Message) []anthropic.MessageParam {
	var result []anthropic.MessageParam
	for _, msg := range msgs {
		var blocks []anthropic.ContentBlockParamUnion
		for _, block := range msg.Content {
			switch block.Type {
			case BlockText:
				blocks = append(blocks, anthropic.NewTextBlock(block.Text))
			case BlockToolUse:
				// 工具调用由 API 返回，不需要在请求中发送
			case BlockToolResult:
				if block.ToolResult != nil {
					blocks = append(blocks, anthropic.NewToolResultBlock(
						block.ToolResult.ToolUseID,
						block.ToolResult.Content,
						block.ToolResult.IsError,
					))
				}
			}
		}
		if len(blocks) > 0 {
			result = append(result, anthropic.MessageParam{
				Role:    anthropic.MessageParamRole(msg.Role),
				Content: blocks,
			})
		}
	}
	return result
}

func convertToAnthropicTools(tools []ToolDef) []anthropic.ToolUnionParam {
	var result []anthropic.ToolUnionParam
	for _, t := range tools {
		result = append(result, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.String(t.Description),
				InputSchema: t.InputSchema,
			},
		})
	}
	return result
}

func convertAnthropicEvent(evt anthropic.MessageStreamEvent) []Event {
	var events []Event

	switch e := evt.AsAny().(type) {
	case anthropic.ContentBlockDeltaEvent:
		switch delta := e.Delta.AsAny().(type) {
		case anthropic.TextDelta:
			events = append(events, Event{Type: EventTextDelta, Text: delta.Text})
		case anthropic.InputJSONDelta:
			// 工具参数增量，在 ContentBlockStop 时处理完整的工具调用
		}
	case anthropic.ContentBlockStartEvent:
		switch cb := e.ContentBlock.AsAny().(type) {
		case anthropic.ToolUseBlock:
			events = append(events, Event{
				Type: EventToolUse,
				ToolCall: &ToolCall{
					ID:   cb.ID,
					Name: cb.Name,
				},
			})
		case anthropic.ThinkingBlock:
			events = append(events, Event{
				Type:     EventThinking,
				Thinking: &ThinkingBlock{Text: cb.Thinking},
			})
		}
	}

	return events
}
```

> 注意: Anthropic Go SDK 的 API 在实际集成时可能需要调整（SDK 版本更新快）。上面是基于 SDK v1.x 的主要思路，具体 API 签名以实际 `go get` 获取的版本为准。

- [ ] **Step 3: 创建 Provider 注册表**

```go
// internal/provider/registry.go
package provider

import (
	"fmt"
	"strings"
)

// ResolveProviderName 根据模型名推断 Provider
func ResolveProviderName(model string) string {
	model = strings.ToLower(model)
	switch {
	case strings.HasPrefix(model, "claude"),
		strings.HasPrefix(model, "sonnet"),
		strings.HasPrefix(model, "opus"),
		strings.HasPrefix(model, "haiku"):
		return "anthropic"
	case strings.HasPrefix(model, "gpt"),
		strings.HasPrefix(model, "o1"),
		strings.HasPrefix(model, "o3"),
		strings.HasPrefix(model, "o4"):
		return "openai"
	default:
		return "openai" // 兜底：走 OpenAI 兼容协议
	}
}

// NewProvider 根据名称创建 Provider 实例
func NewProvider(name, apiKey, model, baseURL string) (Provider, error) {
	switch name {
	case "anthropic":
		return NewAnthropicProvider(apiKey, model, baseURL)
	// case "openai": Phase 4 实现
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}
```

- [ ] **Step 4: 写测试**

```go
// internal/provider/registry_test.go
package provider

import "testing"

func TestResolveProviderName(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"claude-sonnet-4-6", "anthropic"},
		{"claude-opus-4", "anthropic"},
		{"sonnet-4-6", "anthropic"},
		{"haiku-4-5", "anthropic"},
		{"gpt-4o", "openai"},
		{"o1-preview", "openai"},
		{"o3-mini", "openai"},
		{"deepseek-chat", "openai"},       // 兜底
		{"unknown-model", "openai"},       // 兜底
	}
	for _, tt := range tests {
		got := ResolveProviderName(tt.model)
		if got != tt.expected {
			t.Errorf("ResolveProviderName(%q) = %q, want %q", tt.model, got, tt.expected)
		}
	}
}
```

- [ ] **Step 5: 运行测试**

Run: `go test ./internal/provider/ -v`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add internal/provider/ go.mod go.sum
git commit -m "feat: Anthropic Provider + Provider 注册表"
```

---

### Task 5: Tool 接口定义

**Files:**
- Create: `internal/tool/tool.go`
- Test: `internal/tool/tool_test.go`

- [ ] **Step 1: 创建 Tool 接口和 Registry**

```go
// internal/tool/tool.go
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
```

- [ ] **Step 2: 写测试**

```go
// internal/tool/tool_test.go
package tool

import (
	"context"
	"encoding/json"
	"testing"
)

// mockTool 用于测试的 mock 工具
type mockTool struct {
	name     string
	readOnly bool
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string { return "mock tool" }
func (m *mockTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (m *mockTool) IsReadOnly() bool { return m.readOnly }
func (m *mockTool) Execute(_ context.Context, _ json.RawMessage) (*Result, error) {
	return &Result{Content: "ok"}, nil
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockTool{name: "Read", readOnly: true})
	reg.Register(&mockTool{name: "Bash", readOnly: false})

	if _, ok := reg.Get("Read"); !ok {
		t.Error("expected Read to be registered")
	}
	if _, ok := reg.Get("NotExist"); ok {
		t.Error("expected NotExist to not be registered")
	}
	if len(reg.All()) != 2 {
		t.Errorf("expected 2 tools, got %d", len(reg.All()))
	}
	defs := reg.ToolDefs()
	if len(defs) != 2 {
		t.Errorf("expected 2 tool defs, got %d", len(defs))
	}
}
```

- [ ] **Step 3: 运行测试**

Run: `go test ./internal/tool/ -v`
Expected: PASS

- [ ] **Step 4: 提交**

```bash
git add internal/tool/tool.go internal/tool/tool_test.go
git commit -m "feat: Tool 接口 + Registry"
```

---

### Task 6: 4 个核心工具 (Read, Bash, Glob, Grep)

**Files:**
- Create: `internal/tool/builtin/read.go`
- Create: `internal/tool/builtin/bash.go`
- Create: `internal/tool/builtin/glob.go`
- Create: `internal/tool/builtin/grep.go`
- Create: `internal/tool/builtin/register.go`
- Test: `internal/tool/builtin/read_test.go`
- Test: `internal/tool/builtin/bash_test.go`

- [ ] **Step 1: 实现 Read 工具**

```go
// internal/tool/builtin/read.go
package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/xincode-ai/xin-code/internal/tool"
)

type ReadTool struct{}

type readInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

func (t *ReadTool) Name() string        { return "Read" }
func (t *ReadTool) Description() string { return "读取文件内容。支持通过 offset 和 limit 读取部分内容。" }
func (t *ReadTool) IsReadOnly() bool    { return true }
func (t *ReadTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":   map[string]any{"type": "string", "description": "文件的绝对路径"},
			"offset": map[string]any{"type": "integer", "description": "起始行号（从 0 开始）"},
			"limit":  map[string]any{"type": "integer", "description": "读取的行数"},
		},
		"required": []string{"path"},
	}
}

func (t *ReadTool) Execute(_ context.Context, input json.RawMessage) (*tool.Result, error) {
	var in readInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}
	data, err := os.ReadFile(in.Path)
	if err != nil {
		return &tool.Result{Content: fmt.Sprintf("error reading file: %s", err), IsError: true}, nil
	}
	content := string(data)

	// 按行切分并应用 offset/limit
	if in.Offset > 0 || in.Limit > 0 {
		lines := strings.Split(content, "\n")
		start := in.Offset
		if start >= len(lines) {
			return &tool.Result{Content: ""}, nil
		}
		end := len(lines)
		if in.Limit > 0 && start+in.Limit < end {
			end = start + in.Limit
		}
		// 添加行号
		var numbered []string
		for i := start; i < end; i++ {
			numbered = append(numbered, fmt.Sprintf("%d\t%s", i+1, lines[i]))
		}
		content = strings.Join(numbered, "\n")
	}

	return &tool.Result{Content: content}, nil
}
```

- [ ] **Step 2: 实现 Bash 工具**

```go
// internal/tool/builtin/bash.go
package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/xincode-ai/xin-code/internal/tool"
)

type BashTool struct{}

type bashInput struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"` // 毫秒
}

func (t *BashTool) Name() string        { return "Bash" }
func (t *BashTool) Description() string { return "执行 shell 命令并返回输出。" }
func (t *BashTool) IsReadOnly() bool    { return false }
func (t *BashTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{"type": "string", "description": "要执行的 shell 命令"},
			"timeout": map[string]any{"type": "integer", "description": "超时时间（毫秒），默认 120000"},
		},
		"required": []string{"command"},
	}
}

func (t *BashTool) Execute(ctx context.Context, input json.RawMessage) (*tool.Result, error) {
	var in bashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	timeout := 120 * time.Second
	if in.Timeout > 0 {
		timeout = time.Duration(in.Timeout) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", in.Command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var result strings.Builder
	if stdout.Len() > 0 {
		result.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString("STDERR:\n")
		result.WriteString(stderr.String())
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &tool.Result{Content: "command timed out", IsError: true}, nil
		}
		exitErr := ""
		if result.Len() > 0 {
			exitErr = result.String() + "\n"
		}
		return &tool.Result{
			Content: fmt.Sprintf("%sexit status: %s", exitErr, err),
			IsError: true,
		}, nil
	}

	output := result.String()
	if output == "" {
		output = "(no output)"
	}
	return &tool.Result{Content: output}, nil
}
```

> 注意: bash.go 使用了 `strings.Builder` 但缺少 import。实际实现时在 import 块中加上 `"strings"`。

- [ ] **Step 3: 实现 Glob 工具**

```go
// internal/tool/builtin/glob.go
package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xincode-ai/xin-code/internal/tool"
)

type GlobTool struct{}

type globInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

func (t *GlobTool) Name() string        { return "Glob" }
func (t *GlobTool) Description() string { return "按 glob 模式匹配文件路径。" }
func (t *GlobTool) IsReadOnly() bool    { return true }
func (t *GlobTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Glob 模式，如 **/*.go"},
			"path":    map[string]any{"type": "string", "description": "搜索根目录，默认当前目录"},
		},
		"required": []string{"pattern"},
	}
}

func (t *GlobTool) Execute(_ context.Context, input json.RawMessage) (*tool.Result, error) {
	var in globInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	root := in.Path
	if root == "" {
		root = "."
	}

	pattern := filepath.Join(root, in.Pattern)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return &tool.Result{Content: fmt.Sprintf("glob error: %s", err), IsError: true}, nil
	}

	sort.Strings(matches)
	if len(matches) == 0 {
		return &tool.Result{Content: "no matches found"}, nil
	}
	return &tool.Result{Content: strings.Join(matches, "\n")}, nil
}
```

- [ ] **Step 4: 实现 Grep 工具**

```go
// internal/tool/builtin/grep.go
package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/xincode-ai/xin-code/internal/tool"
)

type GrepTool struct{}

type grepInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
	Glob    string `json:"glob,omitempty"`
}

func (t *GrepTool) Name() string        { return "Grep" }
func (t *GrepTool) Description() string { return "搜索文件内容，支持正则表达式。底层使用 grep -rn。" }
func (t *GrepTool) IsReadOnly() bool    { return true }
func (t *GrepTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "搜索模式（正则表达式）"},
			"path":    map[string]any{"type": "string", "description": "搜索路径，默认当前目录"},
			"glob":    map[string]any{"type": "string", "description": "文件过滤，如 *.go"},
		},
		"required": []string{"pattern"},
	}
}

func (t *GrepTool) Execute(ctx context.Context, input json.RawMessage) (*tool.Result, error) {
	var in grepInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	path := in.Path
	if path == "" {
		path = "."
	}

	// 优先用 ripgrep，回退到 grep
	args := []string{"-rn", "--color=never"}
	bin := "grep"
	if _, err := exec.LookPath("rg"); err == nil {
		bin = "rg"
		args = []string{"-n", "--no-heading", "--color=never"}
		if in.Glob != "" {
			args = append(args, "--glob", in.Glob)
		}
	} else if in.Glob != "" {
		args = append(args, "--include="+in.Glob)
	}
	args = append(args, in.Pattern, path)

	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()

	// grep 退出码 1 表示没有匹配，不是错误
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return &tool.Result{Content: "no matches found"}, nil
		}
		return &tool.Result{
			Content: fmt.Sprintf("grep error: %s\n%s", err, stderr.String()),
			IsError: true,
		}, nil
	}

	if output == "" {
		return &tool.Result{Content: "no matches found"}, nil
	}

	// 限制输出大小
	const maxOutput = 50 * 1024
	if len(output) > maxOutput {
		output = output[:maxOutput] + "\n... (output truncated)"
	}

	return &tool.Result{Content: output}, nil
}
```

- [ ] **Step 5: 创建工具注册函数**

```go
// internal/tool/builtin/register.go
package builtin

import "github.com/xincode-ai/xin-code/internal/tool"

// RegisterAll 注册所有内置工具
func RegisterAll(reg *tool.Registry) {
	reg.Register(&ReadTool{})
	reg.Register(&BashTool{})
	reg.Register(&GlobTool{})
	reg.Register(&GrepTool{})
}
```

- [ ] **Step 6: 写 Read 和 Bash 的测试**

```go
// internal/tool/builtin/read_test.go
package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadTool(t *testing.T) {
	// 创建临时文件
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0644)

	tool := &ReadTool{}
	input, _ := json.Marshal(readInput{Path: path})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "line1\nline2\nline3\n" {
		t.Errorf("unexpected content: %q", result.Content)
	}
}

func TestReadToolNotFound(t *testing.T) {
	tool := &ReadTool{}
	input, _ := json.Marshal(readInput{Path: "/nonexistent/file"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for nonexistent file")
	}
}
```

```go
// internal/tool/builtin/bash_test.go
package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestBashTool(t *testing.T) {
	tool := &BashTool{}
	input, _ := json.Marshal(bashInput{Command: "echo hello"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "hello") {
		t.Errorf("expected 'hello' in output, got: %s", result.Content)
	}
}

func TestBashToolError(t *testing.T) {
	tool := &BashTool{}
	input, _ := json.Marshal(bashInput{Command: "exit 1"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for exit 1")
	}
}
```

- [ ] **Step 7: 运行所有测试**

Run: `go test ./internal/tool/... -v`
Expected: PASS

- [ ] **Step 8: 提交**

```bash
git add internal/tool/
git commit -m "feat: 4 个核心工具 — Read, Bash, Glob, Grep"
```

---

### Task 7: Tool 执行器 + 基础权限

**Files:**
- Create: `internal/tool/executor.go`
- Create: `internal/tool/permission.go`
- Test: `internal/tool/executor_test.go`

- [ ] **Step 1: 实现并发执行器**

```go
// internal/tool/executor.go
package tool

import (
	"context"
	"sync"

	"github.com/xincode-ai/xin-code/internal/provider"
)

const maxConcurrentReadTools = 10

// ExecuteResult 单个工具执行结果
type ExecuteResult struct {
	ToolUseID string
	Result    *Result
}

// ExecuteBatch 批量执行工具调用
// 只读工具并发执行，写入工具顺序执行
func (r *Registry) ExecuteBatch(ctx context.Context, calls []*provider.ToolCall, checker PermissionChecker) []ExecuteResult {
	results := make([]ExecuteResult, len(calls))

	// 分组：只读 vs 写入
	type indexedCall struct {
		index int
		call  *provider.ToolCall
	}
	var readCalls, writeCalls []indexedCall

	for i, call := range calls {
		t, ok := r.Get(call.Name)
		if !ok || !t.IsReadOnly() {
			writeCalls = append(writeCalls, indexedCall{i, call})
		} else {
			readCalls = append(readCalls, indexedCall{i, call})
		}
	}

	// 只读工具并发执行
	if len(readCalls) > 0 {
		sem := make(chan struct{}, maxConcurrentReadTools)
		var wg sync.WaitGroup
		for _, ic := range readCalls {
			wg.Add(1)
			sem <- struct{}{}
			go func(ic indexedCall) {
				defer wg.Done()
				defer func() { <-sem }()
				result := r.executeWithPermission(ctx, ic.call, checker)
				results[ic.index] = ExecuteResult{
					ToolUseID: ic.call.ID,
					Result:    result,
				}
			}(ic)
		}
		wg.Wait()
	}

	// 写入工具顺序执行
	for _, ic := range writeCalls {
		result := r.executeWithPermission(ctx, ic.call, checker)
		results[ic.index] = ExecuteResult{
			ToolUseID: ic.call.ID,
			Result:    result,
		}
	}

	return results
}

func (r *Registry) executeWithPermission(ctx context.Context, call *provider.ToolCall, checker PermissionChecker) *Result {
	t, ok := r.Get(call.Name)
	if !ok {
		return &Result{Content: "unknown tool: " + call.Name, IsError: true}
	}

	// 权限检查
	if checker != nil {
		allowed, reason := checker.Check(call.Name, t.IsReadOnly())
		if !allowed {
			return &Result{Content: "permission denied: " + reason, IsError: true}
		}
	}

	result, err := t.Execute(ctx, []byte(call.Input))
	if err != nil {
		return &Result{Content: "execution error: " + err.Error(), IsError: true}
	}
	return result
}
```

- [ ] **Step 2: 实现基础权限检查**

```go
// internal/tool/permission.go
package tool

// PermissionChecker 权限检查接口
type PermissionChecker interface {
	Check(toolName string, isReadOnly bool) (allowed bool, reason string)
}

// PermissionMode 权限模式
type PermissionMode string

const (
	ModeBypass      PermissionMode = "bypass"
	ModeAcceptEdits PermissionMode = "acceptEdits"
	ModeDefault     PermissionMode = "default"
	ModePlan        PermissionMode = "plan"
	ModeInteractive PermissionMode = "interactive"
)

// SimplePermissionChecker 基于模式的简单权限检查器
// Phase 1 使用简单实现，Phase 2 加入规则系统和用户交互
type SimplePermissionChecker struct {
	Mode PermissionMode
}

func (c *SimplePermissionChecker) Check(toolName string, isReadOnly bool) (bool, string) {
	switch c.Mode {
	case ModeBypass:
		return true, ""
	case ModeAcceptEdits:
		// 文件操作自动放行，Bash 等需要检查
		if isReadOnly || toolName == "Write" || toolName == "Edit" {
			return true, ""
		}
		// Phase 2: 这里会弹出 TUI 确认对话框
		// Phase 1: 暂时自动放行
		return true, ""
	case ModeDefault:
		if isReadOnly {
			return true, ""
		}
		// Phase 1: 暂时自动放行写入工具
		// Phase 2: 弹出 TUI 确认
		return true, ""
	case ModePlan:
		if isReadOnly {
			return true, ""
		}
		return false, "plan mode: write operations are not allowed"
	case ModeInteractive:
		// Phase 2: 所有工具都弹框
		// Phase 1: 暂时自动放行
		return true, ""
	default:
		return true, ""
	}
}
```

- [ ] **Step 3: 写测试**

```go
// internal/tool/executor_test.go
package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/xincode-ai/xin-code/internal/provider"
)

type echoTool struct {
	name     string
	readOnly bool
}

func (t *echoTool) Name() string        { return t.name }
func (t *echoTool) Description() string { return "echo" }
func (t *echoTool) InputSchema() map[string]any {
	return map[string]any{"type": "object"}
}
func (t *echoTool) IsReadOnly() bool { return t.readOnly }
func (t *echoTool) Execute(_ context.Context, input json.RawMessage) (*Result, error) {
	return &Result{Content: t.name + ": ok"}, nil
}

func TestExecuteBatch(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&echoTool{name: "Read", readOnly: true})
	reg.Register(&echoTool{name: "Glob", readOnly: true})
	reg.Register(&echoTool{name: "Bash", readOnly: false})

	calls := []*provider.ToolCall{
		{ID: "1", Name: "Read", Input: "{}"},
		{ID: "2", Name: "Glob", Input: "{}"},
		{ID: "3", Name: "Bash", Input: "{}"},
	}
	checker := &SimplePermissionChecker{Mode: ModeBypass}
	results := reg.ExecuteBatch(context.Background(), calls, checker)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, r := range results {
		if r.Result.IsError {
			t.Errorf("result %d unexpected error: %s", i, r.Result.Content)
		}
	}
}

func TestPlanModeBlocksWrites(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&echoTool{name: "Bash", readOnly: false})

	calls := []*provider.ToolCall{
		{ID: "1", Name: "Bash", Input: "{}"},
	}
	checker := &SimplePermissionChecker{Mode: ModePlan}
	results := reg.ExecuteBatch(context.Background(), calls, checker)

	if !results[0].Result.IsError {
		t.Error("expected plan mode to block write tool")
	}
}
```

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/tool/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/tool/executor.go internal/tool/permission.go internal/tool/executor_test.go
git commit -m "feat: Tool 并发执行器 + 五档权限系统"
```

---

### Task 8: Agent 循环引擎

**Files:**
- Create: `agent.go`
- Create: `internal/context/prompt.go`
- Create: `internal/context/project.go`

- [ ] **Step 1: 创建 System Prompt 组装**

```go
// internal/context/prompt.go
package context

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/xincode-ai/xin-code/internal/provider"
)

// BuildSystemPrompt 组装系统提示词
func BuildSystemPrompt(tools []provider.ToolDef, projectInstructions string) string {
	var sb strings.Builder

	// 1. 基础身份
	sb.WriteString("你是 Xin Code，一个 AI 驱动的终端编程助手。你可以读写文件、执行 shell 命令、搜索代码来帮助用户完成编程任务。\n\n")

	// 2. 工具说明
	sb.WriteString("# 可用工具\n\n")
	sb.WriteString("你可以使用以下工具：\n")
	for _, t := range tools {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", t.Name, t.Description))
	}
	sb.WriteString("\n")

	// 3. 行为规范
	sb.WriteString("# 行为规范\n\n")
	sb.WriteString("- 直接给出结论，不要冗余解释\n")
	sb.WriteString("- 修改文件前先读取文件内容\n")
	sb.WriteString("- 不要把 API Key、密码等写入代码\n")
	sb.WriteString("- 使用工具时优先用专用工具（Read 而非 cat，Glob 而非 find）\n\n")

	// 4. 项目指令（XINCODE.md）
	if projectInstructions != "" {
		sb.WriteString("# 项目指令\n\n")
		sb.WriteString(projectInstructions)
		sb.WriteString("\n\n")
	}

	// 5. 环境信息
	sb.WriteString("# 环境信息\n\n")
	cwd, _ := os.Getwd()
	sb.WriteString(fmt.Sprintf("- 工作目录: %s\n", cwd))
	sb.WriteString(fmt.Sprintf("- 操作系统: %s/%s\n", runtime.GOOS, runtime.GOARCH))
	sb.WriteString(fmt.Sprintf("- 当前日期: %s\n", time.Now().Format("2006-01-02")))

	return sb.String()
}
```

- [ ] **Step 2: 创建项目指令加载**

```go
// internal/context/project.go
package context

import "os"

// LoadProjectInstructions 读取 XINCODE.md
func LoadProjectInstructions() string {
	data, err := os.ReadFile("XINCODE.md")
	if err != nil {
		return ""
	}
	return string(data)
}
```

- [ ] **Step 3: 实现 Agent 循环引擎**

```go
// agent.go
package main

import (
	"context"
	"fmt"
	"io"

	xcontext "github.com/xincode-ai/xin-code/internal/context"
	"github.com/xincode-ai/xin-code/internal/provider"
	"github.com/xincode-ai/xin-code/internal/tool"
)

// Agent 核心引擎
type Agent struct {
	provider   provider.Provider
	tools      *tool.Registry
	permission tool.PermissionChecker
	config     *Config
	messages   []provider.Message
	output     io.Writer // 输出目标（Phase 1: os.Stdout, Phase 2: TUI）
}

// NewAgent 创建 Agent 实例
func NewAgent(p provider.Provider, tools *tool.Registry, cfg *Config, w io.Writer) *Agent {
	return &Agent{
		provider:   p,
		tools:      tools,
		permission: &tool.SimplePermissionChecker{Mode: tool.PermissionMode(cfg.Permission.Mode)},
		config:     cfg,
		messages:   make([]provider.Message, 0),
		output:     w,
	}
}

// Run 执行一轮 Agent 循环（用户消息 → API → 工具 → 循环）
func (a *Agent) Run(ctx context.Context, userMessage string) error {
	// 追加用户消息
	a.messages = append(a.messages, provider.NewTextMessage(provider.RoleUser, userMessage))

	// 组装 system prompt
	projectInstructions := xcontext.LoadProjectInstructions()
	systemPrompt := xcontext.BuildSystemPrompt(a.tools.ToolDefs(), projectInstructions)

	turns := 0
	for {
		turns++
		if turns > a.config.MaxTurns {
			fmt.Fprintln(a.output, "\n[达到最大轮次限制]")
			break
		}

		// 构建请求
		req := &provider.Request{
			Model:     a.config.Model,
			System:    systemPrompt,
			Messages:  a.messages,
			Tools:     a.tools.ToolDefs(),
			MaxTokens: a.config.MaxTokens,
		}

		// 流式调用 API
		events, err := a.provider.Stream(ctx, req)
		if err != nil {
			return fmt.Errorf("API error: %w", err)
		}

		// 处理流式事件
		assistantMsg, toolCalls, err := a.processStream(events)
		if err != nil {
			return err
		}

		// 追加 assistant 消息到历史
		a.messages = append(a.messages, assistantMsg)

		// 没有工具调用 → 本轮结束
		if len(toolCalls) == 0 {
			break
		}

		// 执行工具
		results := a.tools.ExecuteBatch(ctx, toolCalls, a.permission)

		// 工具结果追加到消息历史
		for _, r := range results {
			a.messages = append(a.messages,
				provider.NewToolResultMessage(r.ToolUseID, r.Result.Content, r.Result.IsError))
		}
	}

	return nil
}

// processStream 处理流式事件，收集 assistant 消息和工具调用
func (a *Agent) processStream(events <-chan provider.Event) (provider.Message, []*provider.ToolCall, error) {
	var textContent string
	var toolCalls []*provider.ToolCall
	var blocks []provider.ContentBlock

	for evt := range events {
		switch evt.Type {
		case provider.EventTextDelta:
			fmt.Fprint(a.output, evt.Text)
			textContent += evt.Text

		case provider.EventThinking:
			if evt.Thinking != nil {
				fmt.Fprintf(a.output, "\n[thinking] %s\n", evt.Thinking.Text)
			}

		case provider.EventToolUse:
			if evt.ToolCall != nil {
				fmt.Fprintf(a.output, "\n⚙ %s\n", evt.ToolCall.Name)
				toolCalls = append(toolCalls, evt.ToolCall)
				blocks = append(blocks, provider.ContentBlock{
					Type:     provider.BlockToolUse,
					ToolCall: evt.ToolCall,
				})
			}

		case provider.EventUsage:
			// Phase 2: 更新 cost tracker + statusbar
			if evt.Usage != nil {
				fmt.Fprintf(a.output, "\n[tokens: in=%d out=%d]\n",
					evt.Usage.InputTokens, evt.Usage.OutputTokens)
			}

		case provider.EventError:
			return provider.Message{}, nil, fmt.Errorf("stream error: %w", evt.Error)

		case provider.EventDone:
			// 流结束
		}
	}

	// 构建 assistant 消息
	if textContent != "" {
		blocks = append([]provider.ContentBlock{
			{Type: provider.BlockText, Text: textContent},
		}, blocks...)
	}

	msg := provider.Message{
		Role:    provider.RoleAssistant,
		Content: blocks,
	}

	fmt.Fprintln(a.output) // 换行
	return msg, toolCalls, nil
}
```

- [ ] **Step 4: 提交**

```bash
git add agent.go internal/context/
git commit -m "feat: Agent 循环引擎 + System Prompt 组装"
```

---

### Task 9: REPL 集成

**Files:**
- Modify: `main.go`

- [ ] **Step 1: 更新 main.go 为完整 REPL**

```go
// main.go
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/xincode-ai/xin-code/internal/provider"
	"github.com/xincode-ai/xin-code/internal/tool"
	"github.com/xincode-ai/xin-code/internal/tool/builtin"
)

func main() {
	// --version
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("xin-code %s (%s %s)\n", Version, Commit, Date)
		os.Exit(0)
	}

	// 加载配置
	cfg := LoadConfig()

	// 检查 API Key
	if cfg.APIKey == "" {
		fmt.Fprintln(os.Stderr, "错误: 未找到 API Key")
		fmt.Fprintln(os.Stderr, "请设置环境变量: export ANTHROPIC_API_KEY=your-key")
		fmt.Fprintln(os.Stderr, "或: export XINCODE_API_KEY=your-key")
		os.Exit(1)
	}

	// 创建 Provider
	providerName := cfg.Provider
	if providerName == "" {
		providerName = provider.ResolveProviderName(cfg.Model)
	}
	p, err := provider.NewProvider(providerName, cfg.APIKey, cfg.Model, cfg.BaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Provider 初始化失败: %s\n", err)
		os.Exit(1)
	}

	// 注册工具
	tools := tool.NewRegistry()
	builtin.RegisterAll(tools)

	// 创建 Agent
	agent := NewAgent(p, tools, cfg, os.Stdout)

	// 信号处理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// 打印欢迎信息
	fmt.Printf("XIN CODE %s\n", Version)
	fmt.Printf("model: %s  provider: %s\n", cfg.Model, p.Name())
	fmt.Printf("tools: %d  mode: %s\n", len(tools.All()), cfg.Permission.Mode)
	fmt.Println("---")
	fmt.Println("输入消息开始对话。/help 查看命令，/quit 退出。")
	fmt.Println()

	// REPL 循环
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// 斜杠命令
		if strings.HasPrefix(input, "/") {
			switch input {
			case "/quit", "/exit":
				fmt.Println("再见！")
				return
			case "/help":
				fmt.Println("可用命令:")
				fmt.Println("  /help    - 帮助信息")
				fmt.Println("  /model   - 显示当前模型")
				fmt.Println("  /version - 版本信息")
				fmt.Println("  /quit    - 退出")
			case "/model":
				fmt.Printf("当前模型: %s (%s)\n", cfg.Model, p.Name())
			case "/version":
				fmt.Printf("xin-code %s (%s %s)\n", Version, Commit, Date)
			default:
				fmt.Printf("未知命令: %s\n", input)
			}
			continue
		}

		// Shell 命令
		if strings.HasPrefix(input, "!") {
			cmd := strings.TrimPrefix(input, "!")
			result := tools.ExecuteTool(ctx, &provider.ToolCall{
				ID: "shell", Name: "Bash", Input: fmt.Sprintf(`{"command":%q}`, cmd),
			})
			fmt.Println(result.Content)
			continue
		}

		// Agent 对话
		if err := agent.Run(ctx, input); err != nil {
			fmt.Fprintf(os.Stderr, "错误: %s\n", err)
		}
	}
}
```

- [ ] **Step 2: 验证编译和基本功能**

Run: `go build -o xin-code . && echo "build success"`
Expected: `build success`

- [ ] **Step 3: 手动测试（需要 API Key）**

```bash
export ANTHROPIC_API_KEY=your-key
./xin-code
# 输入: /help
# 输入: /version
# 输入: 读一下当前目录有哪些文件
# 输入: /quit
```

- [ ] **Step 4: 提交**

```bash
git add main.go
git commit -m "feat: REPL 交互循环 — 完成 Phase 1 可运行版本"
```

---

### Task 10: 构建系统 + CI 基础

**Files:**
- Create: `Makefile`
- Create: `.github/workflows/build.yml`
- Create: `.golangci.yml`

- [ ] **Step 1: 创建 Makefile**

```makefile
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE     := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w \
  -X main.Version=$(VERSION) \
  -X main.Commit=$(COMMIT) \
  -X main.Date=$(DATE)

.PHONY: build test lint clean

build:
	go build -ldflags "$(LDFLAGS)" -o xin-code .

test:
	go test -v -cover -race -timeout=60s ./...

lint:
	golangci-lint run

clean:
	rm -f xin-code

install: build
	cp xin-code $(GOPATH)/bin/xin-code
```

- [ ] **Step 2: 创建 golangci-lint 配置**

```yaml
# .golangci.yml
version: "2"
linters:
  enable:
    - bodyclose
    - copyloopvar
    - durationcheck
    - exhaustive
    - gocritic
    - gofumpt
    - goimports
    - govet
    - ineffassign
    - misspell
    - noctx
    - unused

linters-settings:
  exhaustive:
    default-signifies-exhaustive: true
  gocritic:
    enabled-tags:
      - diagnostic
      - style
```

- [ ] **Step 3: 创建 GitHub Actions CI**

```yaml
# .github/workflows/build.yml
name: Build

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  build:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - run: go mod download
      - run: go build -v ./...
      - run: go test -v -cover -race -timeout=60s ./...
```

- [ ] **Step 4: 运行本地验证**

Run: `make build && make test`
Expected: build success, all tests PASS

- [ ] **Step 5: 提交**

```bash
git add Makefile .golangci.yml .github/
git commit -m "feat: 构建系统 + CI 配置"
```

---

## Phase 1 完成标志

Phase 1 完成后，你有一个可运行的 CLI Agent：

```bash
$ make build
$ ./xin-code
XIN CODE dev
model: claude-sonnet-4-6-20250514  provider: anthropic
tools: 4  mode: default
---
输入消息开始对话。/help 查看命令，/quit 退出。

> 读一下 main.go 的前 10 行
⚙ Read
1  package main
2
3  import (
...

> /quit
再见！
```

## 后续 Phase 概览

| Phase | 范围 | 关键任务 |
|-------|------|---------|
| **Phase 2** | TUI + 剩余工具 | Bubbletea 主程序、Chat/Input/StatusBar 组件、Write/Edit（Diff 预览）/WebFetch/WebSearch/AskUser/Task 工具、费用追踪、权限对话框 |
| **Phase 3** | 会话 + MCP + 命令 | 会话持久化 + resume、Auto Compact、MCP 客户端（stdio/SSE/HTTP）、36 个斜杠命令完整实现 |
| **Phase 4** | 扩展 + 发布 | OpenAI Provider、CC OAuth 复用、Skills/Plugins/Hooks、GoReleaser + Homebrew Tap、README + CONTRIBUTING |

每个 Phase 将在前一个 Phase 完成后生成独立的实施计划。
