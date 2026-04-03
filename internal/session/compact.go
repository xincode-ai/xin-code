package session

import (
	"fmt"
	"strings"

	"github.com/xincode-ai/xin-code/internal/provider"
)

// 压缩相关常量（对标 CC src/services/compact/autoCompact.ts）
const (
	AutoCompactBuffer    = 13000 // 自动压缩缓冲 token（CC: AUTOCOMPACT_BUFFER_TOKENS）
	SummaryReserve       = 20000 // 摘要生成保留的输出 token（CC: MAX_OUTPUT_TOKENS_FOR_SUMMARY）
	WarningBuffer        = 20000 // 警告阈值缓冲（CC: WARNING_THRESHOLD_BUFFER_TOKENS）
	MicroCompactSize     = 50000 // 单个工具输出超过 50KB 时微压缩
	MicroCompactHead     = 10240 // 微压缩保留头部 10KB
	MicroCompactTail     = 5120  // 微压缩保留尾部 5KB
	KeepRecentTurns      = 5     // 压缩时保留最近 5 轮
)

// CompactResult 压缩结果
type CompactResult struct {
	BeforeTokens int
	AfterTokens  int
	Compacted    bool
	Message      string
}

// lastUserMsgIdx 返回最后一条用户消息的索引
func lastUserMsgIdx(messages []provider.Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == provider.RoleUser {
			return i
		}
	}
	return -1
}

// isCompactSummaryMessage 检查消息是否是压缩摘要（用于跳过）
func isCompactSummaryMessage(msg provider.Message) bool {
	if msg.Role != provider.RoleUser {
		return false
	}
	text := msg.TextContent()
	return strings.HasPrefix(text, "[上下文摘要]\n")
}

// NeedsCompact 检查是否需要自动压缩
// CC 公式：threshold = (maxContext - SummaryReserve) - AutoCompactBuffer
// 例：200K context → threshold = (200000 - 20000) - 13000 = 167000
func NeedsCompact(totalTokens, maxContext int) bool {
	if maxContext <= 0 {
		return false
	}
	effectiveWindow := maxContext - SummaryReserve
	threshold := effectiveWindow - AutoCompactBuffer
	return totalTokens >= threshold
}

// CompactMessages 压缩消息历史
// 策略：保留最近 KeepRecentTurns 轮对话，中间消息生成摘要
func CompactMessages(messages []provider.Message) ([]provider.Message, string) {
	// 找出最后一条用户消息的位置
	lastUser := lastUserMsgIdx(messages)
	if lastUser < 0 {
		return messages, "无用户消息，无需压缩"
	}

	// 找到最近 KeepRecentTurns 轮对话的起始位置
	keepFrom := findKeepBoundary(messages, KeepRecentTurns)

	// 如果保留部分太少，说明消息量小，无需压缩
	if keepFrom >= lastUser {
		return messages, "消息数量较少，无需压缩"
	}

	// 收集需要摘要的消息（跳过压缩摘要消息避免循环）
	var oldMessages []provider.Message
	for i := 0; i < keepFrom; i++ {
		if !isCompactSummaryMessage(messages[i]) {
			oldMessages = append(oldMessages, messages[i])
		}
	}

	// 如果没有实际内容需要压缩
	if len(oldMessages) == 0 {
		return messages, "无历史消息可压缩"
	}

	// 为前面的消息生成摘要
	summary := generateSummary(oldMessages)

	// 构建压缩后的消息列表
	var compacted []provider.Message

	// 摘要作为第一条用户消息
	compacted = append(compacted, provider.NewTextMessage(
		provider.RoleUser,
		"[上下文摘要]\n"+summary,
	))
	// 添加 assistant 的确认
	compacted = append(compacted, provider.NewTextMessage(
		provider.RoleAssistant,
		"已加载上下文摘要，继续当前任务。",
	))

	// 保留最近的消息（包括最后一条用户消息）
	compacted = append(compacted, messages[keepFrom:]...)

	return compacted, fmt.Sprintf("已压缩 %d 条消息为摘要，保留最近 %d 轮", len(oldMessages), KeepRecentTurns)
}

// MicroCompact 微压缩：截断过大的工具输出
func MicroCompact(content string) string {
	if len(content) <= MicroCompactSize {
		return content
	}

	head := content[:MicroCompactHead]
	tail := content[len(content)-MicroCompactTail:]
	omitted := len(content) - MicroCompactHead - MicroCompactTail

	return fmt.Sprintf("%s\n\n... [已省略 %d 字节] ...\n\n%s", head, omitted, tail)
}

// findKeepBoundary 找到保留边界位置
func findKeepBoundary(messages []provider.Message, keepTurns int) int {
	userCount := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == provider.RoleUser {
			userCount++
			if userCount >= keepTurns {
				return i
			}
		}
	}
	return 0
}

// generateSummary 为旧消息生成摘要
func generateSummary(messages []provider.Message) string {
	var sb strings.Builder

	sb.WriteString("之前的对话中：\n")

	turnNum := 0
	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleUser:
			turnNum++
			text := msg.TextContent()
			if len(text) > 200 {
				text = text[:200] + "..."
			}
			sb.WriteString(fmt.Sprintf("- 第 %d 轮用户请求: %s\n", turnNum, text))

		case provider.RoleAssistant:
			text := msg.TextContent()
			if text == "" {
				continue
			}
			if len(text) > 200 {
				text = text[:200] + "..."
			}
			sb.WriteString(fmt.Sprintf("  助手回复: %s\n", text))

			// 记录工具调用
			for _, call := range msg.ToolCalls() {
				sb.WriteString(fmt.Sprintf("  使用工具: %s\n", call.Name))
			}
		}
	}

	return sb.String()
}
