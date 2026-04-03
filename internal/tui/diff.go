package tui

import (
	"fmt"
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
		case "n", "N", "esc":
			d.respond(false)
			return d, nil
		}
	}

	vp, cmd := d.viewport.Update(msg)
	d.viewport = vp
	return d, cmd
}

// View 渲染差异预览弹层（modal 层独占全屏）
func (d DiffDialog) View() string {
	if !d.visible {
		return ""
	}

	// 标题行：文件路径
	title := StyleDiffHeader.Render("差异预览") + "  " + StyleDim.Render(d.path)

	content := []string{
		title,
		d.viewport.View(),
		StyleDim.Render("y 接受 · n 拒绝 · 鼠标滚轮/PgUp/PgDn 浏览"),
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

// renderDiff 渲染 diff 文本，hunk 级别增删高亮 + 行号显示
func renderDiff(diffText string) string {
	lines := strings.Split(diffText, "\n")
	var rendered []string

	// 行号追踪
	var oldLine, newLine int

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff "), strings.HasPrefix(line, "index "):
			// 文件头：蓝紫色加粗
			rendered = append(rendered, StyleDiffHeader.Render(line))

		case strings.HasPrefix(line, "---"), strings.HasPrefix(line, "+++"):
			// 文件路径行：dim 色
			rendered = append(rendered, StyleDim.Render(line))

		case strings.HasPrefix(line, "@@"):
			// hunk header：前置空行分隔 + 蓝紫色
			if len(rendered) > 0 {
				rendered = append(rendered, "")
			}
			rendered = append(rendered, StyleDiffHeader.Render(line))
			// 从 @@ -old,count +new,count @@ 中解析起始行号
			oldLine, newLine = parseHunkHeader(line)

		case strings.HasPrefix(line, "+"):
			// 新增行：绿色 + 行号
			lineNum := fmt.Sprintf("%4d ", newLine)
			rendered = append(rendered, StyleDiffAdd.Render(lineNum+"│ "+line))
			newLine++

		case strings.HasPrefix(line, "-"):
			// 删除行：红色 + 行号
			lineNum := fmt.Sprintf("%4d ", oldLine)
			rendered = append(rendered, StyleDiffDel.Render(lineNum+"│ "+line))
			oldLine++

		default:
			// 上下文行：dim 色 + 行号
			if oldLine > 0 || newLine > 0 {
				lineNum := fmt.Sprintf("%4d ", newLine)
				rendered = append(rendered, StyleDiffCtx.Render(lineNum+"│ "+line))
				oldLine++
				newLine++
			} else {
				rendered = append(rendered, StyleDiffCtx.Render(line))
			}
		}
	}
	return strings.Join(rendered, "\n")
}

// parseHunkHeader 从 @@ -old,count +new,count @@ 中解析起始行号
func parseHunkHeader(header string) (oldStart, newStart int) {
	// 格式: @@ -7,6 +7,8 @@  或  @@ -7 +7,8 @@
	oldStart, newStart = 1, 1
	header = strings.TrimPrefix(header, "@@ ")
	parts := strings.Fields(header)
	for _, p := range parts {
		if strings.HasPrefix(p, "-") {
			p = strings.TrimPrefix(p, "-")
			if idx := strings.Index(p, ","); idx > 0 {
				p = p[:idx]
			}
			fmt.Sscanf(p, "%d", &oldStart)
		} else if strings.HasPrefix(p, "+") && !strings.HasPrefix(p, "++") {
			p = strings.TrimPrefix(p, "+")
			if idx := strings.Index(p, ","); idx > 0 {
				p = p[:idx]
			}
			fmt.Sscanf(p, "%d", &newStart)
		}
	}
	return
}
