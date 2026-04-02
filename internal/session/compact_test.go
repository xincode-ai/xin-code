package session

import (
	"strings"
	"testing"

	"github.com/xincode-ai/xin-code/internal/provider"
)

func TestNeedsCompact(t *testing.T) {
	tests := []struct {
		name       string
		total      int
		max        int
		wantCompact bool
	}{
		{"低使用率", 50000, 200000, false},
		{"中使用率", 160000, 200000, false},
		{"高使用率-刚好阈值", 180000, 200000, true},
		{"高使用率-超过阈值", 190000, 200000, true},
		{"零最大值", 100000, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsCompact(tt.total, tt.max)
			if got != tt.wantCompact {
				t.Errorf("NeedsCompact(%d, %d) = %v, want %v", tt.total, tt.max, got, tt.wantCompact)
			}
		})
	}
}

func TestCompactMessages(t *testing.T) {
	// 构建 12 轮对话
	var messages []provider.Message
	for i := 0; i < 12; i++ {
		messages = append(messages, provider.NewTextMessage(provider.RoleUser, "用户消息"))
		messages = append(messages, provider.NewTextMessage(provider.RoleAssistant, "助手回复"))
	}

	compacted, msg := CompactMessages(messages)

	if len(compacted) >= len(messages) {
		t.Errorf("压缩后消息数 (%d) 应少于原始 (%d)", len(compacted), len(messages))
	}
	if msg == "" {
		t.Error("压缩消息不应为空")
	}
	// 检查包含摘要
	if compacted[0].TextContent() == "" {
		t.Error("压缩后第一条消息应包含摘要")
	}
}

func TestCompactMessagesShort(t *testing.T) {
	// 消息数量少于阈值时不压缩
	messages := []provider.Message{
		provider.NewTextMessage(provider.RoleUser, "hi"),
		provider.NewTextMessage(provider.RoleAssistant, "hello"),
	}

	compacted, _ := CompactMessages(messages)
	if len(compacted) != len(messages) {
		t.Errorf("短对话不应被压缩: got %d, want %d", len(compacted), len(messages))
	}
}

func TestMicroCompact(t *testing.T) {
	// 小内容不截断
	small := "hello world"
	if MicroCompact(small) != small {
		t.Error("小内容不应被截断")
	}

	// 大内容截断
	big := strings.Repeat("x", MicroCompactSize+1000)
	result := MicroCompact(big)
	if len(result) >= len(big) {
		t.Error("大内容应被截断")
	}
	if !strings.Contains(result, "已省略") {
		t.Error("截断结果应包含省略提示")
	}
}
