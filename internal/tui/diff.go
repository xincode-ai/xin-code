package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DiffDialog 差异预览弹层
type DiffDialog struct {
	visible  bool
	path     string
	response chan bool
	viewport viewport.Model
	width    int
	height   int
}

// NewDiffDialog 创建差异预览弹层
func NewDiffDialog() DiffDialog {
	vp := viewport.New(60, 12)
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 3
	return DiffDialog{
		viewport: vp,
	}
}

func (d DiffDialog) Init() tea.Cmd { return nil }

func (d DiffDialog) Update(msg tea.Msg) (DiffDialog, tea.Cmd) {
	if !d.visible {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			d.width = msg.Width
			d.height = msg.Height
		}
		return d, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
		d.resize()
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			d.respond(true)
			return d, nil
		case "n", "N":
			d.respond(false)
			return d, nil
		}
	}

	vp, cmd := d.viewport.Update(msg)
	d.viewport = vp
	return d, cmd
}

// View 渲染差异预览弹层
func (d DiffDialog) View() string {
	if !d.visible {
		return ""
	}

	content := []string{
		StyleDiffHeader.Render("差异预览"),
		StyleDim.Render(d.path),
		d.viewport.View(),
		StyleDim.Render("Y 确认写入  ·  N 取消  ·  鼠标滚轮 / PgUp / PgDn 浏览"),
	}

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorTextDim).
		Padding(1, 2).
		Width(min(100, max(70, d.width-12))).
		Render(strings.Join(content, "\n\n"))

	return lipgloss.Place(d.width, d.height, lipgloss.Center, lipgloss.Center, box)
}

// Show 显示差异预览弹层
func (d *DiffDialog) Show(path string, diffText string, responseCh chan bool) {
	d.visible = true
	d.path = path
	d.response = responseCh
	d.viewport.SetContent(renderDiff(diffText))
	d.resize()
	d.viewport.GotoTop()
}

// Hide 隐藏差异预览弹层
func (d *DiffDialog) Hide() {
	d.visible = false
	d.path = ""
	d.response = nil
}

// IsVisible 是否可见
func (d DiffDialog) IsVisible() bool {
	return d.visible
}

func (d *DiffDialog) resize() {
	width := min(96, max(64, d.width-18))
	height := min(24, max(10, d.height-10))
	d.viewport.Width = width
	d.viewport.Height = height
}

func (d *DiffDialog) respond(confirmed bool) {
	if d.response != nil {
		d.response <- confirmed
		close(d.response)
	}
	d.Hide()
}

func renderDiff(diffText string) string {
	lines := strings.Split(diffText, "\n")
	var rendered []string
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "@@"), strings.HasPrefix(line, "diff "), strings.HasPrefix(line, "index "):
			rendered = append(rendered, StyleDiffHeader.Render(line))
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			rendered = append(rendered, StyleDim.Render(line))
		case strings.HasPrefix(line, "+"):
			rendered = append(rendered, StyleDiffAdd.Render(line))
		case strings.HasPrefix(line, "-"):
			rendered = append(rendered, StyleDiffDel.Render(line))
		default:
			rendered = append(rendered, StyleDiffCtx.Render(line))
		}
	}
	return strings.Join(rendered, "\n")
}
