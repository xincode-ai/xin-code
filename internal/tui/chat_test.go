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

func TestLineToMsgMapping(t *testing.T) {
	cv := NewChatView(80, 24)

	cv.messages = append(cv.messages,
		ChatMessage{ID: "msg-1", Role: "user", Content: "hello"},
		ChatMessage{ID: "msg-2", Role: "thinking", Content: "thinking...", Folded: true},
		ChatMessage{ID: "msg-3", Role: "tool", ToolName: "Bash", Content: "output"},
	)

	cv.invalidateCache()
	cv.refreshContent(true)

	// lineToMsg 应非空
	if len(cv.lineToMsg) == 0 {
		t.Fatal("lineToMsg 不应为空")
	}

	// 第一行应对应消息 0（user）
	if cv.lineToMsg[0] != 0 {
		t.Errorf("第一行应对应消息 0，得到 %d", cv.lineToMsg[0])
	}

	// 映射中应包含所有 3 条消息的索引
	found := map[int]bool{}
	for _, idx := range cv.lineToMsg {
		if idx >= 0 {
			found[idx] = true
		}
	}
	for i := 0; i < 3; i++ {
		if !found[i] {
			t.Errorf("消息 %d 未在 lineToMsg 中出现", i)
		}
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
