package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// InputBox 输入框组件
type InputBox struct {
	textarea      textarea.Model
	history       []string
	histIdx       int // -1 表示当前输入
	current       string // 浏览历史时暂存当前输入
	width         int
	slashCommands []string // 斜杠命令列表（由外部注入）
}

// NewInputBox 创建输入框
func NewInputBox(commands []string) InputBox {
	ta := textarea.New()
	ta.Placeholder = "输入消息... (Enter 发送, Shift+Enter 换行)"
	ta.Prompt = StyleInputPrompt.Render("❯ ")
	ta.CharLimit = 0 // 不限制
	ta.SetHeight(1)  // 初始 1 行高度
	ta.MaxHeight = 6 // 最多扩展到 6 行
	ta.ShowLineNumbers = false
	ta.Focus()

	return InputBox{
		textarea:      ta,
		histIdx:       -1,
		slashCommands: commands,
	}
}

func (i InputBox) Init() tea.Cmd {
	return textarea.Blink
}

// MsgSubmit 用户提交消息
type MsgSubmit struct {
	Text string
}

func (i InputBox) Update(msg tea.Msg) (InputBox, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		i.width = msg.Width
		i.textarea.SetWidth(msg.Width - 4) // 留出边框空间
		return i, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			// Shift+Enter 换行，纯 Enter 提交
			if msg.Alt {
				// Alt+Enter 作为换行的备选方案
				break
			}
			text := strings.TrimSpace(i.textarea.Value())
			if text == "" {
				return i, nil
			}
			// 过滤掉终端 ANSI 转义序列响应（如 OSC 11 背景色查询的响应）
			if containsANSI(text) {
				i.textarea.Reset()
				return i, nil
			}
			// 保存到历史
			i.history = append(i.history, text)
			i.histIdx = -1
			i.textarea.Reset()
			return i, func() tea.Msg { return MsgSubmit{Text: text} }

		case tea.KeyUp:
			// 历史导航：向上
			if i.textarea.Line() == 0 && len(i.history) > 0 {
				if i.histIdx == -1 {
					i.current = i.textarea.Value()
					i.histIdx = len(i.history) - 1
				} else if i.histIdx > 0 {
					i.histIdx--
				}
				i.textarea.SetValue(i.history[i.histIdx])
				return i, nil
			}

		case tea.KeyDown:
			// 历史导航：向下
			if i.histIdx >= 0 {
				if i.histIdx < len(i.history)-1 {
					i.histIdx++
					i.textarea.SetValue(i.history[i.histIdx])
				} else {
					i.histIdx = -1
					i.textarea.SetValue(i.current)
				}
				return i, nil
			}
		}
	}

	var cmd tea.Cmd
	i.textarea, cmd = i.textarea.Update(msg)
	return i, cmd
}

func (i InputBox) View() string {
	// 如果输入以 / 开头，显示命令提示
	val := i.textarea.Value()
	hint := ""
	if strings.HasPrefix(val, "/") && !strings.Contains(val, " ") {
		var matches []string
		for _, cmd := range i.slashCommands {
			if strings.HasPrefix(cmd, val) {
				matches = append(matches, cmd)
			}
		}
		if len(matches) > 0 && len(matches) <= 10 {
			hint = StyleHint.Render("  " + strings.Join(matches, "  "))
		}
	}

	view := i.textarea.View()
	if hint != "" {
		view += "\n" + hint
	}
	return view
}

// Focus 聚焦输入框
func (i *InputBox) Focus() tea.Cmd {
	return i.textarea.Focus()
}

// Blur 失焦
func (i *InputBox) Blur() {
	i.textarea.Blur()
}

// Reset 清空输入框
func (i *InputBox) Reset() {
	i.textarea.Reset()
}

// Value 获取当前值
func (i InputBox) Value() string {
	return i.textarea.Value()
}

// ansiRegex 匹配 ANSI 转义序列和 OSC 响应
var ansiRegex = regexp.MustCompile(`[\x00-\x08\x0e-\x1f]|\x1b\[.*?[a-zA-Z]|\][\d;]*[a-zA-Z/\\]`)

// containsANSI 检测文本是否包含 ANSI 控制序列
func containsANSI(s string) bool {
	if ansiRegex.MatchString(s) {
		return true
	}
	// 额外检查 OSC 响应的常见模式（不一定以 ESC 开头，可能被截断）
	if strings.Contains(s, "rgb:") || strings.HasPrefix(s, "]") {
		return true
	}
	return false
}
