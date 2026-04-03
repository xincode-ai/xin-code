# TUI Claude Code 视觉一致性改造

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Xin Code 的 TUI 完全对标 Claude Code 的交互界面，从消息渲染、颜色体系、布局结构到动画效果实现一致。

**Architecture:** 基于 CC 源码反推实现。核心改造集中在渲染层（chat.go）和样式层（theme.go），布局从「顶栏+Footer」改为「纯内容+底部 Spinner+输入框」。消息渲染从边框块模式切换到 CC 的前缀模式（⎿/⏺/∴）。流式 Markdown 通过每次 delta 全量 Glamour 渲染实现。

**Tech Stack:** Go 1.26 / Bubbletea / Lipgloss / Glamour / Bubbles

**CC 源码参考位置:** `/Users/ocean/Studio/03-lab/03-explore/claude-code源码/claude-code-sourcemap/restored-src/src/`

---

## 文件变更清单

| 文件 | 操作 | 职责 |
|------|------|------|
| `internal/tui/theme.go` | 重写 | CC 颜色体系 + 精简样式 |
| `internal/tui/chat.go` | 重写 | CC 消息渲染（⎿/⏺/∴ 前缀模式 + 流式 Markdown） |
| `internal/tui/app.go` | 修改 | 布局重构（去顶栏、Spinner 行、简化 Footer） |
| `internal/tui/input.go` | 修改 | CC 风格输入框 |
| `internal/tui/statusbar.go` | 删除 | 不再需要独立状态栏组件 |
| `internal/tui/permission.go` | 修改 | CC 风格权限确认 |
| `internal/tui/spinner.go` | 新建 | CC 风格 Spinner（Braille/花型帧 + 随机动词 + 耗时） |

---

### Task 1: theme.go — CC 颜色体系

**Files:**
- Rewrite: `internal/tui/theme.go`

**CC 源码参考：**
- `src/utils/theme.ts:440-454` — Dark theme 颜色定义
- `src/constants/figures.ts` — 符号定义

- [ ] **Step 1: 重写 theme.go 颜色令牌**

用 CC dark theme 的精确 RGB 值替换当前颜色体系。删除所有边框块样式（StyleUserBlock、StyleAssistantBlock、StyleToolBlock 等），新增 CC 前缀模式需要的样式。

