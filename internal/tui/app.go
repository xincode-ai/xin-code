package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/xincode-ai/xin-code/internal/cost"
	"github.com/xincode-ai/xin-code/internal/slash"
)

// AppState TUI 状态机
type AppState int

const (
	StateInput        AppState = iota // 等待用户输入
	StateQuery                        // 等待模型响应
	StateToolExec                     // 工具执行中
	StatePermission                   // 等待权限确认
	StateDiffPreview                  // 等待差异确认
	StateAskUser                      // 询问工具等待用户输入
	StateResumeSelect                 // 会话恢复选择器
)

// EventSender 模型代理向界面发送事件的回调接口
type EventSender func(tea.Msg)

// App TUI 主 Model
type App struct {
	// 布局管理器
	layout Layout

	// 子组件
	chat           ChatView
	input          InputBox
	permission     PermissionDialog
	diff           DiffDialog
	ccSpinner      SpinnerState
	resumeSelector ResumeSelector

	// 斜杠命令
	slashHandler *slash.Handler

	// 状态
	state    AppState
	width    int
	height   int
	quitting bool

	// 模型代理通信
	eventCh         chan tea.Msg // 模型代理协程 -> 终端界面
	submitCh        chan string  // 终端界面 -> 外部模型代理（用户输入）
	askResponseCh   chan string  // 询问工具回传通道
	waitingForEvent bool        // 防止重复注册 waitForEvent

	// 配置
	model    string
	provider string
	tracker  *cost.Tracker
	maxContext int
	version    string
	toolCount  int
	permMode   string
	workDir    string

	// 会话回调（由 main.go 注入）
	OnClear       func()
	OnCompact     func() string
	OnModelSwitch func(string)
	OnExport      func() string
	OnInterrupt   func()
	OnResume      func() string
	OnThemeSwitch func(string) string
	OnLogin       func() string
	OnLogout      func() string
	OnSkillsList  func() string
	OnPluginsList func() string
	OnHooksList   func() string

	// 会话信息
	SessionID    string
	SessionTurns int

	// 临时通知（slash 命令显示类结果，不写入 transcript）
	ephemeralNotice    string
	ephemeralNoticeAge int // tick 计数器，到达阈值后清除
}

// AppConfig TUI 初始化配置
type AppConfig struct {
	Model      string
	Provider   string
	Tracker    *cost.Tracker
	MaxContext int
	Version    string
	ToolCount  int
	PermMode   string
	WorkDir    string
}

// NewApp 创建 TUI 应用
func NewApp(cfg AppConfig) *App {
	slashH := slash.NewHandler()
	chat := NewChatView(80, 20)

	// 极简欢迎消息（CC 风格）
	chat.AddSystemMessage(renderWelcomeBanner(cfg))

	return &App{
		chat:           chat,
		input:          NewInputBox(commandHintsFromSlash(slashH.AllCommands())),
		permission:     NewPermissionDialog(),
		diff:           NewDiffDialog(),
		ccSpinner:      NewSpinnerState(),
		resumeSelector: NewResumeSelector(),
		slashHandler:   slashH,
		state:        StateInput,
		eventCh:      make(chan tea.Msg, 512),
		submitCh:     make(chan string, 8),
		model:        cfg.Model,
		provider:     cfg.Provider,
		tracker:      cfg.Tracker,
		maxContext:   cfg.MaxContext,
		version:      cfg.Version,
		toolCount:    cfg.ToolCount,
		permMode:     cfg.PermMode,
		workDir:      cfg.WorkDir,
	}
}

// SubmitCh 返回用户提交消息的通道（外部模型代理读取）
func (a *App) SubmitCh() <-chan string {
	return a.submitCh
}

// Send 向终端界面发送消息（模型代理协程调用）
func (a *App) Send(msg tea.Msg) {
	a.eventCh <- msg
}

// waitForEvent 等待来自 Agent 的事件
func waitForEvent(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return MsgAgentDone{}
		}
		return msg
	}
}

