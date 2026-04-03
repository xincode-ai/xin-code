package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	styles "github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
)

// 折叠阈值：超过此行数的工具输出自动折叠
const foldThreshold = 8

// msgResponsePrefix CC 风格缩进前缀
const msgResponsePrefix = " " + SymResponse + " "

// ChatMessage 单条转录消息
type ChatMessage struct {
	Role      string // 用户 / 助手 / 工具 / 思考 / 错误 / 系统
	Content   string
	ToolName  string // 工具消息时使用
	ToolID    string // 工具调用标识（用于精确匹配同名工具）
	ToolInput string // 工具输入参数（原始 JSON）
	IsError   bool
	Folded    bool // 是否已折叠
}

// ChatView 转录区域组件
type ChatView struct {
	viewport viewport.Model
	messages []ChatMessage
	width    int
	height   int

	// 流式状态
	streaming bool
	streamBuf string

	// 工具闪烁状态，配合 MsgSpinnerTick 交替
	toolBlink bool

	// Markdown 渲染器
	renderer *glamour.TermRenderer
}

// newGlamourRenderer 创建白色文字的 Glamour 渲染器
func newGlamourRenderer(width int) *glamour.TermRenderer {
	// 基于 dark 主题，覆盖 Document 前景色为白色
	style := styles.DarkStyleConfig
	white := "#FFFFFF"
	style.Document.Color = &white
	style.Paragraph.Color = &white

	r, _ := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(max(20, width-4)),
	)
	return r
}

// NewChatView 创建转录区域
func NewChatView(width, height int) ChatView {
	vp := viewport.New(width, height)
	vp.Style = lipgloss.NewStyle()
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 3

	return ChatView{
		viewport: vp,
		width:    width,
		height:   height,
		renderer: newGlamourRenderer(width),
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
		c.renderer = newGlamourRenderer(msg.Width)
		c.refreshContent(c.shouldAutoScroll())

	case MsgSpinnerTick:
		// 在有执行中工具或 thinking 时刷新（驱动闪烁动画）
		c.toolBlink = !c.toolBlink
		if c.hasInProgressTools() || c.hasThinking() {
			c.refreshContent(c.shouldAutoScroll())
		}

	case MsgTextDelta:
		stick := c.shouldAutoScroll()
		c.streaming = true
		c.streamBuf += msg.Text
		c.refreshContent(stick)

	case MsgThinking:
		stick := c.shouldAutoScroll()
		if len(c.messages) > 0 && c.messages[len(c.messages)-1].Role == "thinking" {
			c.messages[len(c.messages)-1].Content += msg.Text
		} else {
			c.messages = append(c.messages, ChatMessage{
				Role:    "thinking",
				Content: msg.Text,
			})
		}
		c.refreshContent(stick)

	case MsgToolStart:
		stick := c.shouldAutoScroll()
		c.messages = append(c.messages, ChatMessage{
			Role:      "tool",
			ToolName:  msg.Name,
			ToolID:    msg.ID,
			ToolInput: msg.Input,
			Content:   "", // 空内容表示执行中
		})
		c.refreshContent(stick)

	case MsgToolDone:
		stick := c.shouldAutoScroll()
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
		c.refreshContent(stick)

	case MsgResponseDone:
		stick := c.shouldAutoScroll()
		if c.streamBuf != "" {
			c.messages = append(c.messages, ChatMessage{
				Role:    "assistant",
				Content: c.streamBuf,
			})
			c.streamBuf = ""
			c.streaming = false
			c.refreshContent(stick)
		}

	case MsgSubmit:
		c.messages = append(c.messages, ChatMessage{
			Role:    "user",
			Content: msg.Text,
		})
		c.streamBuf = ""
		c.streaming = false
		c.refreshContent(true)

	case MsgSubAgentStart:
		stick := c.shouldAutoScroll()
		c.messages = append(c.messages, ChatMessage{
			Role:    "subagent-start",
			ToolID:  msg.ID,
			Content: msg.Description,
		})
		c.refreshContent(stick)

	case MsgSubAgentDone:
		stick := c.shouldAutoScroll()
		c.messages = append(c.messages, ChatMessage{
			Role:    "subagent-done",
			ToolID:  msg.ID,
			Content: msg.Description + "\n" + msg.Result,
		})
		c.refreshContent(stick)

	case MsgError:
		stick := c.shouldAutoScroll()
		c.messages = append(c.messages, ChatMessage{
			Role:    "error",
			Content: msg.Err.Error(),
		})
		c.refreshContent(stick)
	}

	c.viewport, cmd = c.viewport.Update(msg)
	return c, cmd
}