```go
package tui

import (
	"os"
	"runtime"

	"github.com/charmbracelet/lipgloss"
)

func init() {
	lipgloss.SetHasDarkBackground(true)
	os.Setenv("CLICOLOR_FORCE", "1")
}

// === CC 符号体系（来自 constants/figures.ts）===

// BLACK_CIRCLE 工具状态指示符（CC: macOS 用 ⏺，其他用 ●）
func BlackCircle() string {
	if runtime.GOOS == "darwin" {
		return "⏺"
	}
	return "●"
}

const (
	SymThinking    = "∴"  // Thinking 前缀（U+2234）
	SymResponse    = "⎿"  // MessageResponse 前缀（U+23BF）
	SymPause       = "⏸"  // 中断标记
	SymBlockquote  = "▎"  // 块引用左边框
	SymUserPrompt  = "❯"  // 用户输入提示符
)

// === CC 颜色体系（来自 utils/theme.ts dark theme）===

var (
	// 品牌色 — CC 橙（theme.ts:443 claude: 'rgb(215,119,87)'）
	ColorClaude       = lipgloss.Color("#D77757")
	ColorClaudeShimmer = lipgloss.Color("#EB9F7F")

	// 语义色（theme.ts:460-463）
	ColorSuccess = lipgloss.Color("#2CB74F") // rgb(44,183,79)
	ColorError   = lipgloss.Color("#CC3333") // rgb(204,51,51)
	ColorWarning = lipgloss.Color("#DCA032") // rgb(220,160,50)

	// 权限蓝（theme.ts:447 permission）
	ColorPermission = lipgloss.Color("#B1B9F9") // rgb(177,185,249)

	// 文本色（theme.ts:453-456）
	ColorText     = lipgloss.Color("#FFFFFF") // rgb(255,255,255)
	ColorTextDim  = lipgloss.Color("#A0A0A0") // rgb(160,160,160) inactive
	ColorSubtle   = lipgloss.Color("#6E6E6E") // dim 前缀色

	// 边框（theme.ts:451 promptBorder）
	ColorPromptBorder = lipgloss.Color("#888888") // rgb(136,136,136)

	// Diff 色（theme.ts:464-467）
	ColorDiffAdded   = lipgloss.Color("#4ADE80")
	ColorDiffRemoved = lipgloss.Color("#FB7185")

	// 上下文进度条
	ColorCtxLow  = lipgloss.Color("#34D399")
	ColorCtxMid  = lipgloss.Color("#F59E0B")
	ColorCtxHigh = lipgloss.Color("#F87171")
)

// === CC 风格样式 ===

var (
	// MessageResponse 前缀：dim 色的 "  ⎿  "
	// CC 源码: MessageResponse.tsx:22 — <Text dimColor>{"  "}⎿  </Text>
	StyleMsgResponse = lipgloss.NewStyle().Foreground(ColorSubtle)

	// 用户消息：纯白粗体（CC 无前缀，但需区分）
	StyleUserText = lipgloss.NewStyle().Foreground(ColorText).Bold(true)

	// Thinking：dim + italic（CC: AssistantThinkingMessage.tsx:44）
	StyleThinking = lipgloss.NewStyle().Foreground(ColorTextDim).Italic(true)

	// 工具名：粗体（CC: AssistantToolUseMessage.tsx:200 bold=true）
	StyleToolName = lipgloss.NewStyle().Bold(true)

	// 工具输出：dim
	StyleToolOutput = lipgloss.NewStyle().Foreground(ColorTextDim)

	// 错误文本
	StyleError = lipgloss.NewStyle().Foreground(ColorError)

	// Spinner 品牌色
	StyleSpinner = lipgloss.NewStyle().Foreground(ColorClaude)

	// dim 辅助文本
	StyleDim = lipgloss.NewStyle().Foreground(ColorTextDim)

	// 输入框提示符
	StyleInputPrompt = lipgloss.NewStyle().Foreground(ColorTextDim)

	// 权限标题
	StylePermTitle = lipgloss.NewStyle().Foreground(ColorPermission).Bold(true)

	// Diff
	StyleDiffAdd    = lipgloss.NewStyle().Foreground(ColorDiffAdded)
	StyleDiffDel    = lipgloss.NewStyle().Foreground(ColorDiffRemoved)
	StyleDiffHeader = lipgloss.NewStyle().Foreground(ColorTextDim).Bold(true)
	StyleDiffCtx    = lipgloss.NewStyle().Foreground(ColorTextDim)

	// Footer
	StyleFooter = lipgloss.NewStyle().Foreground(ColorTextDim)
)

// ContextColor 根据上下文使用百分比返回对应颜色
func ContextColor(percent float64) lipgloss.Color {
	switch {
	case percent >= 80:
		return ColorCtxHigh
	case percent >= 60:
		return ColorCtxMid
	default:
		return ColorCtxLow
	}
}
```

- [ ] **Step 2: 验证编译通过**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go build ./internal/tui/`
Expected: 编译错误（其他文件引用了被删除的样式名），这是预期的，后续 Task 会修复。记录所有编译错误。

---

### Task 2: spinner.go — CC 风格 Spinner

**Files:**
- Create: `internal/tui/spinner.go`

**CC 源码参考：**
- `src/components/Spinner/utils.ts:4-11` — Spinner 字符帧
- `src/constants/spinnerVerbs.ts` — 随机动词列表
- `src/components/Spinner.tsx:166-171` — Spinner 格式

- [ ] **Step 1: 创建 spinner.go**

```go
package tui

