package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Panel 可滚动文本面板（用于 /help /config /permissions 等长输出）
type Panel struct {
	visible  bool
	title    string
	viewport viewport.Model
	width    int
	height   int
}

// NewPanel 创建面板
func NewPanel() Panel {
	vp := viewport.New(60, 12)
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 3
	return Panel{
		viewport: vp,
	}
}

// Show 显示面板
func (p *Panel) Show(title, content string) {
	p.visible = true
	p.title = title
	p.viewport.SetContent(content)
	p.resize()
	p.viewport.GotoTop()
}

// Hide 隐藏面板
func (p *Panel) Hide() {
	p.visible = false
}

// IsVisible 是否可见
func (p Panel) IsVisible() bool {
	return p.visible
}

// Update 处理事件
func (p Panel) Update(msg tea.Msg) (Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		p.resize()
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			p.visible = false
			return p, nil
		}
	}

	if p.visible {
		vp, cmd := p.viewport.Update(msg)
		p.viewport = vp
		return p, cmd
	}
	return p, nil
}

// View 渲染面板（居中 modal 风格）
func (p Panel) View() string {
	if !p.visible {
		return ""
	}

	// 标题
	title := lipgloss.NewStyle().Foreground(ColorBrand).Bold(true).Render(p.title)
	hint := StyleDim.Render("Esc 关闭 · 鼠标滚轮/PgUp/PgDn 浏览")

	content := strings.Join([]string{title, p.viewport.View(), hint}, "\n\n")

	boxWidth := min(100, max(60, p.width-12))
	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorTextDim).
		Padding(1, 2).
		Width(boxWidth).
		Render(content)

	return lipgloss.Place(p.width, p.height, lipgloss.Center, lipgloss.Center, box)
}

func (p *Panel) resize() {
	width := min(94, max(54, p.width-20))
	height := min(24, max(8, p.height-10))
	p.viewport.Width = width
	p.viewport.Height = height
}
