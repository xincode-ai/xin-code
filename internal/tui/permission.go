package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// PermissionDialog 权限确认对话框
type PermissionDialog struct {
	visible  bool
	toolName string
	input    string
	response chan PermissionResponse
	width    int
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
			// 'e' for nEver
			p.respond(PermNever)
			return p, nil
		}

	case tea.WindowSizeMsg:
		p.width = msg.Width
	}

	return p, nil
}

func (p PermissionDialog) View() string {
	if !p.visible {
		return ""
	}

	maxInputWidth := p.width - 6
	if maxInputWidth < 40 {
		maxInputWidth = 40
	}

	// 截断输入显示
	inputPreview := p.input
	if len(inputPreview) > 200 {
		inputPreview = inputPreview[:200] + "..."
	}
	// 对齐宽度
	lines := strings.Split(inputPreview, "\n")
	if len(lines) > 5 {
		lines = append(lines[:5], "...")
	}
	inputPreview = strings.Join(lines, "\n")

	title := StylePermTitle.Render("⚠ 权限确认")
	tool := StyleToolName.Render(p.toolName)
	options := StyleHint.Render("[y]允许  [n]拒绝  [a]总是允许  [e]总是拒绝")

	content := fmt.Sprintf(
		"%s\n\n工具: %s\n参数:\n%s\n\n%s",
		title, tool,
		StyleToolOutput.Render(inputPreview),
		options,
	)

	return StylePermBox.Width(maxInputWidth).Render(content)
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