import (
	"fmt"
	"math/rand"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CC Spinner 帧（来自 Spinner/utils.ts:9 macOS）
// 原始: ['·', '✢', '✳', '✶', '✻', '✽']
// CC 做了 forward+reverse: [...chars, ...chars.reverse()]
var spinnerFrames = []string{"·", "✢", "✳", "✶", "✻", "✽", "✻", "✶", "✳", "✢"}

// CC Spinner 动词（来自 spinnerVerbs.ts，精选 40 个）
var spinnerVerbs = []string{
	"Thinking", "Reasoning", "Analyzing", "Crafting", "Computing",
	"Processing", "Generating", "Evaluating", "Composing", "Inferring",
	"Pondering", "Considering", "Deliberating", "Formulating", "Synthesizing",
	"Architecting", "Orchestrating", "Crystallizing", "Contemplating", "Brewing",
	"Cooking", "Hatching", "Forging", "Weaving", "Sculpting",
	"Assembling", "Calibrating", "Deciphering", "Distilling", "Harmonizing",
	"Iterating", "Mapping", "Navigating", "Optimizing", "Refining",
	"Shaping", "Transforming", "Unraveling", "Visualizing", "Working",
}

// SpinnerState 管理 CC 风格 Spinner 的状态
type SpinnerState struct {
	frame     int
	verb      string
	startTime time.Time
	active    bool
}

// NewSpinnerState 创建 Spinner 状态
func NewSpinnerState() SpinnerState {
	return SpinnerState{
		verb: spinnerVerbs[rand.Intn(len(spinnerVerbs))],
	}
}

// Start 开始计时并随机选择动词
func (s *SpinnerState) Start() {
	s.active = true
	s.startTime = time.Now()
	s.verb = spinnerVerbs[rand.Intn(len(spinnerVerbs))]
	s.frame = 0
}

// Stop 停止 Spinner
func (s *SpinnerState) Stop() {
	s.active = false
}

// Tick 推进帧
func (s *SpinnerState) Tick() {
	if s.active {
		s.frame = (s.frame + 1) % len(spinnerFrames)
	}
}

// View 渲染 Spinner 行
// CC 格式: [spinner_glyph] [verb]… [elapsed]
func (s SpinnerState) View() string {
	if !s.active {
		return ""
	}

	glyph := spinnerFrames[s.frame]
	elapsed := time.Since(s.startTime)

	// CC 品牌橙色渲染 spinner glyph
	coloredGlyph := lipgloss.NewStyle().Foreground(ColorClaude).Render(glyph)

	// 耗时格式
	elapsedStr := formatElapsed(elapsed)

	return fmt.Sprintf("  %s %s… %s", coloredGlyph, s.verb, StyleDim.Render(elapsedStr))
}

func formatElapsed(d time.Duration) string {
	secs := int(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	return fmt.Sprintf("%dm%ds", secs/60, secs%60)
}

// MsgSpinnerTick Spinner 定时 tick 消息
type MsgSpinnerTick struct{}

// SpinnerTickCmd 返回 50ms 定时的 tick 命令
func SpinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return MsgSpinnerTick{}
	})
}
```

- [ ] **Step 2: 验证编译**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go build ./internal/tui/`

---

### Task 3: chat.go — CC 消息渲染模式

**Files:**
- Rewrite: `internal/tui/chat.go`

**CC 源码参考：**
- `src/components/MessageResponse.tsx:22` — `⎿` 前缀模式（`"  " + ⎿ + "  "` = 6 字符，dimColor）
- `src/components/ToolUseLoader.tsx:19-20` — 工具状态指示器逻辑
- `src/components/messages/AssistantThinkingMessage.tsx:44` — Thinking 折叠态
- `src/components/messages/AssistantTextMessage.tsx` — AI 文本渲染
- `src/components/messages/AssistantToolUseMessage.tsx:186-228` — 工具调用行

核心渲染规则（直接从 CC 源码提取）：

```
用户消息:     ❯ 用户文本                        （无边框，纯文本）
AI 回复:      ⎿  Markdown内容                   （dim ⎿ 前缀，6字符缩进）
流式输出:     ⎿  累积文本                        （每次 delta 全量 Glamour 渲染）
Thinking:    ∴ Thinking                         （dim + italic，一行）
工具执行中:   ⏺ ToolName(args)                   （dim ⏺，闪烁）
工具完成:     ⏺ ToolName(args)                   （绿色 ⏺）
工具失败:     ⏺ ToolName(args)                   （红色 ⏺）
工具输出:       ⎿  输出内容                      （dim ⎿ 缩进，>8行折叠）
错误:         ⎿  错误文本                        （红色，⎿ 包裹）
```

