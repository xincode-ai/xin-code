package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	styles "github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
)

// 折叠阈值：超过此行数的工具输出自动折叠
const foldThreshold = 8

// toggleMarker 可点击折叠/展开的标记
type toggleMarker struct {
	msgIdx int    // 对应的消息索引
	marker string // 标记文本（用于在 viewport 行中匹配）
}

// streamPreviewThrottle 流式 Markdown 预览最小渲染间隔
const streamPreviewThrottle = 80 * time.Millisecond

// msgResponsePrefix CC 风格缩进前缀
const msgResponsePrefix = " " + SymResponse + " "

// ChatMessage 单条转录消息
type ChatMessage struct {
	ID        string // 稳定标识（递增 ID）
	Role      string // 用户 / 助手 / 工具 / 思考 / 错误 / 系统
	Content   string
	ToolName  string // 工具消息时使用
	ToolID    string // 工具调用标识（用于精确匹配同名工具）
	ToolInput string // 工具输入参数（原始 JSON）
	IsError   bool
	Folded    bool // 是否已折叠
}

// msgIDCounter 消息 ID 递增计数器
var msgIDCounter int

// nextMsgID 生成下一个消息 ID
func nextMsgID() string {
	msgIDCounter++
	return fmt.Sprintf("msg-%d", msgIDCounter)
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

	// 未读分隔线（-1 表示无）
	unreadDividerIdx int
	// 用户是否在底部（用于判断是否插入未读分隔线）
	userAtBottom bool
	// 有未读新消息（用于底部提示）
	hasNewMessages bool

	// Markdown 渲染器
	renderer *glamour.TermRenderer

	// Markdown 渲染缓存（原文 → 渲染结果，避免重复 parse）
	mdCache    map[string]string
	mdCacheMax int

	// 增量渲染：缓存已提交消息的渲染结果，streaming/blink 时只更新尾部
	committedRendered string // 所有已提交消息的渲染文本
	committedMsgCount int    // 缓存对应的消息数量
	committedBlinkSt  bool   // 缓存对应的 blink 状态
	cacheValid        bool   // 缓存是否有效

	// 流式 Markdown 预览：节流渲染当前 assistant 消息
	streamRenderedPreview string    // 当前流式消息的 Markdown 预览（为空则回退纯文本）
	lastPreviewRender     time.Time // 上次预览渲染的时间
	streamDirty           bool      // streamBuf 自上次预览渲染后有变更

	// 工具输出展开状态：true = 所有输出全展开，false = 超阈值自动折叠
	toolOutputExpanded bool

	// 可点击的标记文本 → 消息索引（通过在内容行中搜索标记来定位）
	toggleMarkers []toggleMarker
}

// newGlamourRenderer 创建白色文字的 Glamour 渲染器
func newGlamourRenderer(width int) *glamour.TermRenderer {
	// 基于 dark 主题，覆盖颜色使其在深色终端上更柔和
	style := styles.DarkStyleConfig
	white := "#FFFFFF"
	style.Document.Color = &white
	style.Paragraph.Color = &white

	// inline code：去掉刺眼的红色背景，改为柔和的青色无背景
	codeColor := "#88C0D0"
	style.Code.StylePrimitive.Color = &codeColor
	style.Code.StylePrimitive.BackgroundColor = nil

	r, _ := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(max(20, width-4)),
	)
	return r
}

// contentPadLeft 内容区左侧 padding（对齐 Claude Code 的视觉间距）
const contentPadLeft = 2

// NewChatView 创建转录区域
func NewChatView(width, height int) ChatView {
	innerWidth := width - contentPadLeft
	vp := viewport.New(innerWidth, height)
	vp.Style = lipgloss.NewStyle().PaddingLeft(contentPadLeft)
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 3

	return ChatView{
		viewport:         vp,
		width:            innerWidth,
		height:           height,
		renderer:         newGlamourRenderer(innerWidth),
		mdCache:          make(map[string]string),
		mdCacheMax:       500,
		unreadDividerIdx: -1,
		userAtBottom:     true,
	}
}

