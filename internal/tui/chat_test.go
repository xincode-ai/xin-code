package tui

import (
	"strings"
	"testing"
)

func TestToolOutputFoldExpand(t *testing.T) {
	cv := NewChatView(80, 24)

	longOutput := strings.Repeat("line\n", 20)
	cv.messages = append(cv.messages, ChatMessage{
		ID:       "msg-1",
		Role:     "tool",
		ToolName: "Bash",
		Content:  longOutput,
	})

	// 默认：超阈值自动折叠
	cv.invalidateCache()
	cv.refreshContent(true)
	rendered := cv.viewport.View()
	if !strings.Contains(rendered, "+") || !strings.Contains(rendered, "行") {
		t.Error("超阈值输出应显示折叠提示 [+N 行]")
	}

	// 切换到展开模式
	cv.SetToolOutputExpanded(true)
	cv.invalidateCache()
	cv.refreshContent(true)
	rendered = cv.viewport.View()
	if strings.Contains(rendered, "[+") && strings.Contains(rendered, "行]") {
		t.Error("展开模式不应显示折叠提示")
	}

	// 切回折叠
	cv.SetToolOutputExpanded(false)
	cv.invalidateCache()
	cv.refreshContent(true)
	rendered = cv.viewport.View()
	if !strings.Contains(rendered, "+") {
		t.Error("折叠模式应恢复折叠提示")
	}
}

func TestClickToggleToolFold(t *testing.T) {
	cv := NewChatView(80, 24)

	longOutput := strings.Repeat("line\n", 20)
	cv.messages = append(cv.messages, ChatMessage{
		ID:       "msg-1",
		Role:     "tool",
		ToolName: "Bash",
		Content:  longOutput,
	})

	// 默认自动折叠
	cv.invalidateCache()
	cv.refreshContent(true)
	rendered := cv.viewport.View()
	if !strings.Contains(rendered, "[+") {
		t.Fatal("初始应折叠")
	}

	// 模拟点击 toggle：手动设置 Folded（等价于鼠标点击 handler 的行为）
	cv.messages[0].Folded = true // XOR: autoFold=true, Folded=true → 不折叠
	cv.invalidateCache()
	cv.refreshContent(true)
	rendered = cv.viewport.View()
	if strings.Contains(rendered, "[+") && strings.Contains(rendered, "行]") {
		t.Error("点击后应展开，不应有折叠提示")
	}

	// 再次 toggle 回折叠
	cv.messages[0].Folded = false // XOR: autoFold=true, Folded=false → 折叠
	cv.invalidateCache()
	cv.refreshContent(true)
	rendered = cv.viewport.View()
	if !strings.Contains(rendered, "[+") {
		t.Error("再次点击应恢复折叠")
	}
}

func TestClickToggleThinkingFold(t *testing.T) {
	cv := NewChatView(80, 24)

	cv.messages = append(cv.messages, ChatMessage{
		ID:      "msg-1",
		Role:    "thinking",
		Content: "This is a long thinking process with lots of detail...",
		Folded:  true, // 默认折叠
	})

	// 折叠态不显示完整内容
	cv.invalidateCache()
	cv.refreshContent(true)
	rendered := cv.viewport.View()
	if strings.Contains(rendered, "lots of detail") {
		t.Error("折叠态不应显示完整内容")
	}

	// 点击展开
	cv.messages[0].Folded = false
	cv.invalidateCache()
	cv.refreshContent(true)
	rendered = cv.viewport.View()
	if !strings.Contains(rendered, "lots of detail") {
		t.Error("展开态应显示完整内容")
	}
}

func TestToggleLinesMapping(t *testing.T) {
	cv := NewChatView(80, 24)

	cv.messages = append(cv.messages,
		ChatMessage{ID: "msg-1", Role: "user", Content: "hello"},
		ChatMessage{ID: "msg-2", Role: "thinking", Content: "thinking...", Folded: true},
		ChatMessage{ID: "msg-3", Role: "tool", ToolName: "Bash", Content: "output line"},
	)

	cv.invalidateCache()
	cv.refreshContent(true)

	// toggleLines 应只包含可折叠消息（thinking=1, tool=2），不包含 user=0
	if len(cv.toggleLines) == 0 {
		t.Fatal("toggleLines 不应为空")
	}

	// user 消息不应出现在 toggleLines 中
	for _, idx := range cv.toggleLines {
		if idx == 0 {
			t.Error("user 消息不应在 toggleLines 中")
		}
	}

	// thinking 和 tool 消息应出现
	foundThinking := false
	foundTool := false
	for _, idx := range cv.toggleLines {
		if idx == 1 {
			foundThinking = true
		}
		if idx == 2 {
			foundTool = true
		}
	}
	if !foundThinking {
		t.Error("thinking 消息应在 toggleLines 中")
	}
	if !foundTool {
		t.Error("tool 消息应在 toggleLines 中")
	}
}

func TestToolOutputShortNoFold(t *testing.T) {
	cv := NewChatView(80, 24)

	cv.messages = append(cv.messages, ChatMessage{
		ID:       "msg-1",
		Role:     "tool",
		ToolName: "Read",
		Content:  "hello\nworld",
	})
	cv.invalidateCache()
	cv.refreshContent(true)
	rendered := cv.viewport.View()
	if strings.Contains(rendered, "[+") {
		t.Error("短输出不应折叠")
	}
}