- [ ] **Step 1: 重写 chat.go**

```go
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

const foldThreshold = 8

// === CC MessageResponse 前缀 ===
// 来自 MessageResponse.tsx:22: <Text dimColor>{"  "}⎿  </Text>
// 共 6 字符宽度: 2空格 + ⎿ + 2空格
const msgResponsePrefix = "  " + SymResponse + "  "

// msgResponseIndent 后续行的等宽空格缩进（与 ⎿ 前缀对齐）
var msgResponseIndent = strings.Repeat(" ", lipgloss.Width(msgResponsePrefix))

type ChatMessage struct {
	Role      string
	Content   string
	ToolName  string
	ToolID    string
	ToolInput string
	IsError   bool
	Folded    bool
}

type ChatView struct {
	viewport viewport.Model
	messages []ChatMessage
	width    int
	height   int

	streaming bool
	streamBuf string

	renderer *glamour.TermRenderer

	// 工具闪烁状态（配合 SpinnerTick）
	toolBlink bool
}

func NewChatView(width, height int) ChatView {
	vp := viewport.New(width, height)
	vp.Style = lipgloss.NewStyle()
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 3

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(maxInt(20, width-8)),
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
			glamour.WithWordWrap(maxInt(20, msg.Width-8)),
		)
		c.refreshContent(c.shouldAutoScroll())

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
			Content:   "",
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

	case MsgError:
		stick := c.shouldAutoScroll()
		c.messages = append(c.messages, ChatMessage{
			Role:    "error",
			Content: msg.Err.Error(),
		})
		c.refreshContent(stick)

	case MsgSpinnerTick:
		// 切换工具闪烁状态（模拟 CC 的 useBlink）
		c.toolBlink = !c.toolBlink
		c.refreshContent(c.shouldAutoScroll())
	}

	c.viewport, cmd = c.viewport.Update(msg)
	return c, cmd
}

func (c ChatView) View() string {
	return c.viewport.View()
}

func (c ChatView) shouldAutoScroll() bool {
	return c.viewport.AtBottom() || c.viewport.TotalLineCount() == 0
}

// refreshContent 重新渲染所有消息
func (c *ChatView) refreshContent(stickToBottom bool) {
	var sb strings.Builder

	for i, msg := range c.messages {
		if i > 0 {
			sb.WriteString("\n")
		}
		rendered := c.renderMessage(msg)
		if rendered != "" {
			sb.WriteString(rendered)
		}
	}

	// 流式接收中的 AI 回复
	if c.streaming && c.streamBuf != "" {
		if len(c.messages) > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(c.renderStreamingMessage())
	}

	c.viewport.SetContent(sb.String())
	if stickToBottom {
		c.viewport.GotoBottom()
	}
}

func (c *ChatView) renderMessage(msg ChatMessage) string {
	switch msg.Role {
	case "user":
		// CC: UserPromptMessage — 纯文本，无装饰
		return StyleUserText.Render(SymUserPrompt + " " + msg.Content)

	case "assistant":
		// CC: AssistantTextMessage — MessageResponse 包裹 + Markdown
		body := msg.Content
		if c.renderer != nil {
			if rendered, err := c.renderer.Render(body); err == nil {
				body = strings.TrimSpace(rendered)
			}
		}
		return wrapMessageResponse(body)

	case "thinking":
		// CC: AssistantThinkingMessage.tsx:44
		// 折叠态: <Text dimColor italic>∴ Thinking</Text>
		return StyleThinking.Render(SymThinking + " Thinking")

	case "tool":
		return c.renderToolMessage(msg)

	case "system":
		return StyleDim.Render(msg.Content)

	case "error":
		// CC: 错误也用 MessageResponse 包裹
		return wrapMessageResponse(StyleError.Render(msg.Content))

	default:
		return msg.Content
	}
}

func (c *ChatView) renderStreamingMessage() string {
	body := c.streamBuf
	// 流式 Markdown：每次 delta 全量 Glamour 渲染
	if c.renderer != nil {
		if rendered, err := c.renderer.Render(body); err == nil {
			body = strings.TrimSpace(rendered)
		}
	}
	return wrapMessageResponse(body)
}

func (c *ChatView) renderToolMessage(msg ChatMessage) string {
	circle := BlackCircle()
	argPreview := toolArgPreview(msg.ToolName, msg.ToolInput)

	// 工具头部行: ⏺ ToolName(args)
	// CC: AssistantToolUseMessage.tsx:186-228
	toolLabel := StyleToolName.Render(msg.ToolName)
	if argPreview != "" {
		toolLabel += StyleDim.Render("(" + argPreview + ")")
	}

	isInProgress := (msg.Content == "")

	if isInProgress {
		// CC: ToolUseLoader — dim ⏺ 闪烁
		// 来自 ToolUseLoader.tsx:20: 交替显示 BLACK_CIRCLE 和 " "
		indicator := " "
		if c.toolBlink {
			indicator = circle
		}
		return StyleDim.Render(indicator) + " " + toolLabel
	}

	// 已完成
	var indicatorStyle lipgloss.Style
	if msg.IsError {
		// CC: isError → color="error"
		indicatorStyle = lipgloss.NewStyle().Foreground(ColorError)
	} else {
		// CC: success → color="success"
		indicatorStyle = lipgloss.NewStyle().Foreground(ColorSuccess)
	}
	header := indicatorStyle.Render(circle) + " " + toolLabel

	// 无输出
	if msg.Content == "" {
		return header
	}

	// 工具输出用 ⎿ 包裹（CC: UserToolSuccessMessage 用 MessageResponse）
	lines := strings.Split(msg.Content, "\n")
	lineCount := len(lines)
	outputLines := lines

	if msg.Folded || lineCount > foldThreshold {
		previewEnd := 3
		if previewEnd > lineCount {
			previewEnd = lineCount
		}
		outputLines = lines[:previewEnd]
	}

	output := StyleToolOutput.Render(strings.Join(outputLines, "\n"))
	body := header + "\n" + wrapMessageResponse(output)

	if len(outputLines) < lineCount {
		remaining := lineCount - len(outputLines)
		body += "\n" + StyleDim.Render(msgResponseIndent+"… +" + fmt.Sprintf("%d lines", remaining))
	}

	return body
}

// wrapMessageResponse 实现 CC 的 MessageResponse 组件
// CC 源码 MessageResponse.tsx:22:
//   <Text dimColor>{"  "}⎿  </Text>
//   后续内容 flex-grow 填充
func wrapMessageResponse(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return StyleMsgResponse.Render(msgResponsePrefix)
	}

	lines := strings.Split(content, "\n")
	// 第一行加 ⎿ 前缀
	lines[0] = StyleMsgResponse.Render(msgResponsePrefix) + lines[0]
	// 后续行等宽缩进
	for i := 1; i < len(lines); i++ {
		lines[i] = msgResponseIndent + lines[i]
	}
	return strings.Join(lines, "\n")
}

// toolArgPreview 从工具输入 JSON 中提取关键参数
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
		"Bash": "command", "Read": "path", "Write": "path",
		"Edit": "path", "Glob": "pattern", "Grep": "pattern",
		"WebFetch": "url", "WebSearch": "query", "AskUser": "question",
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

func (c *ChatView) AddSystemMessage(text string) {
	c.messages = append(c.messages, ChatMessage{Role: "system", Content: text})
	c.refreshContent(true)
}

func (c *ChatView) Clear() {
	c.messages = nil
	c.streamBuf = ""
	c.streaming = false
	c.refreshContent(true)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

- [ ] **Step 2: 验证编译**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go build ./internal/tui/`

