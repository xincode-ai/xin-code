package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/xincode-ai/xin-code/internal/cost"
)

// StatusBar 状态栏组件
type StatusBar struct {
	width      int
	model      string
	tracker    *cost.Tracker
	maxContext int // 最大上下文 token 数
}

// NewStatusBar 创建状态栏
func NewStatusBar(model string, tracker *cost.Tracker, maxContext int) StatusBar {
	return StatusBar{
		model:      model,
		tracker:    tracker,
		maxContext:  maxContext,
	}
}

func (s StatusBar) Init() tea.Cmd { return nil }

func (s StatusBar) Update(msg tea.Msg) (StatusBar, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
	}
	return s, nil
}

func (s StatusBar) View() string {
	if s.width == 0 {
		return ""
	}

	// 品牌标识
	brand := StyleBrand.Render("⚡ XIN CODE")

	// 模型
	model := StyleModel.Render(s.model)

	// 费用
	costStr := StyleCost.Render(s.tracker.CostString())

	// 上下文进度条
	ctxBar := s.renderContextBar()

	// 组装：品牌 | 模型 | 费用 | 进度条
	left := fmt.Sprintf("%s │ %s │ %s", brand, model, costStr)
	right := ctxBar

	// 计算填充宽度
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := s.width - leftW - rightW - 2 // 两侧各留 1 字符
	if gap < 1 {
		gap = 1
	}

	bar := StyleStatusBar.Width(s.width).Render(
		left + strings.Repeat(" ", gap) + right,
	)
	return bar
}

func (s StatusBar) renderContextBar() string {
	if s.maxContext <= 0 {
		return ""
	}

	// 直接从 Tracker 获取 token 数，避免重复计数
	usedTokens := s.tracker.TotalTokens()

	percent := float64(usedTokens) / float64(s.maxContext) * 100
	if percent > 100 {
		percent = 100
	}

	// 进度条：10 个字符宽
	barWidth := 10
	filled := int(percent / 100 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	color := ContextColor(percent)
	filledStyle := lipgloss.NewStyle().Foreground(color)
	emptyStyle := lipgloss.NewStyle().Foreground(ColorTextDim)

	bar := filledStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", barWidth-filled))

	label := lipgloss.NewStyle().Foreground(color).Render(fmt.Sprintf("%.0f%%", percent))

	return fmt.Sprintf("ctx [%s] %s", bar, label)
}
