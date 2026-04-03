package tui

import (
	"encoding/json"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PermissionDialog 权限确认对话框
type PermissionDialog struct {
	visible  bool
	toolName string
	input    string
	response chan PermissionResponse
	width    int
	height   int

	feedbackKey string // 刚按下的键 ("y"/"n"/"a"/"e")
	feedbackAge int    // tick 计数，>= 2 时清除
}

// NewPermissionDialog 创建权限对话框
func NewPermissionDialog() PermissionDialog {
	return PermissionDialog{}
}

func (p PermissionDialog) Init() tea.Cmd { return nil }

func (p PermissionDialog) Update(msg tea.Msg) (PermissionDialog, tea.Cmd) {
	if !p.visible {
		return p, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			p.feedbackKey = "y"
			p.respond(PermAllow)
			return p, nil
		case "n", "N":
			p.feedbackKey = "n"
			p.respond(PermDeny)
			return p, nil
		case "a", "A":
			p.feedbackKey = "a"
			p.respond(PermAlways)
			return p, nil
		case "e", "E":
			p.feedbackKey = "e"
			p.respond(PermNever)
			return p, nil
		case "esc", "ctrl+c":
			p.respond(PermDeny)
			return p, nil
		}

	case MsgSpinnerTick:
		if p.feedbackKey != "" {
			p.feedbackAge++
			if p.feedbackAge >= 2 {
				p.feedbackKey = ""
				p.feedbackAge = 0
			}
		}

	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
	}

	return p, nil
}

func (p PermissionDialog) View() string {
	if !p.visible {
		return ""
	}

	cardWidth := min(78, max(52, p.width-6))
	box := p.Card(cardWidth)
	return lipgloss.Place(p.width, p.height, lipgloss.Center, lipgloss.Bottom, box)
}

// Card 渲染权限确认卡片（圆角边框 + 按键高亮反馈）
func (p PermissionDialog) Card(width int) string {
	if width < 40 {
		width = 40
	}

	// 工具名（品牌色加粗）+ 摘要（dim 色）
	summary := toolInputSummary(p.toolName, p.input)
	nameStyle := lipgloss.NewStyle().Foreground(ColorPerm).Bold(true)
	maxSummaryWidth := width - lipgloss.Width(p.toolName) - 8
	if maxSummaryWidth < 20 {
		maxSummaryWidth = 20
	}
	summaryText := truncateText(summary, maxSummaryWidth)
	header := nameStyle.Render(p.toolName) + "  " + StyleDim.Render(summaryText)

	keys := p.renderKeys()

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorPerm).
		Padding(0, 1).
		Width(width).
		Render(header + "\n" + keys)
}

// renderKeys 渲染快捷键提示（当前按下的键高亮）
func (p PermissionDialog) renderKeys() string {
	type keyDef struct {
		key   string
		label string
	}
	defs := []keyDef{
		{"y", "Y 允许"},
		{"n", "N 拒绝"},
		{"a", "A 总是"},
		{"e", "E 从不"},
	}
	var parts []string
	for _, d := range defs {
		style := StyleDim
		if p.feedbackKey == d.key {
			style = lipgloss.NewStyle().Foreground(ColorPerm).Bold(true)
		}
		parts = append(parts, style.Render(d.label))
	}
	return strings.Join(parts, "  ")
}

// toolInputSummary 根据工具名提取紧凑摘要
func toolInputSummary(toolName, rawInput string) string {
	if rawInput == "" {
		return "无参数"
	}

	// 尝试解析 JSON
	var input map[string]interface{}
	if err := json.Unmarshal([]byte(rawInput), &input); err != nil {
		if len(rawInput) > 80 {
			return rawInput[:80] + "..."
		}
		return rawInput
	}

	switch toolName {
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			if len(cmd) > 80 {
				return cmd[:80] + "..."
			}
			return cmd
		}
	case "Edit", "Write", "Read":
		if fp, ok := input["file_path"].(string); ok {
			return fp
		}
		// 兼容旧字段名
		if fp, ok := input["path"].(string); ok {
			return fp
		}
	case "Glob":
		if p, ok := input["pattern"].(string); ok {
			return p
		}
	case "Grep":
		if p, ok := input["pattern"].(string); ok {
			path := ""
			if pp, ok := input["path"].(string); ok {
				path = " in " + pp
			}
			return p + path
		}
	}

	// 兜底：JSON 前 80 字符
	if len(rawInput) > 80 {
		return rawInput[:80] + "..."
	}
	return rawInput
}

// Show 显示权限对话框
func (p *PermissionDialog) Show(toolName, input string, responseCh chan PermissionResponse) {
	p.visible = true
	p.toolName = toolName
	p.input = input
	p.response = responseCh
}

// Hide 隐藏对话框
func (p *PermissionDialog) Hide() {
	p.visible = false
	p.toolName = ""
	p.input = ""
	p.response = nil
}

// IsVisible 是否可见
func (p PermissionDialog) IsVisible() bool {
	return p.visible
}

func (p *PermissionDialog) respond(r PermissionResponse) {
	if p.response != nil {
		p.response <- r
		close(p.response)
	}
	p.Hide()
}