---

### Task 4: app.go — 布局重构

**Files:**
- Modify: `internal/tui/app.go`

**改动要点：**
1. 删除 StatusBar 组件引用
2. 删除 renderTopBar / renderSidebar / renderHomeDashboard 等
3. 用自定义 SpinnerState 替换 bubbles spinner
4. 简化 Footer 为单行
5. Spinner Tick 驱动工具闪烁 + Spinner 动画
6. 布局：消息列表 + Spinner 行 + 输入框 + Footer 行

- [ ] **Step 1: 重写 app.go 的结构体和 NewApp**

删除 `statusBar`、`workspacePeek`、`toolPeek` 等字段。用 `SpinnerState` 替代 `spinner.Model`。简化欢迎消息。

关键变更点：

```go
// App 结构体中：
// 删除: statusBar StatusBar, workspacePeek, toolPeek
// 替换: spinner spinner.Model → ccSpinner SpinnerState
// 保留: chat, input, permission, diff, slashHandler, state, 通信 channel, 配置

// NewApp 中：
// 删除: NewStatusBar、readWorkspacePeek、renderHomeDashboard
// 简化欢迎消息为极简风格
// 初始化 ccSpinner: NewSpinnerState()
```

- [ ] **Step 2: 重写 Init() 和 Update()**