// safeWaitForEvent 带防重复保护的 waitForEvent
func (a *App) safeWaitForEvent() tea.Cmd {
	if a.waitingForEvent {
		return nil
	}
	a.waitingForEvent = true
	return func() tea.Msg {
		msg, ok := <-a.eventCh
		if !ok {
			return MsgAgentDone{}
		}
		return msg
	}
}

func (a *App) Init() tea.Cmd {
	a.waitingForEvent = true
	return tea.Batch(
		a.input.Init(),
		SpinnerTickCmd(),
		waitForEvent(a.eventCh),
	)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg.(type) {
	case MsgTextDelta, MsgThinking, MsgToolStart, MsgToolDone,
		MsgUsage, MsgResponseDone, MsgAgentDone,
		MsgPermissionRequest, MsgDiffPreview, MsgAskUser, MsgError,
		MsgSystemNotice, MsgSubAgentStart, MsgSubAgentDone:
		a.waitingForEvent = false
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.resizeLayout()
		return a, nil

	case tea.MouseMsg:
		// 鼠标滚轮事件转发给 chat viewport 处理滚动
		a.chat, _ = a.chat.Update(msg)
		return a, nil

	case tea.KeyMsg:
		// 任意按键清除临时通知
		if a.ephemeralNotice != "" {
			a.clearEphemeralNotice()
		}

		if a.permission.IsVisible() {
			a.permission, _ = a.permission.Update(msg)
			if !a.permission.IsVisible() {
				a.state = StateToolExec
				cmds = append(cmds, a.safeWaitForEvent())
			}
			return a, tea.Batch(cmds...)
		}

		if a.diff.IsVisible() {
			a.diff, _ = a.diff.Update(msg)
			if !a.diff.IsVisible() {
				a.state = StateToolExec
				cmds = append(cmds, a.safeWaitForEvent())
			}
			return a, tea.Batch(cmds...)
		}

		// 会话恢复选择器拦截键盘事件
		if a.resumeSelector.IsVisible() {
			a.resumeSelector, _ = a.resumeSelector.Update(msg)
			if !a.resumeSelector.IsVisible() {
				a.state = StateInput
				cmds = append(cmds, a.input.Focus())
			}
			return a, tea.Batch(cmds...)
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			if a.state == StateQuery || a.state == StateToolExec {
				if a.OnInterrupt != nil {
					a.OnInterrupt()
				}
				a.state = StateInput
				a.ccSpinner.Stop()
				a.chat.AddSystemMessage("已请求中断当前操作。")
				return a, a.input.Focus()
			}
			a.quitting = true
			return a, tea.Quit
		case tea.KeyCtrlL:
			a.chat.Clear()
			return a, nil
		}

		if a.shouldRouteKeyToChat(msg) {
			a.chat, _ = a.chat.Update(msg)
			return a, nil
		}

		if a.state == StateAskUser {
			if msg.Type == tea.KeyEnter {
				answer := strings.TrimSpace(a.input.Value())
				if answer != "" {
					responseCh := a.askResponseCh
					a.askResponseCh = nil
					a.input.Reset()
					a.state = StateToolExec
					cmd := func() tea.Msg {
						if responseCh != nil {
							responseCh <- answer
						}
						return nil
					}
					cmds = append(cmds, cmd, a.safeWaitForEvent())
					a.resizeLayout()
					return a, tea.Batch(cmds...)
				}
			}
			a.input, _ = a.input.Update(msg)
			a.resizeLayout()
			return a, nil
		}

	case MsgSubmit:
		text := msg.Text
		if strings.HasPrefix(text, "/") {
			model, cmd := a.handleSlashCommand(text)
			a.resizeLayout()
			return model, cmd
		}

		a.chat, _ = a.chat.Update(msg)
		a.state = StateQuery
		submitCmd := func() tea.Msg {
			a.submitCh <- text
			return nil
		}
		cmds = append(cmds, submitCmd, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgTextDelta:
		a.state = StateQuery
		if !a.ccSpinner.active {
			a.ccSpinner.Start()
		}
		a.chat, _ = a.chat.Update(msg)
		cmds = append(cmds, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgThinking:
		a.chat, _ = a.chat.Update(msg)
		cmds = append(cmds, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgToolStart:
		a.state = StateToolExec
		if !a.ccSpinner.active {
			a.ccSpinner.Start()
		}
		a.chat, _ = a.chat.Update(msg)
		cmds = append(cmds, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgToolDone:
		a.chat, _ = a.chat.Update(msg)
		cmds = append(cmds, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgUsage:
		if msg.Usage != nil {
			a.tracker.AddUsage(
				msg.Usage.InputTokens,
				msg.Usage.OutputTokens,
				msg.Usage.CacheCreationInputTokens,
				msg.Usage.CacheReadInputTokens,
			)
		}
		cmds = append(cmds, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgResponseDone:
		a.chat, _ = a.chat.Update(msg)
		cmds = append(cmds, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgAgentDone:
		a.state = StateInput
		a.ccSpinner.ClearSubAgents()
		a.ccSpinner.Stop()
		if msg.Err != nil {
			a.chat, _ = a.chat.Update(MsgError{Err: msg.Err})
		}
		cmds = append(cmds, a.input.Focus())
		return a, tea.Batch(cmds...)

	case MsgPermissionRequest:
		a.state = StatePermission
		a.permission.Show(msg.ToolName, msg.Input, msg.Response)
		a.input.Blur()
		return a, nil

	case MsgDiffPreview:
		a.state = StateDiffPreview
		a.diff.Show(msg.Path, msg.DiffText, msg.Response)
		a.input.Blur()
		return a, nil

	case MsgAskUser:
		a.state = StateAskUser
		a.askResponseCh = msg.Response
		a.chat.AddSystemMessage("需要你补充输入：\n" + msg.Question)
		cmds = append(cmds, a.input.Focus())
		return a, tea.Batch(cmds...)

	case MsgSubAgentStart:
		a.ccSpinner.AddSubAgent(msg.ID, msg.Description)
		a.chat, _ = a.chat.Update(msg)
		cmds = append(cmds, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgSubAgentDone:
		a.ccSpinner.CompleteSubAgent(msg.ID)
		a.chat, _ = a.chat.Update(msg)
		cmds = append(cmds, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgSystemNotice:
		a.chat.AddSystemMessage(msg.Text)
		cmds = append(cmds, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgError:
		a.chat, _ = a.chat.Update(msg)
		a.state = StateInput
		a.ccSpinner.Stop()
		cmds = append(cmds, a.input.Focus())
		return a, tea.Batch(cmds...)

	case MsgSpinnerTick:
		a.ccSpinner.Tick()
		a.chat, _ = a.chat.Update(msg) // 转发给 chat 驱动工具闪烁
		// 临时通知老化
		if a.ephemeralNotice != "" {
			a.ephemeralNoticeAge++
			if a.ephemeralNoticeAge >= ephemeralNoticeTTL {
				a.clearEphemeralNotice()
			}
		}
		cmds = append(cmds, SpinnerTickCmd())
		return a, tea.Batch(cmds...)
	}

	if a.state == StateInput || a.state == StateAskUser {
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		a.resizeLayout()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return a, tea.Batch(cmds...)
}

func (a *App) View() string {
	if a.quitting {
		return StyleDim.Render("Goodbye.\n")
	}
	if a.width == 0 {
		return ""
	}

	contentWidth := a.width - 2
	if contentWidth < 40 {
		contentWidth = a.width
	}

	var content LayerContent

	// Modal 层：diff 预览独占
	if a.diff.IsVisible() {
		content.Modal = a.diff.View()
		return a.layout.Render(content)
	}

	// Overlay 层：选择器等覆盖组件
	if a.resumeSelector.IsVisible() {
		content.Overlay = a.resumeSelector.View()
	}

	// BottomFloat 层：临时通知 / spinner / 权限确认
	var floatParts []string
	if a.ephemeralNotice != "" {
		// 临时通知渲染（带左侧边框，与 slash hints 风格统一）
		noticeWidth := min(80, max(40, contentWidth-4))
		notice := lipgloss.NewStyle().
			BorderLeft(true).
			BorderForeground(ColorBrand).
			PaddingLeft(1).
			Width(noticeWidth).
			Render(StyleDim.Render(a.ephemeralNotice))
		floatParts = append(floatParts, notice)
	}
	if spinnerView := a.ccSpinner.View(); spinnerView != "" {
		floatParts = append(floatParts, spinnerView)
	}
	if a.permission.IsVisible() {
		cardWidth := min(88, max(48, contentWidth-4))
		floatParts = append(floatParts, a.permission.Card(cardWidth))
	}
	if len(floatParts) > 0 {
		content.BottomFloat = strings.Join(floatParts, "\n")
	}

	// Bottom 层：输入框 + 状态栏
	inputView := a.renderInput(contentWidth)
	statusView := a.renderStatusLine()
	content.Bottom = inputView + "\n" + statusView

	// 主滚动区：transcript（占满剩余高度）
	chatHeight := a.layout.MainHeight(content.Bottom, content.BottomFloat, content.Overlay)
	a.chat, _ = a.chat.Update(tea.WindowSizeMsg{Width: contentWidth, Height: chatHeight})
	content.Main = a.chat.View()

	return lipgloss.NewStyle().Padding(0, 1).Render(a.layout.Render(content))
}

// renderInput 渲染输入区（仅上边框，CC 风格）
func (a *App) renderInput(contentWidth int) string {
	inputBorder := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderTop(true).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderForeground(ColorInputBorder).
		Width(contentWidth)
	return inputBorder.Render(a.input.View())
}

func (a *App) handleSlashCommand(cmd string) (tea.Model, tea.Cmd) {
	ctx := &slash.Context{
		Model:               a.model,
		Provider:            a.provider,
		Version:             a.version,
		PermMode:            a.permMode,
		Currency:            a.tracker.Currency(),
		MaxContext:          a.maxContext,
		InputTokens:         a.tracker.InputTokens(),
		OutputTokens:        a.tracker.OutputTokens(),
		TotalTokens:         a.tracker.TotalTokens(),
		CostString:          a.tracker.CostString(),
		CostUSD:             a.tracker.TotalCostUSD(),
		SessionID:           a.SessionID,
		SessionTurns:        a.SessionTurns,
		WorkDir:             a.workDir,
		CacheCreationTokens: a.tracker.CacheCreationTokens(),
		CacheReadTokens:     a.tracker.CacheReadTokens(),
		OnClear:             a.OnClear,
		OnCompact:           a.OnCompact,
		OnModelSwitch:       a.OnModelSwitch,
		OnExport:            a.OnExport,
		OnResume:            a.OnResume,
		OnLogin:             a.OnLogin,
		OnLogout:            a.OnLogout,
		OnSkillsList:        a.OnSkillsList,
		OnPluginsList:       a.OnPluginsList,
		OnHooksList:         a.OnHooksList,
		OnThemeSwitch: func(mode string) string {
			switch mode {
			case "light":
				SetTheme(ThemeLight)
			default:
				SetTheme(ThemeDark)
			}
			// 清空 Markdown 缓存（颜色变了，需要重新渲染）
			a.chat.mdCache = make(map[string]string)
			a.chat.refreshContent(a.chat.shouldAutoScroll())
			return fmt.Sprintf("已切换到 %s 主题", mode)
		},
	}

	result, handled := a.slashHandler.Handle(cmd, ctx)
	if !handled {
		return a, nil
	}

	switch result.Type {
	case slash.ResultAction:
		switch result.Content {
		case "quit":
			a.quitting = true
			return a, tea.Quit
		case "clear":
			a.chat.Clear()
			return a, nil
		}

	case slash.ResultPrompt:
		a.chat.AddSystemMessage(fmt.Sprintf("执行命令 %s", cmd))
		a.state = StateQuery
		submitCmd := func() tea.Msg {
			a.submitCh <- result.Content
			return nil
		}
		return a, tea.Batch(submitCmd, a.safeWaitForEvent())

	case slash.ResultDisplay:
		// 纯信息展示类命令不写入 transcript，显示为临时通知
		a.showEphemeralNotice(result.Content)
	}

	return a, nil
}

func (a *App) resizeLayout() {
	if a.width == 0 || a.height == 0 {
		return
	}
	// 同步布局管理器尺寸
	a.layout.width = a.width
	a.layout.height = a.height

	contentWidth := a.width - 2
	if contentWidth < 40 {
		contentWidth = a.width
	}
	// 输入框无左右边框，只需减去少量余量
	a.input, _ = a.input.Update(tea.WindowSizeMsg{Width: contentWidth - 2})
	a.permission, _ = a.permission.Update(tea.WindowSizeMsg{Width: contentWidth, Height: a.height})
	a.diff, _ = a.diff.Update(tea.WindowSizeMsg{Width: contentWidth, Height: a.height})
	a.resumeSelector, _ = a.resumeSelector.Update(tea.WindowSizeMsg{Width: contentWidth, Height: a.height})
}


// ephemeralNoticeTTL 临时通知存活的 tick 数（80ms/tick × 125 ≈ 10秒）
const ephemeralNoticeTTL = 125

// showEphemeralNotice 显示临时通知（不写入 transcript）
func (a *App) showEphemeralNotice(content string) {
	a.ephemeralNotice = content
	a.ephemeralNoticeAge = 0
}

// clearEphemeralNotice 清除临时通知
func (a *App) clearEphemeralNotice() {
	a.ephemeralNotice = ""
	a.ephemeralNoticeAge = 0
}

func (a *App) shouldRouteKeyToChat(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "pgup", "pgdown", "home", "end":
		return true
	default:
		return false
	}
}

func (a *App) contextPercent() int {
	if a.maxContext <= 0 {
		return 0
	}
	used := a.tracker.TotalTokens()
	percent := float64(used) / float64(a.maxContext) * 100
	if percent > 100 {
		percent = 100
	}
	return int(percent + 0.5)
}

func (a *App) contentWidth() int {
	width := a.width - 2
	if width < 40 {
		width = a.width
	}
	return width
}

func permissionLabel(mode string) string {
	switch mode {
	case "default":
		return "默认确认"
	case "acceptEdits":
		return "自动接受编辑"
	case "plan":
		return "仅规划"
	case "bypassPermissions":
		return "完全放行"
	case "interactive":
		return "交互确认"
	default:
		if mode == "" {
			return "未设置"
		}
		return mode
	}
}

func shortModelName(model string) string {
	parts := strings.Split(model, "-")
	if len(parts) >= 5 {
		last := parts[len(parts)-1]
		if len(last) == 8 && strings.Trim(last, "0123456789") == "" {
			return strings.Join(parts[:len(parts)-1], "-")
		}
	}
	return truncateText(model, 18)
}

func commandHintsFromSlash(commands []*slash.Command) []CommandHint {
	hints := make([]CommandHint, 0, len(commands))
	for _, cmd := range commands {
		hints = append(hints, CommandHint{
			Name:        cmd.Name,
			Description: cmd.Description,
		})
	}
	return hints
}

// renderWelcomeBanner 渲染精简欢迎横幅（ASCII art + 核心信息）
func renderWelcomeBanner(cfg AppConfig) string {
	art := []string{
		"  ▀▄ ▄▀",
		"    █  ",
		"  ▄▀ ▀▄",
	}

	orange := lipgloss.NewStyle().Foreground(ColorBrand)
	bold := lipgloss.NewStyle().Foreground(ColorText).Bold(true)
	dim := StyleDim

	// 右侧信息：产品名+版本、模型、工作目录（去掉权限模式，状态栏已有）
	info := []string{
		bold.Render("Xin Code") + "  " + dim.Render("v"+cfg.Version),
		dim.Render(shortModelName(cfg.Model)),
		dim.Render(cfg.WorkDir),
	}

	var lines []string
	lines = append(lines, "")
	for i := 0; i < len(art); i++ {
		right := ""
		if i < len(info) {
			right = info[i]
		}
		lines = append(lines, orange.Render(art[i])+"    "+right)
	}

	return strings.Join(lines, "\n")
}

func truncateText(s string, limit int) string {
	if limit <= 0 || lipgloss.Width(s) <= limit {
		return s
	}
	runes := []rune(s)
	if len(runes) <= 1 {
		return s
	}
	if len(runes) > limit-1 {
		runes = runes[:limit-1]
	}
	return string(runes) + "…"
}
