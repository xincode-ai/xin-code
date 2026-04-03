package tui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
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
	textarea        textarea.Model
	history         []string
	histIdx         int    // -1 表示当前输入
	current         string // 浏览历史时暂存当前输入
	width           int
	slashCommands   []CommandHint // 斜杠命令列表（由外部注入）
	lastMouseTime   time.Time     // 最近一次鼠标事件时间戳（用于抑制转义碎片）
	completionIdx   int           // -1 = 无选中
	completionItems []CommandHint // 当前模糊匹配结果缓存
}

// NewInputBox 创建输入框
func NewInputBox(commands []CommandHint) InputBox {
	ta := textarea.New()
	ta.Placeholder = "输入消息..."
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
		completionIdx: -1,
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

// RecordMouseEvent 记录鼠标事件时间（由 app.go 调用）
func (i *InputBox) RecordMouseEvent() {
	i.lastMouseTime = time.Now()
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
		// 鼠标事件后 150ms 内抑制疑似转义碎片（数字+分号+字母组合）
		if msg.Type == tea.KeyRunes && !i.lastMouseTime.IsZero() &&
			time.Since(i.lastMouseTime) < 150*time.Millisecond {
			s := msg.String()
			if mouseFragmentRegex.MatchString(s) {
				return i, nil
			}
		}
		switch msg.Type {
		case tea.KeyTab:
			// Tab 补全斜杠命令
			if len(i.completionItems) > 0 {
				idx := i.completionIdx
				if idx < 0 {
					idx = 0
				}
				i.textarea.SetValue(i.completionItems[idx].Name + " ")
				i.completionIdx = -1
				i.completionItems = nil
				return i, nil
			}

		case tea.KeyEnter:
			// Alt+Enter 换行
			if msg.Alt {
				i.textarea.InsertString("\n")
				return i, nil
			}
			// 补全列表可见且有选中项：补全并提交该命令
			if i.hasActiveCompletion() && i.completionIdx >= 0 {
				text := i.completionItems[i.completionIdx].Name
				i.completionIdx = -1
				i.completionItems = nil
				i.textarea.Reset()
				i.history = append(i.history, text)
				i.histIdx = -1
				return i, func() tea.Msg { return MsgSubmit{Text: text} }
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
			i.completionIdx = -1
			i.completionItems = nil
			i.textarea.Reset()
			return i, func() tea.Msg { return MsgSubmit{Text: text} }

		case tea.KeyUp:
			// 补全列表导航：向上
			if i.hasActiveCompletion() {
				if i.completionIdx > 0 {
					i.completionIdx--
				}
				return i, nil
			}
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
			// 补全列表导航：向下
			if i.hasActiveCompletion() {
				maxIdx := min(len(i.completionItems)-1, 7)
				if i.completionIdx < maxIdx {
					i.completionIdx++
				}
				return i, nil
			}
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

		case tea.KeyEsc:
			// 补全列表可见时：关闭补全
			if i.hasActiveCompletion() {
				i.completionIdx = -1
				i.completionItems = nil
				return i, nil
			}
		}
	}

	var cmd tea.Cmd
	i.textarea, cmd = i.textarea.Update(msg)

	// 同步补全状态
	newMatches := i.matchSlashCommands()
	i.completionItems = newMatches
	if len(newMatches) > 0 {
		if i.completionIdx < 0 {
			i.completionIdx = 0
		}
		if i.completionIdx >= len(newMatches) {
			i.completionIdx = len(newMatches) - 1
		}
	} else {
		i.completionIdx = -1
	}

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

// hasActiveCompletion 判断补全列表是否处于活跃状态
func (i InputBox) hasActiveCompletion() bool {
	val := strings.TrimSpace(i.textarea.Value())
	return strings.HasPrefix(val, "/") && !strings.Contains(val, " ") && len(i.completionItems) > 0
}

func (i InputBox) renderSlashHint() string {
	matches := i.completionItems
	if len(matches) == 0 {
		return ""
	}

	boxWidth := min(72, max(34, i.width-2))
	var lines []string
	limit := min(8, len(matches))
	for idx, cmd := range matches[:limit] {
		name := truncateText(cmd.Name, 16)
		desc := truncateText(cmd.Description, max(12, boxWidth-24))
		if idx == i.completionIdx {
			line := lipgloss.NewStyle().Foreground(ColorBrand).Bold(true).Render(name) +
				"  " + lipgloss.NewStyle().Foreground(ColorText).Render(desc)
			lines = append(lines, "❯ "+line)
		} else {
			line := lipgloss.NewStyle().Foreground(ColorText).Bold(true).Render(name) +
				"  " + StyleDim.Render(desc)
			lines = append(lines, "  "+line)
		}
	}
	if len(matches) > limit {
		lines = append(lines, StyleDim.Render(fmt.Sprintf("  还有 %d 个命令", len(matches)-limit)))
	}
	lines = append(lines, StyleDim.Render("  Tab 补全 · Enter 执行 · ↑↓ 选择"))

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorInputBorder).
		PaddingLeft(1).
		Width(boxWidth).
		Render(strings.Join(lines, "\n"))
}

func (i InputBox) matchSlashCommands() []CommandHint {
	val := strings.TrimSpace(i.textarea.Value())
	if !strings.HasPrefix(val, "/") || strings.Contains(val, " ") {
		return nil
	}

	type scored struct {
		hint  CommandHint
		score int
	}
	var matches []scored
	for _, cmd := range i.slashCommands {
		if ok, sc := fuzzyMatchCommand(val, cmd.Name); ok {
			matches = append(matches, scored{cmd, sc})
		}
	}
	sort.Slice(matches, func(a, b int) bool {
		if matches[a].score != matches[b].score {
			return matches[a].score > matches[b].score
		}
		return matches[a].hint.Name < matches[b].hint.Name
	})
	result := make([]CommandHint, 0, len(matches))
	for _, m := range matches {
		result = append(result, m.hint)
	}
	return result
}

// fuzzyMatchCommand 模糊匹配斜杠命令
// input 和 target 都以 "/" 开头，如 "/co" 和 "/commit"
// 匹配规则：input 的每个字符按顺序出现在 target 中（子序列匹配）
// 评分：+10 每个匹配字符，+5 位置对齐（前缀式），+3 连续匹配
func fuzzyMatchCommand(input, target string) (bool, int) {
	inputLower := strings.ToLower(input)
	targetLower := strings.ToLower(target)
	inputRunes := []rune(inputLower)
	targetRunes := []rune(targetLower)

	if len(inputRunes) == 0 {
		return true, 0
	}
	if len(inputRunes) > len(targetRunes) {
		return false, 0
	}

	score := 0
	ti := 0 // target 索引
	lastMatchIdx := -1

	for ii := 0; ii < len(inputRunes); ii++ {
		found := false
		for ti < len(targetRunes) {
			if targetRunes[ti] == inputRunes[ii] {
				score += 10
				// 位置对齐加分（input[ii] 匹配 target[ii]，前缀式）
				if ti == ii {
					score += 5
				}
				// 连续匹配加分
				if lastMatchIdx >= 0 && ti == lastMatchIdx+1 {
					score += 3
				}
				lastMatchIdx = ti
				ti++
				found = true
				break
			}
			ti++
		}
		if !found {
			return false, 0
		}
	}
	return true, score
}

// ansiRegex 匹配 ANSI 转义序列和 OSC 响应
var ansiRegex = regexp.MustCompile(`[\x00-\x08\x0e-\x1f]|\x1b\[.*?[a-zA-Z]|\][\d;]*[a-zA-Z/\\]`)

// sgrMouseRegex 匹配泄漏的 SGR 鼠标转义序列片段（ESC 被拆到前一次读取）
var sgrMouseRegex = regexp.MustCompile(`\[<\d+;\d+;\d+[Mm]`)

// mouseFragmentRegex 匹配鼠标转义序列碎片（数字/分号/M/m 组合）
var mouseFragmentRegex = regexp.MustCompile(`^[\d;<>Mm]+$`)

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
