package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// 折叠阈值：超过此行数的工具输出自动折叠
const foldThreshold = 8

// ChatMessage 单条消息
type ChatMessage struct {
	Role      string // "user" / "assistant" / "tool" / "thinking" / "error" / "system"
	Content   string
	ToolName  string // 工具消息时使用
	ToolID    string // 工具调用 ID（用于精确匹配同名工具）
	ToolInput string // 工具输入参数 (raw JSON)
	IsError   bool
	Folded    bool // 是否已折叠
}

// ChatView 对话区域组件
type ChatView struct {
	viewport viewport.Model
	messages []ChatMessage
	width    int
	height   int

	// 流式状态
	streaming      bool
	streamBuf      string
	streamToolName string // 当前正在执行的工具名

	// Glamour 渲染器
	renderer *glamour.TermRenderer
}

// NewChatView 创建对话区域
func NewChatView(width, height int) ChatView {
	vp := viewport.New(width, height)
	vp.Style = lipgloss.NewStyle()

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(width-4),
	)

	return ChatView{
		viewport: vp,
		width:    width,
		height:   height,
		renderer: renderer,
	}
}

func (c ChatView) Init() tea.Cmd { return nil }

func (c ChatView) Update(msg tea.Msg) (ChatView, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
		c.viewport.Width = msg.Width
		c.viewport.Height = msg.Height
		c.renderer, _ = glamour.NewTermRenderer(
			glamour.WithStylePath("dark"),
			glamour.WithWordWrap(msg.Width-4),
		)
		c.refreshContent()

	case MsgTextDelta:
		c.streaming = true
		c.streamBuf += msg.Text
		c.refreshContent()
		c.viewport.GotoBottom()

	case MsgThinking:
		// 合并连续的 thinking 消息为一条，避免冗长显示
		if len(c.messages) > 0 && c.messages[len(c.messages)-1].Role == "thinking" {
			c.messages[len(c.messages)-1].Content += msg.Text
		} else {
			c.messages = append(c.messages, ChatMessage{
				Role:    "thinking",
				Content: msg.Text,
			})
		}
		c.refreshContent()
		c.viewport.GotoBottom()

	case MsgToolStart:
		c.streamToolName = msg.Name
		c.messages = append(c.messages, ChatMessage{
			Role:      "tool",
			ToolName:  msg.Name,
			ToolID:    msg.ID,
			ToolInput: msg.Input,
			Content:   "执行中...",
		})
		c.refreshContent()
		c.viewport.GotoBottom()

	case MsgToolDone:
		c.streamToolName = ""
		for i := len(c.messages) - 1; i >= 0; i-- {
			if c.messages[i].Role == "tool" && c.messages[i].ToolID == msg.ID {
				c.messages[i].Content = msg.Output
				c.messages[i].IsError = msg.IsError
				if lines := strings.Count(msg.Output, "\n"); lines > foldThreshold {
					c.messages[i].Folded = true
				}
				break
			}
		}
		c.refreshContent()
		c.viewport.GotoBottom()

	case MsgResponseDone:
		if c.streamBuf != "" {
			c.messages = append(c.messages, ChatMessage{
				Role:    "assistant",
				Content: c.streamBuf,
			})
			c.streamBuf = ""
			c.streaming = false
			c.refreshContent()
			c.viewport.GotoBottom()
		}

	case MsgSubmit:
		c.messages = append(c.messages, ChatMessage{
			Role:    "user",
			Content: msg.Text,
		})
		c.streamBuf = ""
		c.streaming = false
		c.refreshContent()
		c.viewport.GotoBottom()

	case MsgError:
		c.messages = append(c.messages, ChatMessage{
			Role:    "error",
			Content: msg.Err.Error(),
		})
		c.refreshContent()
		c.viewport.GotoBottom()
	}

	c.viewport, cmd = c.viewport.Update(msg)
	return c, cmd
}

func (c ChatView) View() string {
	return c.viewport.View()
}

// refreshContent 重新渲染所有消息到 viewport
func (c *ChatView) refreshContent() {
	var sb strings.Builder

	for i, msg := range c.messages {
		// 用户消息和 assistant 消息前增加空行，提供视觉层次
		if i > 0 && (msg.Role == "user" || msg.Role == "assistant") {
			sb.WriteString("\n")
		}
		rendered := c.renderMessage(msg)
		if rendered != "" {
			sb.WriteString(rendered)
			sb.WriteString("\n")
		}
	}

	// 流式接收中的 AI 回复（CC 风格：● 前缀 + 光标）
	if c.streaming && c.streamBuf != "" {
		sb.WriteString("\n")
		sb.WriteString(StyleToolRunning.Render("● "))
		sb.WriteString(c.streamBuf)
		sb.WriteString(StyleHint.Render(" ▊"))
		sb.WriteString("\n")
	}

	c.viewport.SetContent(sb.String())
}