// SetToolOutputExpanded 设置工具输出展开状态
func (c *ChatView) SetToolOutputExpanded(expanded bool) {
	c.toolOutputExpanded = expanded
}

// IsToolOutputExpanded 返回工具输出展开状态
func (c ChatView) IsToolOutputExpanded() bool {
	return c.toolOutputExpanded
}

func (c ChatView) Init() tea.Cmd { return nil }

func (c ChatView) Update(msg tea.Msg) (ChatView, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		innerWidth := msg.Width - contentPadLeft
		c.width = innerWidth
		c.height = msg.Height
		c.viewport.Width = innerWidth
		c.viewport.Height = msg.Height
		c.renderer = newGlamourRenderer(msg.Width)
		c.mdCache = make(map[string]string)
		c.invalidateCache()
		c.refreshContent(c.shouldAutoScroll())

	case MsgSpinnerTick:
		needRefresh := false
		// 只在有执行中工具或 thinking 时翻转闪烁
		if c.hasInProgressTools() || c.hasThinking() {
			c.toolBlink = !c.toolBlink
			needRefresh = true
		}
		// 流式 Markdown 预览定时刷新（补充 delta 未触发的更新）
		if c.streaming && c.streamDirty {
			c.renderStreamingPreview()
			needRefresh = true
		}
		if needRefresh {
			c.refreshContent(c.shouldAutoScroll())
		}

	case MsgTextDelta:
		stick := c.shouldAutoScroll()
		c.streaming = true
		c.streamBuf += msg.Text
		c.streamDirty = true
		c.maybeRefreshStreamingPreview()
		c.refreshStreaming(stick) // 增量：只追加 streamBuf，不重建 transcript

	case MsgThinking:
		stick := c.shouldAutoScroll()
		if len(c.messages) > 0 && c.messages[len(c.messages)-1].Role == "thinking" {
			c.messages[len(c.messages)-1].Content += msg.Text
		} else {
			c.messages = append(c.messages, ChatMessage{
				ID:      nextMsgID(),
				Role:    "thinking",
				Content: msg.Text,
			})
		}
		c.invalidateCache()
		c.refreshContent(stick)

	case MsgToolStart:
		stick := c.shouldAutoScroll()
		c.messages = append(c.messages, ChatMessage{
			ID:        nextMsgID(),
			Role:      "tool",
			ToolName:  msg.Name,
			ToolID:    msg.ID,
			ToolInput: msg.Input,
			Content:   "", // 空内容表示执行中
		})
		c.markNewMessageIfScrolledUp(stick)
		c.invalidateCache()
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
		c.invalidateCache()
		c.refreshContent(stick)

	case MsgResponseDone:
		stick := c.shouldAutoScroll()
		if c.streamBuf != "" {
			c.messages = append(c.messages, ChatMessage{
				ID:      nextMsgID(),
				Role:    "assistant",
				Content: c.streamBuf,
			})
			c.streamBuf = ""
			c.streaming = false
			c.streamRenderedPreview = ""
			c.streamDirty = false
			c.markNewMessageIfScrolledUp(stick)
			c.invalidateCache()
			c.refreshContent(stick) // 最终完整 Glamour 渲染走 committed cache
		}

	case MsgSubmit:
		c.messages = append(c.messages, ChatMessage{
			ID:      nextMsgID(),
			Role:    "user",
			Content: msg.Text,
		})
		c.streamBuf = ""
		c.streaming = false
		c.streamRenderedPreview = ""
		c.streamDirty = false
		c.unreadDividerIdx = -1
		c.hasNewMessages = false
		c.userAtBottom = true
		c.invalidateCache()
		c.refreshContent(true)

	case MsgSubAgentStart:
		stick := c.shouldAutoScroll()
		c.messages = append(c.messages, ChatMessage{
			ID:      nextMsgID(),
			Role:    "subagent-start",
			ToolID:  msg.ID,
			Content: msg.Description,
		})
		c.invalidateCache()
		c.refreshContent(stick)

	case MsgSubAgentDone:
		stick := c.shouldAutoScroll()
		c.messages = append(c.messages, ChatMessage{
			ID:      nextMsgID(),
			Role:    "subagent-done",
			ToolID:  msg.ID,
			Content: msg.Description + "\n" + msg.Result,
		})
		c.invalidateCache()
		c.refreshContent(stick)

	case MsgError:
		stick := c.shouldAutoScroll()
		c.messages = append(c.messages, ChatMessage{
			ID:      nextMsgID(),
			Role:    "error",
			Content: msg.Err.Error(),
		})
		c.invalidateCache()
		c.refreshContent(stick)
	}

	// 鼠标左键点击/释放：拦截（防止穿透到 viewport 触发滚动）
	if mouseMsg, ok := msg.(tea.MouseMsg); ok &&
		mouseMsg.Button == tea.MouseButtonLeft {
		// 只在释放时触发 toggle
		if mouseMsg.Action == tea.MouseActionRelease {
			lines := strings.Split(c.viewport.View(), "\n")
			if mouseMsg.Y >= 0 && mouseMsg.Y < len(lines) {
				lineContent := lines[mouseMsg.Y]
				for _, m := range c.toggleMarkers {
					if m.msgIdx < len(c.messages) && strings.Contains(lineContent, m.marker) {
						c.messages[m.msgIdx].Folded = !c.messages[m.msgIdx].Folded
						c.invalidateCache()
						c.refreshContent(false)
						break
					}
				}
			}
		}
		return c, nil // 左键事件始终拦截，不传给 viewport
	}

	// t 键 toggle thinking 折叠/展开（在 viewport 之前拦截）
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "t" {
		for i := len(c.messages) - 1; i >= 0; i-- {
			if c.messages[i].Role == "thinking" {
				c.messages[i].Folded = !c.messages[i].Folded
				c.invalidateCache()
				c.refreshContent(false)
				return c, cmd
			}
		}
	}

	// Ctrl+O toggle 工具输出展开/折叠
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyCtrlO {
		c.toolOutputExpanded = !c.toolOutputExpanded
		c.invalidateCache()
		c.refreshContent(false)
		return c, nil
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
		ID:      nextMsgID(),
		Role:    "system",
		Content: text,
	})
	c.invalidateCache()
	c.refreshContent(true)
}

