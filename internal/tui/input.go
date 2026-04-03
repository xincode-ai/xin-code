package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CommandHint 斜杠命令提示
type CommandHint struct {
	Name        string
	Description string
}

// InputBox 输入框组件
type InputBox struct {
	textarea      textarea.Model
	history       []string
	histIdx       int    // -1 表示当前输入
	current       string // 浏览历史时暂存当前输入
	width         int
	slashCommands []CommandHint // 斜杠命令列表（由外部注入）
}

// NewInputBox 创建输入框
func NewInputBox(commands []CommandHint) InputBox {
	ta := textarea.New()
	ta.Placeholder = "输入需求，回车发送，Alt+Enter 换行，/ 打开命令"
	ta.Prompt = StyleInputPrompt.Render("› ")
	ta.CharLimit = 0 // 不限制
	ta.SetHeight(1)  // 初始 1 行高度
	ta.MaxHeight = 6 // 最多扩展到 6 行
	ta.ShowLineNumbers = false
	ta.Focus()
	ta.FocusedStyle.CursorLine = ta.FocusedStyle.CursorLine.UnsetBackground()
	ta.FocusedStyle.Base = ta.FocusedStyle.Base.Foreground(ColorText)
	ta.FocusedStyle.Placeholder = ta.FocusedStyle.Placeholder.Foreground(ColorTextDim)
	ta.FocusedStyle.Prompt = ta.FocusedStyle.Prompt.Foreground(ColorBrand).Bold(true)
	ta.BlurredStyle = ta.FocusedStyle

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
		width := msg.Width - 2
		if width < 24 {
			width = 24
		}
		i.textarea.SetWidth(width)
		return i, nil

	case tea.KeyMsg:
		// 过滤泄漏的 SGR 鼠标转义序列片段，防止输入到 textarea
		if msg.Type == tea.KeyRunes && sgrMouseRegex.MatchString(msg.String()) {
			return i, nil
		}
		switch msg.Type {
		case tea.KeyTab:
			// Tab 补全斜杠命令
			matches := i.matchSlashCommands()
			if len(matches) == 1 {
				// 唯一匹配：直接补全
				i.textarea.SetValue(matches[0].Name + " ")
				return i, nil
			}
			if len(matches) > 1 {
				// 多个匹配：补全公共前缀
				prefix := commonPrefix(matches)
				if prefix != i.textarea.Value() {
					i.textarea.SetValue(prefix)
				}
				return i, nil
			}

		case tea.KeyEnter:
			// Alt+Enter 换行
			if msg.Alt {
				i.textarea.InsertString("\n")
				return i, nil
			}
			text := strings.TrimSpace(i.textarea.Value())
			if text == "" {
				return i, nil
			}
			// 过滤掉终端 ANSI 转义序列响应（如背景色查询的响应）
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
	var sections []string
	if hint := i.renderSlashHint(); hint != "" {
		sections = append(sections, hint)
	}
	sections = append(sections, i.textarea.View())
	return strings.Join(sections, "\n")
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

// Height 返回输入组件当前高度
func (i InputBox) Height() int {
	height := i.textarea.Height()
	if hint := i.renderSlashHint(); hint != "" {
		height += lipgloss.Height(hint)
	}
	return height
}

func (i InputBox) renderSlashHint() string {
	matches := i.matchSlashCommands()
	if len(matches) == 0 {
		return ""
	}

	boxWidth := min(72, max(34, i.width-2))
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Foreground(ColorTextDim).Bold(true).Render("commands"))

	limit := min(6, len(matches))
	for _, cmd := range matches[:limit] {
		line := fmt.Sprintf("%s  %s",
			lipgloss.NewStyle().Foreground(ColorText).Render(truncateText(cmd.Name, 16)),
			StyleDim.Render(truncateText(cmd.Description, max(12, boxWidth-24))),
		)
		lines = append(lines, line)
	}

	if len(matches) > limit {
		lines = append(lines, StyleDim.Render(fmt.Sprintf("还有 %d 个命令，继续输入可缩小范围", len(matches)-limit)))
	}

	return lipgloss.NewStyle().BorderLeft(true).BorderForeground(ColorTextDim).PaddingLeft(1).Width(boxWidth).Render(strings.Join(lines, "\n"))
}

func (i InputBox) matchSlashCommands() []CommandHint {
	val := strings.TrimSpace(i.textarea.Value())
	if !strings.HasPrefix(val, "/") || strings.Contains(val, " ") {
		return nil
	}

	var matches []CommandHint
	for _, cmd := range i.slashCommands {
		if strings.HasPrefix(cmd.Name, val) {
			matches = append(matches, cmd)
		}
	}
	return matches
}

// ansiRegex 匹配 ANSI 转义序列和 OSC 响应
var ansiRegex = regexp.MustCompile(`[\x00-\x08\x0e-\x1f]|\x1b\[.*?[a-zA-Z]|\][\d;]*[a-zA-Z/\\]`)

// sgrMouseRegex 匹配泄漏的 SGR 鼠标转义序列片段（ESC 被拆到前一次读取）
var sgrMouseRegex = regexp.MustCompile(`\[<\d+;\d+;\d+[Mm]`)

// containsANSI 检测文本是否包含 ANSI 控制序列
func containsANSI(s string) bool {
	if ansiRegex.MatchString(s) {
		return true
	}
	// 额外检查 OSC 响应的常见模式（不一定以 ESC 开头，可能被截断）
	if strings.Contains(s, "rgb:") || strings.HasPrefix(s, "]") {
		return true
	}
	// 检查泄漏的 SGR 鼠标序列片段
	if sgrMouseRegex.MatchString(s) {
		return true
	}
	return false
}

// commonPrefix 计算多个命令名的公共前缀（Tab 补全用）
func commonPrefix(commands []CommandHint) string {
	if len(commands) == 0 {
		return ""
	}
	prefix := commands[0].Name
	for _, cmd := range commands[1:] {
		for !strings.HasPrefix(cmd.Name, prefix) {
			prefix = prefix[:len(prefix)-1]
			if prefix == "" {
				return ""
			}
		}
	}
	return prefix
}