func (c ChatView) View() string {
	return c.viewport.View()
}

// AddSystemMessage 添加系统消息
func (c *ChatView) AddSystemMessage(text string) {
	c.messages = append(c.messages, ChatMessage{
		Role:    "system",
		Content: text,
	})
	c.refreshContent(true)
}

// Clear 清空消息
func (c *ChatView) Clear() {
	c.messages = nil
	c.streamBuf = ""
	c.streaming = false
	c.refreshContent(true)
}

// HasMessages 返回是否有用户发起的消息（非 system 消息）
func (c ChatView) HasMessages() bool {
	for _, m := range c.messages {
		if m.Role != "system" {
			return true
		}
	}
	return c.streaming
}

// refreshContent 重新渲染所有消息到可滚动视口
func (c *ChatView) refreshContent(stickToBottom bool) {
	var sb strings.Builder

	for i, msg := range c.messages {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		rendered := c.renderMessage(msg)
		if rendered != "" {
			sb.WriteString(rendered)
		}
	}

	if c.streaming && c.streamBuf != "" {
		if len(c.messages) > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(c.renderStreamingMessage())
	}

	c.viewport.SetContent(sb.String())
	if stickToBottom {
		c.viewport.GotoBottom()
	}
}

func (c ChatView) hasInProgressTools() bool {
	for _, m := range c.messages {
		if m.Role == "tool" && m.Content == "" {
			return true
		}
	}
	return false
}

func (c ChatView) hasThinking() bool {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].Role == "thinking" {
			return true
		}
		// thinking 只在最近的消息中有意义，遇到其他 role 就停
		if c.messages[i].Role == "user" || c.messages[i].Role == "assistant" {
			return false
		}
	}
	return false
}

func (c ChatView) shouldAutoScroll() bool {
	return c.viewport.AtBottom() || c.viewport.TotalLineCount() == 0
}

// renderMessage 按 Role 分派渲染
func (c *ChatView) renderMessage(msg ChatMessage) string {
	switch msg.Role {
	case "user":
		// ❯ 用户文本（白色粗体）
		prefix := StyleUserText.Render(SymUserPrompt + " ")
		return prefix + StyleUserText.Render(strings.TrimSpace(msg.Content))

	case "assistant":
		// MessageResponse 包裹 Glamour 渲染的 Markdown
		body := msg.Content
		if c.renderer != nil {
			rendered, err := c.renderer.Render(msg.Content)
			if err == nil {
				body = strings.TrimSpace(rendered)
			}
		}
		return wrapMessageResponse(body)

	case "thinking":
		// ∴ Thinking — 符号在 brand/dim 色间闪烁，给用户"还在运转"的感知
		symStyle := StyleThinking
		if c.toolBlink {
			symStyle = lipgloss.NewStyle().Foreground(ColorBrand).Italic(true)
		}
		return symStyle.Render(SymThinking) + StyleThinking.Render(" Thinking")

	case "tool":
		return c.renderToolMessage(msg)

	case "subagent-start":
		// ⏺ Agent: {description}（品牌色）
		marker := lipgloss.NewStyle().Foreground(ColorBrand).Render(BlackCircle())
		label := lipgloss.NewStyle().Foreground(ColorBrand).Bold(true).Render("Agent")
		desc := StyleDim.Render(msg.Content)
		return marker + " " + label + ": " + desc

	case "subagent-done":
		// ⎿ Agent 完成: {result 摘要}
		parts := strings.SplitN(msg.Content, "\n", 2)
		desc := parts[0]
		result := ""
		if len(parts) > 1 {
			result = parts[1]
		}
		marker := lipgloss.NewStyle().Foreground(ColorSuccess).Render(BlackCircle())
		label := lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true).Render("Agent 完成")
		header := marker + " " + label + ": " + StyleDim.Render(desc)
		if result != "" {
			return header + "\n" + wrapMessageResponse(StyleToolOutput.Render(result))
		}
		return header

	case "system":
		return StyleDim.Render(msg.Content)

	case "error":
		return wrapMessageResponse(StyleError.Render(msg.Content))

	default:
		return msg.Content
	}
}