// Clear 清空消息
func (c *ChatView) Clear() {
	c.messages = nil
	c.streamBuf = ""
	c.streaming = false
	c.streamRenderedPreview = ""
	c.streamDirty = false
	c.unreadDividerIdx = -1
	c.hasNewMessages = false
	c.userAtBottom = true
	c.invalidateCache()
	c.refreshContent(true)
}

// LoadMessages 从外部替换全部消息并刷新视图（用于 /resume 恢复 transcript）
func (c *ChatView) LoadMessages(msgs []ChatMessage) {
	for i := range msgs {
		if msgs[i].ID == "" {
			msgs[i].ID = nextMsgID()
		}
	}
	c.messages = msgs
	c.streamBuf = ""
	c.streaming = false
	c.streamRenderedPreview = ""
	c.streamDirty = false
	c.unreadDividerIdx = -1
	c.hasNewMessages = false
	c.userAtBottom = true
	c.invalidateCache()
	c.refreshContent(false)
	c.viewport.GotoBottom()
}

// InvalidateCache 清空 Markdown 渲染缓存并重建（主题切换后调用）
func (c *ChatView) InvalidateCache() {
	c.mdCache = make(map[string]string)
	c.invalidateCache()
	c.refreshContent(c.shouldAutoScroll())
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

// invalidateCache 标记已提交消息缓存失效（消息增删改时调用）
func (c *ChatView) invalidateCache() {
	c.cacheValid = false
}

// rebuildCommittedCache 重建已提交消息的渲染缓存，同时收集可点击标记
func (c *ChatView) rebuildCommittedCache() {
	var sb strings.Builder
	var markers []toggleMarker

	for i, msg := range c.messages {
		if i == c.unreadDividerIdx && i > 0 {
			sb.WriteString("\n\n")
			dividerWidth := c.width - 4
			if dividerWidth < 10 {
				dividerWidth = 10
			}
			divStyle := lipgloss.NewStyle().Foreground(ColorWarning)
			sideWidth := (dividerWidth - 8) / 2
			if sideWidth < 2 {
				sideWidth = 2
			}
			sb.WriteString(divStyle.Render(
				strings.Repeat("━", sideWidth) + " 新消息 " + strings.Repeat("━", sideWidth)))
		}
		if i > 0 {
			sb.WriteString("\n\n")
		}
		rendered := c.renderMessage(msg)
		if rendered != "" {
			sb.WriteString(rendered)

			// 收集可点击标记（用渲染后的实际行内容匹配，保证唯一性）
			switch msg.Role {
			case "thinking":
				// 用 header 首行做标记（含字数，每条 thinking 唯一）
				firstLine := strings.SplitN(rendered, "\n", 2)[0]
				markers = append(markers, toggleMarker{msgIdx: i, marker: firstLine})
			case "tool":
				if msg.Content != "" {
					// header 行（含工具名+参数，每条工具唯一）
					firstLine := strings.SplitN(rendered, "\n", 2)[0]
					markers = append(markers, toggleMarker{msgIdx: i, marker: firstLine})
					// 折叠提示行
					rlines := strings.Split(rendered, "\n")
					lastLine := rlines[len(rlines)-1]
					if strings.Contains(lastLine, "点击展开") {
						markers = append(markers, toggleMarker{msgIdx: i, marker: lastLine})
					}
				}
			}
		}
	}
	c.committedRendered = sb.String()
	c.toggleMarkers = markers
	c.committedMsgCount = len(c.messages)
	c.committedBlinkSt = c.toolBlink
	c.cacheValid = true
}

// refreshContent 重新渲染消息到视口（利用缓存避免全量重建）
func (c *ChatView) refreshContent(stickToBottom bool) {
	// 判断缓存是否可用
	needRebuild := !c.cacheValid ||
		c.committedMsgCount != len(c.messages) ||
		(c.committedBlinkSt != c.toolBlink && (c.hasInProgressTools() || c.hasThinking()))

	if needRebuild {
		c.rebuildCommittedCache()
	}

	// 拼接：已提交缓存 + 流式尾部
	content := c.committedRendered
	if c.streaming && c.streamBuf != "" {
		if len(c.messages) > 0 {
			content += "\n\n"
		}
		content += c.renderStreamingMessage()
	}

	c.viewport.SetContent(content)
	if stickToBottom {
		c.viewport.GotoBottom()
	}
}

// refreshStreaming 只更新流式尾部，不重建已提交消息（MsgTextDelta 专用）
func (c *ChatView) refreshStreaming(stickToBottom bool) {
	// 确保 committed 缓存存在
	if !c.cacheValid {
		c.rebuildCommittedCache()
	}

	content := c.committedRendered
	if c.streamBuf != "" {
		if len(c.messages) > 0 {
			content += "\n\n"
		}
		content += c.renderStreamingMessage()
	}

	c.viewport.SetContent(content)
	if stickToBottom {
		c.viewport.GotoBottom()
	}
}

// View 渲染转录区（含底部"跳到最新"提示）
func (c ChatView) ViewWithHint() string {
	view := c.viewport.View()
	if c.hasNewMessages && !c.userAtBottom {
		count := c.countNewMessages()
		hint := lipgloss.NewStyle().Foreground(ColorBrand).Render(
			fmt.Sprintf("  ↓ %d 条新消息，按 End 跳到最新", count))
		return view + "\n" + hint
	}
	return view
}

func (c ChatView) countNewMessages() int {
	if c.unreadDividerIdx < 0 {
		return 1
	}
	count := 0
	for i := c.unreadDividerIdx; i < len(c.messages); i++ {
		if c.messages[i].Role != "system" && c.messages[i].Role != "thinking" {
			count++
		}
	}
	if count < 1 {
		count = 1
	}
	return count
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

func (c *ChatView) shouldAutoScroll() bool {
	atBottom := c.viewport.AtBottom() || c.viewport.TotalLineCount() == 0
	c.userAtBottom = atBottom
	if atBottom {
		// 用户回到底部，清除未读状态
		c.unreadDividerIdx = -1
		c.hasNewMessages = false
	}
	return atBottom
}

// markNewMessageIfScrolledUp 如果用户不在底部，标记未读分隔线
func (c *ChatView) markNewMessageIfScrolledUp(atBottom bool) {
	if !atBottom && c.unreadDividerIdx < 0 {
		c.unreadDividerIdx = len(c.messages) - 1 // 在最新消息前插入分隔线
		c.hasNewMessages = true
	}
}

// renderMessage 按 Role 分派渲染
func (c *ChatView) renderMessage(msg ChatMessage) string {
	switch msg.Role {
	case "user":
		// ❯ 用户文本（白色粗体）
		prefix := StyleUserText.Render(SymUserPrompt + " ")
		return prefix + StyleUserText.Render(strings.TrimSpace(msg.Content))

	case "assistant":
		// MessageResponse 包裹 Glamour 渲染的 Markdown（带缓存）
		body := msg.Content
		if c.renderer != nil {
			if cached, ok := c.mdCache[body]; ok {
				body = cached
			} else {
				rendered, err := c.renderer.Render(body)
				if err == nil {
					original := body
					body = strings.TrimSpace(rendered)
					// 缓存满时简单清空重来
					if len(c.mdCache) >= c.mdCacheMax {
						c.mdCache = make(map[string]string)
					}
					c.mdCache[original] = body
				}
			}
		}
		return wrapMessageResponse(body)

	case "thinking":
		// ∴ Thinking — 符号在 brand/dim 色间闪烁，给用户"还在运转"的感知
		symStyle := StyleThinking
		if c.toolBlink {
			symStyle = lipgloss.NewStyle().Foreground(ColorBrand).Italic(true)
		}

		runeCount := utf8.RuneCountInString(msg.Content)
		countStr := ""
		if runeCount > 0 {
			countStr = StyleDim.Render(fmt.Sprintf(" (%d 字)", runeCount))
		}

		if msg.Folded || msg.Content == "" {
			// 折叠态：▶ ∴ Thinking (N 字)  首行摘要…
			toggle := StyleDim.Render("▶ ")
			header := toggle + symStyle.Render(SymThinking) + StyleThinking.Render(" Thinking") + countStr
			if msg.Content != "" {
				// 取首行前 40 个 rune 作为摘要
				summary := strings.ReplaceAll(msg.Content, "\n", " ")
				runes := []rune(summary)
				if len(runes) > 40 {
					summary = string(runes[:40]) + "…"
				}
				header += "  " + StyleDim.Render(summary)
			}
			return header
		}

		// 展开态：▼ ∴ Thinking (N 字) + 完整内容（限宽自动换行）
		toggle := StyleDim.Render("▼ ")
		header := toggle + symStyle.Render(SymThinking) + StyleThinking.Render(" Thinking") + countStr
		// 内容区宽度 = 终端宽度 - wrapMessageResponse 前缀缩进（约 6 列）
		bodyWidth := c.width - 6
		if bodyWidth < 20 {
			bodyWidth = 20
		}
		bodyStyle := lipgloss.NewStyle().Foreground(ColorTextDim).Italic(true).Width(bodyWidth)
		body := bodyStyle.Render(msg.Content)
		return header + "\n" + wrapMessageResponse(body)

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
		return lipgloss.NewStyle().
			BorderLeft(true).
			BorderForeground(ColorError).
			PaddingLeft(1).
			Render("⚠ " + StyleError.Render(msg.Content))

	default:
		return msg.Content
	}
}

// renderStreamingMessage 流式输出：优先使用 Markdown 预览，无预览时回退纯文本
func (c *ChatView) renderStreamingMessage() string {
	if c.streamRenderedPreview != "" {
		return wrapMessageResponse(c.streamRenderedPreview)
	}
	return wrapMessageResponse(c.streamBuf)
}

// maybeRefreshStreamingPreview 节流 + 边界触发的流式 Markdown 预览
func (c *ChatView) maybeRefreshStreamingPreview() {
	if !c.streamDirty || c.streamBuf == "" {
		return
	}
	elapsed := time.Since(c.lastPreviewRender)
	if elapsed >= streamPreviewThrottle || c.hitMarkdownBoundary() {
		c.renderStreamingPreview()
	}
}

// hitMarkdownBoundary 检查 streamBuf 尾部是否命中 Markdown 结构边界
func (c *ChatView) hitMarkdownBoundary() bool {
	buf := c.streamBuf
	n := len(buf)
	if n < 2 {
		return false
	}

	// 段落边界：\n\n
	if n >= 2 && buf[n-2] == '\n' && buf[n-1] == '\n' {
		return true
	}

	// 检查最后一行的开头特征
	lastNL := strings.LastIndex(buf[:n-1], "\n")
	var lastLine string
	if lastNL >= 0 {
		lastLine = buf[lastNL+1:]
	} else {
		lastLine = buf
	}
	trimmed := strings.TrimSpace(lastLine)

	// 代码块 fence 开/关
	if strings.HasPrefix(trimmed, "```") {
		return true
	}
	// 标题行
	if len(trimmed) > 1 && trimmed[0] == '#' && (trimmed[1] == ' ' || trimmed[1] == '#') {
		return true
	}
	// 无序列表项
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		return true
	}
	// 有序列表项
	if len(trimmed) >= 3 && trimmed[0] >= '0' && trimmed[0] <= '9' {
		for i := 1; i < len(trimmed) && i < 4; i++ {
			if trimmed[i] == '.' && i+1 < len(trimmed) && trimmed[i+1] == ' ' {
				return true
			}
			if trimmed[i] < '0' || trimmed[i] > '9' {
				break
			}
		}
	}
	// 引用块
	if strings.HasPrefix(trimmed, "> ") {
		return true
	}
	// 表格行
	if strings.HasPrefix(trimmed, "|") {
		return true
	}

	return false
}

// renderStreamingPreview 对当前 streamBuf 执行 Glamour Markdown 渲染
// 渲染失败时回退为空（renderStreamingMessage 会降级为纯文本）
func (c *ChatView) renderStreamingPreview() {
	if c.renderer == nil || c.streamBuf == "" {
		return
	}
	rendered, err := c.renderer.Render(c.streamBuf)
	if err == nil {
		c.streamRenderedPreview = strings.TrimSpace(rendered)
	} else {
		// 渲染失败：清空预览，降级为纯文本（避免显示旧的 stale 预览）
		c.streamRenderedPreview = ""
	}
	// 无论成败都更新时间戳和 dirty 标记，避免频繁重试
	c.streamDirty = false
	c.lastPreviewRender = time.Now()
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
	// 折叠逻辑：autoFold XOR 用户手动 toggle（Folded）
	// 默认 Folded=false：超阈值自动折叠；用户点击后 Folded=true 反转为展开
	autoFold := lineCount > foldThreshold
	if !c.toolOutputExpanded && (autoFold != msg.Folded) {
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
	isFolded := len(displayLines) < lineCount
	if isFolded {
		outputBody += "\n" + lipgloss.NewStyle().Foreground(ColorBrand).Render(
			fmt.Sprintf("▶ [+%d 行] 点击展开", lineCount-len(displayLines)))
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