Init 中用 `SpinnerTickCmd()` 替代 `spinner.Tick`。

Update 中：
- 删除 `spinner.TickMsg` 处理
- 新增 `MsgSpinnerTick` 处理：推进 SpinnerState.Tick()，转发给 chat（驱动工具闪烁），继续 tick
- StateQuery/StateToolExec 时 Spinner 活跃，StateInput 时停止

- [ ] **Step 3: 重写 View() 和布局方法**

```
布局结构（从上到下）：
[消息列表 — 占满剩余高度]
[Spinner 行 — 仅在 Query/ToolExec 时显示，1行]
[输入框]
[Footer 行 — 1行：模型 · 费用 · 上下文% · /help]
```

renderComposer 简化：
```go
func (a *App) renderComposer() string {
    var parts []string
    // Spinner 行
    if a.state == StateQuery || a.state == StateToolExec {
        parts = append(parts, a.ccSpinner.View())
    }
    // 输入框
    parts = append(parts, a.input.View())
    // Footer 单行
    parts = append(parts, a.renderFooter())
    return strings.Join(parts, "\n")
}

func (a *App) renderFooter() string {
    // CC 风格：极简单行
    // model · $cost · context% · /help
    model := shortModelName(a.model)
    cost := a.tracker.CostString()
    ctx := fmt.Sprintf("%d%%", a.contextPercent())
    return StyleFooter.Render(fmt.Sprintf("%s · %s · %s context · /help", model, cost, ctx))
}
```

- [ ] **Step 4: 删除不再需要的方法**

删除以下方法/函数：
- `renderTopBar`、`renderBody`、`renderChatPanel`、`renderSidebar`、`renderSidebarSection`
- `renderHomeDashboard`、`readWorkspacePeek`
- `renderInfoCard`、`renderFooterStatus`、`stateFooter`、`stateChip`、`stateLabel`、`stateHelp`
- `contextBar`、`bodyHeight`、`sidebarWidth`、`chatPanelWidth`
- `fillLine`、`fillVertical`、`innerWidth`
- 所有 `Style*` 引用替换为 Task 1 中的新样式

- [ ] **Step 5: 验证编译**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go build ./internal/tui/`

---

### Task 5: input.go — CC 风格输入框

**Files:**
- Modify: `internal/tui/input.go`

- [ ] **Step 1: 修改 NewInputBox**

```go
// 改动点：
ta.Placeholder = "Send a message..."  // CC 风格占位符
ta.Prompt = StyleInputPrompt.Render("> ")  // CC 用 > 作为输入提示
// 删除 FocusedStyle 的自定义颜色设置（用默认即可）
```

- [ ] **Step 2: 简化 renderSlashHint**

删除复杂的 StylePromptOverlay/StyleComposerSlash，用简单的 dim 文本显示匹配命令。

- [ ] **Step 3: 清理不再存在的样式引用**

替换所有 `StylePanelTitle`、`StyleComposerSlash`、`StylePanelMeta`、`StylePromptOverlay` 为 `StyleDim`。

- [ ] **Step 4: 验证编译**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go build ./internal/tui/`

