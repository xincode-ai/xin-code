package slash

import (
	"strings"
	"testing"
)

func TestHandleHelp(t *testing.T) {
	h := NewHandler()
	ctx := &Context{
		Model:   "test-model",
		Version: "0.1.0",
	}

	result, handled := h.Handle("/help", ctx)
	if !handled {
		t.Fatal("/help 应被处理")
	}
	if result.Type != ResultDisplay {
		t.Errorf("结果类型应为 display: got %s", result.Type)
	}
	if !strings.Contains(result.Content, "命令列表") {
		t.Error("帮助内容应包含命令列表")
	}
}

func TestHandleVersion(t *testing.T) {
	h := NewHandler()
	ctx := &Context{Version: "v0.1.0-test"}

	result, handled := h.Handle("/version", ctx)
	if !handled {
		t.Fatal("/version 应被处理")
	}
	if !strings.Contains(result.Content, "v0.1.0-test") {
		t.Error("版本信息应包含版本号")
	}
}

func TestHandleModel(t *testing.T) {
	h := NewHandler()
	ctx := &Context{Model: "claude-sonnet-4-6"}

	// 无参数：显示当前模型
	result, _ := h.Handle("/model", ctx)
	if !strings.Contains(result.Content, "claude-sonnet-4-6") {
		t.Error("应显示当前模型")
	}

	// 有参数：切换模型
	switched := false
	ctx.OnModelSwitch = func(m string) { switched = true }
	result, _ = h.Handle("/model gpt-4o", ctx)
	if !switched {
		t.Error("应调用 OnModelSwitch")
	}
	if !strings.Contains(result.Content, "gpt-4o") {
		t.Error("切换确认应包含新模型名")
	}
}

func TestHandleQuit(t *testing.T) {
	h := NewHandler()
	ctx := &Context{}

	result, handled := h.Handle("/quit", ctx)
	if !handled {
		t.Fatal("/quit 应被处理")
	}
	if result.Type != ResultAction {
		t.Errorf("结果类型应为 action: got %s", result.Type)
	}
	if result.Content != "quit" {
		t.Errorf("动作内容应为 quit: got %s", result.Content)
	}
}

func TestHandleExit(t *testing.T) {
	h := NewHandler()
	ctx := &Context{}

	result, handled := h.Handle("/exit", ctx)
	if !handled {
		t.Fatal("/exit 应被处理")
	}
	if result.Content != "quit" {
		t.Error("/exit 应触发 quit 动作")
	}
}

func TestHandleUnknown(t *testing.T) {
	h := NewHandler()
	ctx := &Context{}

	result, handled := h.Handle("/unknown", ctx)
	if !handled {
		t.Fatal("未知命令也应被处理（返回错误提示）")
	}
	if !strings.Contains(result.Content, "未知命令") {
		t.Error("应返回未知命令提示")
	}
}

func TestHandleNonSlash(t *testing.T) {
	h := NewHandler()
	ctx := &Context{}

	_, handled := h.Handle("hello", ctx)
	if handled {
		t.Fatal("非斜杠开头不应被处理")
	}
}

func TestHandleCommit(t *testing.T) {
	h := NewHandler()
	ctx := &Context{}

	result, handled := h.Handle("/commit", ctx)
	if !handled {
		t.Fatal("/commit 应被处理")
	}
	if result.Type != ResultPrompt {
		t.Errorf("结果类型应为 prompt: got %s", result.Type)
	}
	if result.Content == "" {
		t.Error("prompt 内容不应为空")
	}
}

func TestHandleCost(t *testing.T) {
	h := NewHandler()
	ctx := &Context{
		InputTokens:         1000,
		OutputTokens:        500,
		TotalTokens:         1500,
		CostString:          "¥0.0100",
		CacheCreationTokens: 200,
		CacheReadTokens:     800,
	}

	result, handled := h.Handle("/cost", ctx)
	if !handled {
		t.Fatal("/cost 应被处理")
	}
	if !strings.Contains(result.Content, "1000") {
		t.Error("应包含输入 token 数")
	}
	if !strings.Contains(result.Content, "缓存命中率") {
		t.Error("应包含缓存命中率")
	}
}

func TestHandleContext(t *testing.T) {
	h := NewHandler()
	ctx := &Context{
		TotalTokens: 160000,
		MaxContext:   200000,
	}

	result, _ := h.Handle("/context", ctx)
	if !strings.Contains(result.Content, "80.0%") {
		t.Error("应显示 80% 使用率")
	}
}

func TestHandleClear(t *testing.T) {
	h := NewHandler()
	cleared := false
	ctx := &Context{
		OnClear: func() { cleared = true },
	}

	result, _ := h.Handle("/clear", ctx)
	if !cleared {
		t.Error("应调用 OnClear")
	}
	if result.Type != ResultAction {
		t.Errorf("结果类型应为 action: got %s", result.Type)
	}
}

func TestAllCommands(t *testing.T) {
	h := NewHandler()
	cmds := h.AllCommands()

	// 应至少有 25 个命令
	if len(cmds) < 25 {
		t.Errorf("命令数量应 >= 25: got %d", len(cmds))
	}

	// 检查排序
	for i := 1; i < len(cmds); i++ {
		if cmds[i].Name < cmds[i-1].Name {
			t.Errorf("命令未按名称排序: %s < %s", cmds[i].Name, cmds[i-1].Name)
		}
	}
}

func TestCommandNames(t *testing.T) {
	h := NewHandler()
	names := h.CommandNames()

	if len(names) == 0 {
		t.Fatal("命令名列表不应为空")
	}

	// 检查关键命令存在
	required := []string{"/help", "/quit", "/exit", "/model", "/commit", "/cost", "/context"}
	for _, req := range required {
		found := false
		for _, name := range names {
			if name == req {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("缺少必要命令: %s", req)
		}
	}
}
