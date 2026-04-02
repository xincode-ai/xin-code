package session

import (
	"strings"
	"testing"

	"github.com/xincode-ai/xin-code/internal/provider"
)

func TestNewSession(t *testing.T) {
	sess := NewSession("claude-sonnet-4-6", "/tmp/test")

	if sess.ID == "" {
		t.Error("会话 ID 不应为空")
	}
	if sess.Model != "claude-sonnet-4-6" {
		t.Errorf("模型不匹配: got %s", sess.Model)
	}
	if sess.WorkDir != "/tmp/test" {
		t.Errorf("工作目录不匹配: got %s", sess.WorkDir)
	}
	if sess.Turns != 0 {
		t.Errorf("初始轮次应为 0: got %d", sess.Turns)
	}
}

func TestAddMessage(t *testing.T) {
	sess := NewSession("test-model", "/tmp")

	// 用户消息增加轮次
	sess.AddMessage(provider.NewTextMessage(provider.RoleUser, "hello"))
	if sess.Turns != 1 {
		t.Errorf("用户消息后轮次应为 1: got %d", sess.Turns)
	}
	if len(sess.Messages) != 1 {
		t.Errorf("消息数应为 1: got %d", len(sess.Messages))
	}

	// assistant 消息不增加轮次
	sess.AddMessage(provider.NewTextMessage(provider.RoleAssistant, "hi"))
	if sess.Turns != 1 {
		t.Errorf("assistant 消息后轮次仍应为 1: got %d", sess.Turns)
	}
	if len(sess.Messages) != 2 {
		t.Errorf("消息数应为 2: got %d", len(sess.Messages))
	}
}

func TestExportMarkdown(t *testing.T) {
	sess := NewSession("test-model", "/tmp")
	sess.AddMessage(provider.NewTextMessage(provider.RoleUser, "写个 hello world"))
	sess.AddMessage(provider.NewTextMessage(provider.RoleAssistant, "好的，给你一个 Go 的 hello world"))

	md := sess.ExportMarkdown()

	if !strings.Contains(md, "Session:") {
		t.Error("导出应包含 Session 标题")
	}
	if !strings.Contains(md, "写个 hello world") {
		t.Error("导出应包含用户消息")
	}
	if !strings.Contains(md, "hello world") {
		t.Error("导出应包含 assistant 消息")
	}
}

func TestUpdateCost(t *testing.T) {
	sess := NewSession("test-model", "/tmp")
	sess.UpdateCost(100, 50, 0.001)
	sess.UpdateCost(200, 100, 0.002)

	if sess.TotalInputTokens != 300 {
		t.Errorf("输入 token 应为 300: got %d", sess.TotalInputTokens)
	}
	if sess.TotalOutputTokens != 150 {
		t.Errorf("输出 token 应为 150: got %d", sess.TotalOutputTokens)
	}
	if sess.TotalCostUSD < 0.002 {
		t.Errorf("费用应 >= 0.002: got %f", sess.TotalCostUSD)
	}
}
