package tui

import (
	"encoding/json"
	"fmt"
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
			p.respond(PermAllow)
			return p, nil
		case "n", "N":
			p.respond(PermDeny)
			return p, nil
		case "a", "A":
			p.respond(PermAlways)
			return p, nil
		case "e", "E":
			// 使用 E 表示始终拒绝
			p.respond(PermNever)
			return p, nil
		case "esc", "ctrl+c":
			p.respond(PermDeny)
			return p, nil
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

// Card 渲染权限确认卡片
func (p PermissionDialog) Card(width int) string {
	if width < 40 {
		width = 40
	}

	preview := p.previewLines(width - 16)

	header := lipgloss.JoinHorizontal(
		lipgloss.Left,
		StylePermTitle.Render("权限确认"),
		"  ",
		StyleToolName.Render(p.toolName),
	)

	if len(preview) > 0 {
		header = lipgloss.JoinHorizontal(
			lipgloss.Left,
			header,
			"  ",
			StyleDim.Render(preview[0]),
		)
	}

	sections := []string{header}
	if len(preview) > 1 {
		for _, line := range preview[1:] {
			sections = append(sections, StyleToolOutput.Render("  "+line))
		}
	}
	sections = append(sections, StyleDim.Render("Y 允许  ·  N 拒绝  ·  A 始终允许  ·  E 始终拒绝  ·  Esc 取消"))

	return lipgloss.NewStyle().
		BorderLeft(true).
		BorderForeground(ColorPerm).
		PaddingLeft(1).
		Width(width).
		Render(strings.Join(sections, "\n"))
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

func (p PermissionDialog) previewLines(maxWidth int) []string {
	preview := summarizePermissionInput(p.toolName, p.input)
	lines := strings.Split(preview, "\n")
	if len(lines) > 2 {
		lines = append(lines[:2], fmt.Sprintf("… 另有 %d 行参数", len(lines)-2))
	}

	for i, line := range lines {
		lines[i] = truncateText(strings.TrimSpace(line), maxWidth)
	}
	return lines
}

func summarizePermissionInput(toolName string, raw string) string {
	if raw == "" {
		return "无参数"
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return raw
	}

	switch toolName {
	case "Bash":
		if cmd, ok := payload["command"].(string); ok && cmd != "" {
			return cmd
		}
	case "Read", "Write", "Edit":
		if path, ok := payload["path"].(string); ok && path != "" {
			return path
		}
	case "Glob", "Grep":
		if pattern, ok := payload["pattern"].(string); ok && pattern != "" {
			return pattern
		}
	}

	if pretty, err := json.MarshalIndent(payload, "", "  "); err == nil {
		return string(pretty)
	}
	return raw
}
