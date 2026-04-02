package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// 折叠阈值
const foldThreshold = 20

// ChatMessage 单条消息
type ChatMessage struct {
	Role    string // "user" / "assistant" / "tool" / "thinking" / "error"
	Content string
	ToolName string // 工具消息时使用
	IsError  bool
	Folded   bool // 是否已折叠
}

// ChatView 对话区域组件
type ChatView struct {
	viewport viewport.Model
	messages []ChatMessage
	width    int
	height   int

	// 流式状态
	streaming    bool
	streamBuf    string
	streamToolName string // 当前正在执行的工具名

	// Glamour 渲染器
	renderer *glamour.TermRenderer
}

// NewChatView 创建对话区域
func NewChatView(width, height int) ChatView {
	vp := viewport.New(width, height)
	vp.Style = lipgloss.NewStyle()

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
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
		// 重新创建渲染器
		c.renderer, _ = glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(msg.Width-4),
		)
		c.refreshContent()

	case MsgTextDelta:
		c.streaming = true
		c.streamBuf += msg.Text
		c.refreshContent()
		c.viewport.GotoBottom()

	case MsgThinking:
		// 追加 thinking 消息
		c.messages = append(c.messages, ChatMessage{
			Role:    "thinking",
			Content: msg.Text,
		})
		c.refreshContent()
		c.viewport.GotoBottom()

	case MsgToolStart:
		c.streamToolName = msg.Name
		c.messages = append(c.messages, ChatMessage{
			Role:     "tool",
			ToolName: msg.Name,
			Content:  "执行中...",
		})
		c.refreshContent()
		c.viewport.GotoBottom()

	case MsgToolDone:
		c.streamToolName = ""
		// 更新最后一条工具消息
		for i := len(c.messages) - 1; i >= 0; i-- {
			if c.messages[i].Role == "tool" && c.messages[i].ToolName == msg.Name {
				c.messages[i].Content = msg.Output
				c.messages[i].IsError = msg.IsError
				// 长输出自动折叠
				if lines := strings.Count(msg.Output, "\n"); lines > foldThreshold {
					c.messages[i].Folded = true
				}
				break
			}
		}
		c.refreshContent()
		c.viewport.GotoBottom()

	case MsgResponseDone:
		// 流式结束，提交 streamBuf 为 assistant 消息
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

	for _, msg := range c.messages {
		sb.WriteString(c.renderMessage(msg))
		sb.WriteString("\n")
	}

	// 如果正在流式接收，渲染 streamBuf
	if c.streaming && c.streamBuf != "" {
		sb.WriteString(StyleAIMsg.Render(c.streamBuf))
		sb.WriteString(StyleHint.Render("▊")) // 闪烁光标
		sb.WriteString("\n")
	}

	c.viewport.SetContent(sb.String())
}

func (c *ChatView) renderMessage(msg ChatMessage) string {
	switch msg.Role {
	case "user":
		prefix := StyleUserPrefix.Render("> ")
		content := StyleUserMsg.Render(msg.Content)
		return prefix + content

	case "assistant":
		// 使用 Glamour 渲染 Markdown
		if c.renderer != nil {
			rendered, err := c.renderer.Render(msg.Content)
			if err == nil {
				return strings.TrimSpace(rendered)
			}
		}
		return StyleAIMsg.Render(msg.Content)

	case "thinking":
		// 折叠显示 thinking
		lines := strings.Split(msg.Content, "\n")
		preview := msg.Content
		if len(lines) > 3 {
			preview = strings.Join(lines[:3], "\n") + "\n..."
		}
		return StyleThinking.Render("💭 " + preview)

	case "tool":
		return c.renderToolMessage(msg)

	case "error":
		return StyleErrorMsg.Render("✗ " + msg.Content)

	default:
		return msg.Content
	}
}

func (c *ChatView) renderToolMessage(msg ChatMessage) string {
	icon := "⚙"
	nameStyle := StyleToolRunning
	if msg.Content == "执行中..." {
		nameStyle = StyleToolRunning
		return nameStyle.Render(fmt.Sprintf("%s %s 执行中...", icon, msg.ToolName))
	}

	if msg.IsError {
		icon = "✗"
		nameStyle = StyleToolError
	} else {
		icon = "✓"
		nameStyle = StyleToolSuccess
	}

	header := nameStyle.Render(fmt.Sprintf("%s %s", icon, msg.ToolName))

	if msg.Folded {
		lines := strings.Split(msg.Content, "\n")
		lineCount := len(lines)
		preview := strings.Join(lines[:3], "\n")
		return header + "\n" + StyleToolOutput.Render(preview) + "\n" +
			StyleHint.Render(fmt.Sprintf("  ... (%d 行, 已折叠)", lineCount))
	}

	if msg.Content != "" {
		return header + "\n" + StyleToolOutput.Render(msg.Content)
	}
	return header
}

// AddSystemMessage 添加系统消息
func (c *ChatView) AddSystemMessage(text string) {
	c.messages = append(c.messages, ChatMessage{
		Role:    "assistant",
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