// renderStreamingMessage 流式输出：对整个 streamBuf 调 Glamour 渲染后包裹
func (c *ChatView) renderStreamingMessage() string {
	body := c.streamBuf
	if c.renderer != nil {
		rendered, err := c.renderer.Render(body)
		if err == nil {
			body = strings.TrimSpace(rendered)
		}
	}
	return wrapMessageResponse(body)
}

// renderToolMessage 渲染工具调用行
//
//	执行中（Content 为空）：[闪烁⏺] ToolName(args)      — dim ⏺ 交替
//	完成：                  [绿⏺] ToolName(args) + ⎿ 输出
//	失败：                  [红⏺] ToolName(args) + ⎿ 输出
func (c *ChatView) renderToolMessage(msg ChatMessage) string {
	argPreview := toolArgPreview(msg.ToolName, msg.ToolInput)
	nameStr := StyleToolName.Render(msg.ToolName)
	argsStr := ""
	if argPreview != "" {
		argsStr = StyleDim.Render("(" + argPreview + ")")
	}
	callStr := nameStr + argsStr

	running := msg.Content == ""

	var marker string
	switch {
	case running:
		// 闪烁：交替显示 ⏺ 和空格
		if c.toolBlink {
			marker = StyleDim.Render(BlackCircle())
		} else {
			marker = " "
		}
	case msg.IsError:
		marker = lipgloss.NewStyle().Foreground(ColorError).Render(BlackCircle())
	default:
		marker = lipgloss.NewStyle().Foreground(ColorSuccess).Render(BlackCircle())
	}

	header := marker + " " + callStr

	if running {
		return header
	}

	// 有输出内容，用 MessageResponse 包裹
	output := msg.Content
	lines := strings.Split(output, "\n")
	lineCount := len(lines)

	var displayLines []string
	if msg.Folded || lineCount > foldThreshold {
		// 折叠：显示前 3 行 + 省略提示
		previewEnd := 3
		if previewEnd > lineCount {
			previewEnd = lineCount
		}
		displayLines = lines[:previewEnd]
	} else {
		displayLines = lines
	}

	outputBody := StyleToolOutput.Render(strings.Join(displayLines, "\n"))
	if len(displayLines) < lineCount {
		outputBody += "\n" + StyleDim.Render(fmt.Sprintf("… +%d lines", lineCount-len(displayLines)))
	}

	return header + "\n" + wrapMessageResponse(outputBody)
}

// wrapMessageResponse 将内容用 CC 风格的 ⎿ 前缀包裹
//
//	第一行: "  ⎿  " + content_line_1    （dim 色的前缀）
//	后续行: "      " + content_line_N    （等宽空格缩进）
func wrapMessageResponse(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return StyleMsgResponse.Render(msgResponsePrefix)
	}

	prefix := StyleMsgResponse.Render(msgResponsePrefix)
	indent := strings.Repeat(" ", lipgloss.Width(msgResponsePrefix))

	// 压缩连续空行（Glamour 段落间距太大）
	lines := strings.Split(content, "\n")
	var compressed []string
	prevEmpty := false
	for _, line := range lines {
		isEmpty := strings.TrimSpace(line) == ""
		if isEmpty && prevEmpty {
			continue // 跳过连续空行
		}
		compressed = append(compressed, line)
		prevEmpty = isEmpty
	}

	var result []string
	for i, line := range compressed {
		if i == 0 {
			result = append(result, prefix+line)
		} else {
			result = append(result, indent+line)
		}
	}
	return strings.Join(result, "\n")
}

// toolArgPreview 从工具输入 JSON 中提取关键参数作为预览
func toolArgPreview(name, rawInput string) string {
	if rawInput == "" {
		return ""
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(rawInput), &m); err != nil {
		if len(rawInput) > 50 {
			return rawInput[:47] + "..."
		}
		return rawInput
	}

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
