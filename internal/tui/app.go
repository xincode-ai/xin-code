package tui

import (
	"encoding/json"
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
	StatePanel                        // 可滚动文本面板
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
	panel          Panel
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

	// 鼠标模式
	mouseMode     MouseMode // 当前鼠标模式（默认 Browse）
	mouseModePrev MouseMode // modal 打开前的模式（关闭后恢复）

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
	OnResume      func() ([]ResumeEntry, error)
	OnResumeLoad  func(sessionID string) error
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

	// AskUser 问题文本（bottomFloat 显示，不写入 transcript）
	askQuestion string

	// 只读浏览模式（/resume 恢复不兼容会话时启用）
	readOnly       bool
	readOnlyReason string
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
		panel:          NewPanel(),
		ccSpinner:      NewSpinnerState(),
		resumeSelector: NewResumeSelector(),
		slashHandler:   slashH,
		state:        StateInput,
		mouseMode:    MouseModeBrowse,
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
		// 记录鼠标事件时间戳（用于 input 抑制转义碎片）
		a.input.RecordMouseEvent()
		// 鼠标事件按可见层级分发：最上层优先
		if a.panel.IsVisible() {
			a.panel, _ = a.panel.Update(msg)
			return a, nil
		}
		if a.diff.IsVisible() {
			a.diff, _ = a.diff.Update(msg)
			return a, nil
		}
		if a.resumeSelector.IsVisible() {
			a.resumeSelector, _ = a.resumeSelector.Update(msg)
			return a, nil
		}
		// 无 modal/overlay 时，转发给 chat viewport
		a.chat, _ = a.chat.Update(msg)
		return a, nil

	case tea.KeyMsg:
		// 任意按键清除临时通知
		if a.ephemeralNotice != "" {
			a.clearEphemeralNotice()
		}

		// Panel 拦截键盘事件（最高 modal 层级）
		if a.panel.IsVisible() {
			a.panel, _ = a.panel.Update(msg)
			if !a.panel.IsVisible() {
				a.state = StateInput
				cmds = append(cmds, a.input.Focus(), a.leaveModalMouseMode())
			}
			return a, tea.Batch(cmds...)
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
				cmds = append(cmds, a.safeWaitForEvent(), a.leaveModalMouseMode())
			}
			return a, tea.Batch(cmds...)
		}

		// 会话恢复选择器拦截键盘事件
		if a.resumeSelector.IsVisible() {
			wasVisible := a.resumeSelector.IsVisible()
			selectedBefore := a.resumeSelector.SelectedEntry()
			a.resumeSelector, _ = a.resumeSelector.Update(msg)
			if wasVisible && !a.resumeSelector.IsVisible() {
				// 选择器关闭
				if selectedBefore != nil && msg.Type == tea.KeyEnter {
					switch selectedBefore.Mode {
					case ResumeBlocked:
						// 不兼容：拒绝恢复，显示原因
						a.showEphemeralNotice(selectedBefore.ModeReason)
					case ResumeReadonly:
						// 只读浏览：加载 transcript 但禁止发送
						if a.OnResumeLoad != nil {
							if err := a.OnResumeLoad(selectedBefore.ID); err != nil {
								a.showEphemeralNotice("恢复会话失败: " + err.Error())
							} else {
								a.readOnly = true
								a.readOnlyReason = ReasonReadonlyNotice
								a.showEphemeralNotice(ReasonReadonlyNotice)
							}
						}
					default: // ResumeContinue
						// 完全恢复：可继续对话
						if a.OnResumeLoad != nil {
							if err := a.OnResumeLoad(selectedBefore.ID); err != nil {
								a.showEphemeralNotice("恢复会话失败: " + err.Error())
							} else {
								a.readOnly = false
								a.readOnlyReason = ""
								a.showEphemeralNotice("已恢复会话: " + selectedBefore.ID)
							}
						}
					}
				}
				a.state = StateInput
				cmds = append(cmds, a.input.Focus(), a.leaveModalMouseMode())
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
		case tea.KeyCtrlY:
			// 切换鼠标模式：浏览 ↔ 复制
			return a, a.toggleMouseMode()
		case tea.KeyEsc:
			// 复制模式下 Esc 返回浏览模式
			if a.mouseMode == MouseModeSelect {
				a.mouseMode = MouseModeBrowse
				return a, tea.EnableMouseCellMotion
			}
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
					a.askQuestion = ""
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
			// 只读模式下，先检查 slash 命令安全等级，拦截危险命令
			if a.readOnly && !a.slashHandler.IsReadOnlySafe(text) {
				cmdName := strings.Fields(text)[0]
				a.showEphemeralNotice("当前会话为只读浏览，不能执行 " + cmdName)
				return a, nil
			}
			model, cmd := a.handleSlashCommand(text)
			a.resizeLayout()
			return model, cmd
		}

		// 只读模式拦截普通消息发送
		if a.readOnly {
			a.showEphemeralNotice(a.readOnlyReason)
			return a, nil
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
		// 权限卡片是 BottomFloat 层（非全屏 modal），纯键盘交互，不需要切换鼠标模式
		return a, nil

	case MsgDiffPreview:
		a.state = StateDiffPreview
		a.diff.Show(msg.Path, msg.DiffText, msg.Response)
		a.input.Blur()
		return a, a.enterModalMouseMode()

	case MsgAskUser:
		a.state = StateAskUser
		a.askResponseCh = msg.Response
		a.askQuestion = msg.Question
		a.resizeLayout()
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
		a.showEphemeralNotice(msg.Text)
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

	contentWidth := a.contentWidth()

	var content LayerContent

	// BottomFloat 层：临时通知 / spinner / 权限确认 / AskUser 问题
	var floatParts []string
	if a.askQuestion != "" && a.state == StateAskUser {
		// AskUser 问题卡片（不写入 transcript）
		askWidth := min(80, max(40, contentWidth-4))
		askCard := lipgloss.NewStyle().
			BorderLeft(true).
			BorderForeground(ColorPerm).
			PaddingLeft(1).
			Width(askWidth).
			Render(lipgloss.NewStyle().Foreground(ColorPerm).Bold(true).Render("需要输入") + "\n" + StyleDim.Render(a.askQuestion))
		floatParts = append(floatParts, askCard)
	}
	if a.ephemeralNotice != "" {
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

	// Bottom 层：Composer（双分隔线 + 输入 + 状态栏）
	content.Bottom = renderComposer(a.input.View(), ComposerConfig{
		Model:      a.model,
		PermMode:   a.permMode,
		MouseMode:  a.mouseMode,
		ReadOnly:   a.readOnly,
		Tracker:    a.tracker,
		MaxContext: a.maxContext,
	}, contentWidth)

	// 主滚动区：transcript（含"有新消息"提示，尺寸同步由 resizeLayout 负责）
	content.Main = a.chat.ViewWithHint()

	// Overlay 层：选择器等覆盖组件（居中浮层，只覆盖 box 区域，不抹背景）
	if a.resumeSelector.IsVisible() {
		content.Overlay = a.resumeSelector.View()
	}

	// Modal 层：diff / panel（覆盖全屏，非空行替换 base，保留背景感）
	if a.diff.IsVisible() {
		content.Modal = a.diff.View()
	} else if a.panel.IsVisible() {
		content.Modal = a.panel.View()
	}

	return lipgloss.NewStyle().Padding(0, 1).Render(a.layout.Render(content))
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
		OnResume: func() ([]slash.ResumeEntryData, error) {
			if a.OnResume == nil {
				return nil, nil
			}
			entries, err := a.OnResume()
			if err != nil {
				return nil, err
			}
			result := make([]slash.ResumeEntryData, len(entries))
			for i, e := range entries {
				result[i] = slash.ResumeEntryData{
					ID:         e.ID,
					Model:      e.Model,
					Provider:   e.Provider,
					BaseURL:    e.BaseURL,
					AuthSource: e.AuthSource,
					Turns:      e.Turns,
					Cost:       e.Cost,
					Mode:       string(e.Mode),
					ModeReason: e.ModeReason,
				}
			}
			return result, nil
		},
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
			a.chat.InvalidateCache()
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
		// 直接进入执行状态，不往 transcript 插入系统消息
		a.state = StateQuery
		submitCmd := func() tea.Msg {
			a.submitCh <- result.Content
			return nil
		}
		return a, tea.Batch(submitCmd, a.safeWaitForEvent())

	case slash.ResultNotice:
		// 短提示：临时通知
		a.showEphemeralNotice(result.Content)

	case slash.ResultPanel:
		// 长内容：打开可滚动面板
		a.panel.Show(cmd, result.Content)
		a.state = StatePanel
		a.input.Blur()
		return a, a.enterModalMouseMode()

	case slash.ResultSelector:
		// 选择器：解析数据并打开 ResumeSelector
		a.openResumeSelector(result.Content)
		return a, a.enterModalMouseMode()
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

	contentWidth := a.contentWidth()

	// 子组件尺寸同步
	a.input, _ = a.input.Update(tea.WindowSizeMsg{Width: contentWidth - 2})
	a.permission, _ = a.permission.Update(tea.WindowSizeMsg{Width: contentWidth, Height: a.height})
	a.diff, _ = a.diff.Update(tea.WindowSizeMsg{Width: contentWidth, Height: a.height})
	a.panel, _ = a.panel.Update(tea.WindowSizeMsg{Width: contentWidth, Height: a.height})
	a.resumeSelector, _ = a.resumeSelector.Update(tea.WindowSizeMsg{Width: contentWidth, Height: a.height})

	// 计算主区高度并同步 chat viewport（关键：从 View 移到此处）
	a.syncChatSize()
}

// syncChatSize 计算主区高度并同步 chat viewport 尺寸
func (a *App) syncChatSize() {
	contentWidth := a.contentWidth()
	bottomContent := renderComposer(a.input.View(), ComposerConfig{
		Model:      a.model,
		PermMode:   a.permMode,
		MouseMode:  a.mouseMode,
		ReadOnly:   a.readOnly,
		Tracker:    a.tracker,
		MaxContext: a.maxContext,
	}, contentWidth)
	chatHeight := a.layout.MainHeight(bottomContent)
	a.chat, _ = a.chat.Update(tea.WindowSizeMsg{Width: contentWidth, Height: chatHeight})
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

// openResumeSelector 解析 JSON 数据并打开选择器
func (a *App) openResumeSelector(jsonData string) {
	var slashEntries []slash.ResumeEntryData
	if err := json.Unmarshal([]byte(jsonData), &slashEntries); err != nil {
		a.showEphemeralNotice("解析会话数据失败")
		return
	}

	entries := make([]ResumeEntry, len(slashEntries))
	for i, e := range slashEntries {
		entries[i] = ResumeEntry{
			ID:         e.ID,
			Model:      e.Model,
			Provider:   e.Provider,
			BaseURL:    e.BaseURL,
			AuthSource: e.AuthSource,
			Turns:      e.Turns,
			Cost:       e.Cost,
			Mode:       ResumeMode(e.Mode),
			ModeReason: e.ModeReason,
		}
	}

	a.resumeSelector.Show(entries)
	a.state = StateResumeSelect
	a.input.Blur()
}

// IsReadOnly 返回当前是否处于只读浏览模式
func (a *App) IsReadOnly() bool {
	return a.readOnly
}

// SetModel 切换运行时模型（/resume 恢复后调用）
func (a *App) SetModel(model string) {
	a.model = model
}

// SetProvider 切换运行时 provider 名称（/resume 恢复后调用）
func (a *App) SetProvider(provider string) {
	a.provider = provider
}

// RestoreTranscript 从结构化消息恢复前台 transcript（/resume 后调用）
func (a *App) RestoreTranscript(msgs []ChatMessage) {
	a.chat.LoadMessages(msgs)
	a.resizeLayout()
}

// toggleMouseMode 切换浏览/复制模式
func (a *App) toggleMouseMode() tea.Cmd {
	if a.mouseMode == MouseModeBrowse {
		a.mouseMode = MouseModeSelect
		return tea.DisableMouse
	}
	a.mouseMode = MouseModeBrowse
	return tea.EnableMouseCellMotion
}

// enterModalMouseMode modal/panel/selector 打开时调用：保存当前模式，强制浏览
func (a *App) enterModalMouseMode() tea.Cmd {
	a.mouseModePrev = a.mouseMode
	a.mouseMode = MouseModeBrowse
	return tea.EnableMouseCellMotion
}

// leaveModalMouseMode modal/panel/selector 关闭时调用：恢复之前的鼠标模式
func (a *App) leaveModalMouseMode() tea.Cmd {
	a.mouseMode = a.mouseModePrev
	if a.mouseMode == MouseModeSelect {
		return tea.DisableMouse
	}
	return nil // 已经在浏览模式
}

func (a *App) shouldRouteKeyToChat(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "pgup", "pgdown", "home", "end":
		return true
	case "t":
		// t 键只在非输入状态下路由到 chat（toggle thinking）
		return a.state != StateInput && a.state != StateAskUser
	default:
		return false
	}
}

func (a *App) contentWidth() int {
	width := a.width - 2
	if width < 40 {
		width = a.width
	}
	return width
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
