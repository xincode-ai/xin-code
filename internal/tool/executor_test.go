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
func (t *echoTool) Execute(_ context.Context, _ json.RawMessage) (*Result, error) {
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