---

### Task 6: permission.go + diff.go — 清理样式引用

**Files:**
- Modify: `internal/tui/permission.go`
- Modify: `internal/tui/diff.go`

- [ ] **Step 1: 更新 permission.go 样式引用**

替换：
- `StylePermTitle` → 保留（Task 1 已定义）
- `StyleToolName` → 保留
- `StylePanelMeta` → `StyleDim`
- `StyleToolOutput` → `StyleToolOutput`（保留）
- `StyleHint` → `StyleDim`
- `StylePermissionStrip` → 简化为带左边框的样式，颜色用 `ColorPermission`

- [ ] **Step 2: 更新 diff.go 样式引用**

替换：
- `StyleDiffHeader`、`StyleDiffAdd`、`StyleDiffDel`、`StyleDiffCtx` → 保留（Task 1 已定义）
- `StylePanelMeta` → `StyleDim`
- `StyleSectionCard` → 简化

- [ ] **Step 3: 验证编译**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go build ./internal/tui/`

---

### Task 7: 删除 statusbar.go + 全局编译验证

**Files:**
- Delete: `internal/tui/statusbar.go`

- [ ] **Step 1: 删除 statusbar.go**

```bash
rm /Users/ocean/Studio/01-workshop/06-开源项目/xin-code/internal/tui/statusbar.go
```

- [ ] **Step 2: 清理 app.go 中的 StatusBar 引用**

确认 app.go 中已移除所有 `StatusBar` / `NewStatusBar` / `statusBar` 引用。

- [ ] **Step 3: 全量编译验证**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go build -o /dev/null .`
Expected: BUILD SUCCESS

- [ ] **Step 4: 构建可执行文件并测试**

```bash
cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code
go build -ldflags "-X main.Version=dev -X main.Commit=local -X main.Date=$(date -u +%Y%m%d)" -o xin-code .
```

---

### Task 8: 视觉验证 + 修复

- [ ] **Step 1: 运行并截图对比**

启动 xin-code，发送测试消息，对比 CC 的实际输出：
- 用户消息：应显示 `❯ 你好`（无边框）
- AI 回复：应显示 `  ⎿  回复内容`（dim ⎿ 前缀 + Markdown 格式化）
- Thinking：应显示 `∴ Thinking`（dim italic）
- 工具执行中：应显示 `⏺ Bash(command)`（dim ⏺ 闪烁）
- 工具完成：应显示 `⏺ Bash(command)`（绿色 ⏺）+ `  ⎿  输出`
- 错误：应显示 `  ⎿  错误文本`（红色）
- Spinner：应显示 `  ✶ Thinking… 3s`（品牌橙色）
- Footer：应显示 `claude-sonnet-4-6 · ¥0.00 · 0% context · /help`

- [ ] **Step 2: 修复发现的视觉问题**

根据实际运行效果逐个修复差异。

---

## CC → Xin Code 渲染规则速查表

| CC 组件 | CC 源码位置 | 渲染规则 | Xin Code 实现 |
|---------|------------|----------|--------------|
| MessageResponse | `MessageResponse.tsx:22` | `"  ⎿  "` dimColor 前缀 | `wrapMessageResponse()` |
| ToolUseLoader | `ToolUseLoader.tsx:19-20` | dim ⏺ 闪烁 / 绿 ⏺ / 红 ⏺ | `renderToolMessage()` + `toolBlink` |
| AssistantThinking | `AssistantThinkingMessage.tsx:44` | `∴ Thinking` dim italic | `StyleThinking.Render("∴ Thinking")` |
| AssistantText | `AssistantTextMessage.tsx` | MessageResponse + Markdown | `wrapMessageResponse(glamour(text))` |
| UserPrompt | `UserPromptMessage.tsx` | 纯文本 | `❯ text` |
| Spinner | `Spinner/utils.ts:9` | 花型帧 + 动词 + 耗时 | `SpinnerState.View()` |
| Theme | `theme.ts:443` | claude: rgb(215,119,87) | `ColorClaude = "#D77757"` |
