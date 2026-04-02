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