func (c *ChatView) renderMessage(msg ChatMessage) string {
	switch msg.Role {
	case "user":
		prefix := StyleUserPrefix.Render("❯ ")
		content := StyleUserMsg.Render(msg.Content)
		return prefix + content

	case "assistant":
		// CC 风格：● 前缀标记 AI 响应块
		prefix := lipgloss.NewStyle().Foreground(ColorText).Render("● ")
		if c.renderer != nil {
			rendered, err := c.renderer.Render(msg.Content)
			if err == nil {
				return prefix + strings.TrimSpace(rendered)
			}
		}
		return prefix + StyleAIMsg.Render(msg.Content)

	case "thinking":
		// 极简显示，仅一行提示（对标 CC 的 thinking 指示器）
		return StyleThinking.Render("  ∴ thinking")

	case "tool":
		return c.renderToolMessage(msg)

	case "system":
		// 系统消息：已用 Lipgloss 预渲染，不经过 Glamour
		return msg.Content

	case "error":
		return StyleErrorMsg.Render("  ✗ " + msg.Content)

	default:
		return msg.Content
	}
}

func (c *ChatView) renderToolMessage(msg ChatMessage) string {
	// 解析工具参数预览
	argPreview := toolArgPreview(msg.ToolName, msg.ToolInput)

	// 执行中：蓝色 ⏺ + bold 工具名 + dim 参数
	if msg.Content == "执行中..." {
		header := StyleToolRunning.Render("  ⏺ ") + StyleToolName.Render(msg.ToolName)
		if argPreview != "" {
			header += StyleHint.Render("(" + argPreview + ")")
		}
		return header
	}

	// 完成状态：绿/红图标 + bold 工具名 + dim 参数
	var icon string
	var iconStyle lipgloss.Style
	if msg.IsError {
		icon = "✗"
		iconStyle = StyleToolError
	} else {
		icon = "✓"
		iconStyle = StyleToolSuccess
	}

	header := iconStyle.Render("  "+icon+" ") + StyleToolName.Render(msg.ToolName)
	if argPreview != "" {
		header += StyleHint.Render("(" + argPreview + ")")
	}

	// 无输出
	if msg.Content == "" {
		return header
	}

	lines := strings.Split(msg.Content, "\n")
	lineCount := len(lines)

	// 折叠显示
	if msg.Folded || lineCount > foldThreshold {
		previewEnd := 3
		if previewEnd > lineCount {
			previewEnd = lineCount
		}
		outputLines := formatToolOutput(lines[:previewEnd])
		output := StyleToolOutput.Render(strings.Join(outputLines, "\n"))
		remaining := lineCount - previewEnd
		if remaining > 0 {
			hint := StyleHint.Render(fmt.Sprintf("      … +%d lines (已折叠)", remaining))
			return header + "\n" + output + "\n" + hint
		}
		return header + "\n" + output
	}

	// 完整显示（短输出）
	outputLines := formatToolOutput(lines)
	return header + "\n" + StyleToolOutput.Render(strings.Join(outputLines, "\n"))
}

// formatToolOutput 格式化工具输出行（缩进对齐，CC 风格）
func formatToolOutput(lines []string) []string {
	result := make([]string, 0, len(lines))
	for i, line := range lines {
		if i == 0 {
			result = append(result, "    ⎿ "+line)
		} else {
			result = append(result, "      "+line)
		}
	}
	return result
}

// toolArgPreview 从工具输入 JSON 中提取关键参数作为预览
func toolArgPreview(name, rawInput string) string {
	if rawInput == "" {
		return ""
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(rawInput), &m); err != nil {
		// 非 JSON，直接截断返回
		if len(rawInput) > 50 {
			return rawInput[:47] + "..."
		}
		return rawInput
	}

	// 根据工具名选择最有代表性的参数
	keyArgs := map[string]string{
		"Bash":      "command",
		"Read":      "path",
		"Write":     "path",
		"Edit":      "path",
		"Glob":      "pattern",
		"Grep":      "pattern",
		"WebFetch":  "url",
		"WebSearch": "query",
		"AskUser":   "question",
	}

	if key, ok := keyArgs[name]; ok {
		if val, ok := m[key].(string); ok && val != "" {
			if len(val) > 60 {
				return val[:57] + "..."
			}
			return val
		}
	}

	// 兜底：取第一个字符串值
	for _, v := range m {
		if s, ok := v.(string); ok && s != "" {
			if len(s) > 60 {
				return s[:57] + "..."
			}
			return s
		}
	}

	return ""
}

// AddSystemMessage 添加系统消息（已预渲染的 ANSI 文本，不经过 Glamour）
func (c *ChatView) AddSystemMessage(text string) {
	c.messages = append(c.messages, ChatMessage{
		Role:    "system",
		Content: text,
	})
	c.refreshContent()
	c.viewport.GotoBottom()
}

// Clear 清空消息
func (c *ChatView) Clear() {
	c.messages = nil
	c.streamBuf = ""
	c.streaming = false
	c.refreshContent()
}
