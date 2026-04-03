package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ResumeMode 恢复能力级别
type ResumeMode string

const (
	ResumeContinue ResumeMode = "continue" // 完全兼容，可继续对话
	ResumeReadonly ResumeMode = "readonly" // 可浏览历史，不可续接
	ResumeBlocked  ResumeMode = "blocked"  // 不兼容，不可恢复
)

// 统一文案常量（selector / Enter 提示 / 输入区 共用）
const (
	LabelContinue       = "可继续"
	LabelReadonly        = "仅浏览"
	LabelBlocked         = "不兼容"
	ReasonCompatible     = "同运行环境，可继续"
	ReasonAuthMismatch   = "认证方式不兼容，仅可浏览"
	ReasonProviderBlock  = "跨 provider，不可恢复"
	ReasonEndpointBlock  = "跨 endpoint，不可恢复"
	ReasonReadonlyNotice = "当前会话为只读浏览，运行环境不兼容，不能继续对话"
)

// ResumeEntry 历史会话条目（含兼容性元数据）
type ResumeEntry struct {
	ID           string
	Model        string
	Provider     string
	BaseURL      string
	AuthSource   string
	Turns        int
	Cost         string
	Mode         ResumeMode // 恢复能力
	ModeReason   string     // 原因文案
}

// ResumeSelector 会话恢复选择器（真正可滚动）
type ResumeSelector struct {
	visible      bool
	entries      []ResumeEntry
	selected     int
	scrollOffset int // 视口起始索引
	visibleRows  int // 可见行数
	width        int
	height       int
}

const defaultVisibleRows = 10

// NewResumeSelector 创建会话恢复选择器
func NewResumeSelector() ResumeSelector {
	return ResumeSelector{visibleRows: defaultVisibleRows}
}

// Show 显示选择器并填充条目
func (r *ResumeSelector) Show(entries []ResumeEntry) {
	r.visible = true
	r.entries = entries
	r.selected = 0
	r.scrollOffset = 0
}

// Hide 隐藏选择器
func (r *ResumeSelector) Hide() {
	r.visible = false
}

// IsVisible 是否可见
func (r ResumeSelector) IsVisible() bool {
	return r.visible
}

// SelectedEntry 返回当前选中的条目
func (r ResumeSelector) SelectedEntry() *ResumeEntry {
	if r.selected >= 0 && r.selected < len(r.entries) {
		return &r.entries[r.selected]
	}
	return nil
}

// ensureVisible 确保 selected 在视口内
func (r *ResumeSelector) ensureVisible() {
	if r.selected < r.scrollOffset {
		r.scrollOffset = r.selected
	}
	if r.selected >= r.scrollOffset+r.visibleRows {
		r.scrollOffset = r.selected - r.visibleRows + 1
	}
	if r.scrollOffset < 0 {
		r.scrollOffset = 0
	}
	maxOffset := len(r.entries) - r.visibleRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if r.scrollOffset > maxOffset {
		r.scrollOffset = maxOffset
	}
}

// Update 处理键盘和鼠标事件
func (r ResumeSelector) Update(msg tea.Msg) (ResumeSelector, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.width = msg.Width
		r.height = msg.Height
		// 视口行数：高度 - header(2行) - 底部提示(1行) - padding
		rows := msg.Height/2 - 4
		if rows < 5 {
			rows = 5
		}
		if rows > 20 {
			rows = 20
		}
		r.visibleRows = rows
		r.ensureVisible()

	case tea.MouseMsg:
		switch msg.Type {
		case tea.MouseWheelUp:
			if r.selected > 0 {
				r.selected--
				r.ensureVisible()
			}
		case tea.MouseWheelDown:
			if r.selected < len(r.entries)-1 {
				r.selected++
				r.ensureVisible()
			}
		}

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			if r.selected > 0 {
				r.selected--
				r.ensureVisible()
			}
		case tea.KeyDown:
			if r.selected < len(r.entries)-1 {
				r.selected++
				r.ensureVisible()
			}
		case tea.KeyPgUp:
			r.selected -= r.visibleRows
			if r.selected < 0 {
				r.selected = 0
			}
			r.ensureVisible()
		case tea.KeyPgDown:
			r.selected += r.visibleRows
			if r.selected >= len(r.entries) {
				r.selected = len(r.entries) - 1
			}
			r.ensureVisible()
		case tea.KeyHome:
			r.selected = 0
			r.ensureVisible()
		case tea.KeyEnd:
			r.selected = len(r.entries) - 1
			r.ensureVisible()
		case tea.KeyEnter:
			r.visible = false
			// 选中后由外部处理
		case tea.KeyEsc:
			r.visible = false
			r.entries = nil // Esc 取消
		}
	}
	return r, nil
}

// View 渲染选择器列表（居中浮层）
func (r ResumeSelector) View() string {
	if !r.visible || len(r.entries) == 0 {
		return ""
	}

	bold := lipgloss.NewStyle().Bold(true).Foreground(ColorText)
	dim := StyleDim
	hl := lipgloss.NewStyle().Foreground(ColorBrand).Bold(true)

	var lines []string
	lines = append(lines, bold.Render("恢复会话")+"  "+dim.Render("↑/↓ 选择  Enter 恢复  Esc 取消"))
	lines = append(lines, "")

	// 只渲染可视窗口内的条目
	end := r.scrollOffset + r.visibleRows
	if end > len(r.entries) {
		end = len(r.entries)
	}

	for i := r.scrollOffset; i < end; i++ {
		e := r.entries[i]
		cursor := "  "
		style := dim
		if i == r.selected {
			cursor = hl.Render("❯ ")
			style = lipgloss.NewStyle().Foreground(ColorText)
		}
		idShort := e.ID
		if len(idShort) > 8 {
			idShort = idShort[:8]
		}

		// 兼容性状态标签
		var modeTag string
		switch e.Mode {
		case ResumeContinue:
			modeTag = lipgloss.NewStyle().Foreground(ColorSuccess).Render(LabelContinue)
		case ResumeReadonly:
			modeTag = lipgloss.NewStyle().Foreground(ColorWarning).Render(LabelReadonly)
		case ResumeBlocked:
			modeTag = lipgloss.NewStyle().Foreground(ColorError).Render(LabelBlocked)
			style = StyleDim // 不兼容条目整体 dim
		default:
			modeTag = dim.Render("?")
		}

		info := style.Render(fmt.Sprintf("%s  %s  %d 轮  %s", idShort, e.Model, e.Turns, e.Cost))
		lines = append(lines, cursor+info+"  "+modeTag)

		// 选中项显示 reason 详情行
		if i == r.selected && e.ModeReason != "" {
			reasonLine := "     " + dim.Render(e.ModeReason)
			lines = append(lines, reasonLine)
		}
	}

	// 滚动指示器
	if r.scrollOffset > 0 {
		lines = append([]string{lines[0], lines[1], dim.Render("  ↑ 更多会话")}, lines[2:]...)
	}
	if end < len(r.entries) {
		lines = append(lines, dim.Render(fmt.Sprintf("  ↓ 还有 %d 个会话", len(r.entries)-end)))
	}

	content := strings.Join(lines, "\n")

	// 裸 box（不含 Place 居中，由 layout.go overlayAt 定位）
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorBrand).
		Padding(1, 2).
		Width(r.boxWidth()).
		Render(content)
}

// boxWidth 返回 box 渲染宽度
func (r ResumeSelector) boxWidth() int {
	return min(72, max(48, r.width-16))
}
