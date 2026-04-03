package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/xincode-ai/xin-code/internal/cost"
)

// ComposerConfig 底部 Composer 渲染所需数据
type ComposerConfig struct {
	Model      string
	PermMode   string
	MouseMode  MouseMode
	ReadOnly   bool
	Tracker    *cost.Tracker
	MaxContext int
	WorkDir    string
}

// renderComposer 渲染 Claude 风格的双分隔线 Composer
// 结构：
//   ─── 上分隔线 ───
//   prompt 输入区
//   ─── 下分隔线 ───
//   状态栏（左：模型 · 费用  右：上下文 · 权限模式）
func renderComposer(inputView string, cfg ComposerConfig, width int) string {
	borderStyle := lipgloss.NewStyle().
		Foreground(ColorInputBorder)
	separator := borderStyle.Render(strings.Repeat("─", width))

	// 状态栏：左右分区
	statusLine := renderComposerStatus(cfg, width)

	return strings.Join([]string{
		separator,
		inputView,
		separator,
		statusLine,
	}, "\n")
}

// renderComposerStatus 渲染 Composer 底部状态栏（左右分区）
func renderComposerStatus(cfg ComposerConfig, width int) string {
	dim := StyleDim

	// 左侧：模型 · 费用 [· 只读]
	leftParts := []string{
		dim.Render(shortModelName(cfg.Model)),
		dim.Render(cfg.Tracker.CostString()),
	}
	if cfg.ReadOnly {
		leftParts = append(leftParts, lipgloss.NewStyle().Foreground(ColorWarning).Render("只读"))
	}
	left := strings.Join(leftParts, " · ")

	// 右侧：上下文 · 权限模式
	var rightParts []string

	// 上下文使用量（带颜色分级）
	if cfg.MaxContext > 0 {
		used := cfg.Tracker.TotalTokens()
		percent := float64(used) / float64(cfg.MaxContext) * 100
		if percent > 100 {
			percent = 100
		}
		ctxColor := ContextColor(percent)
		ctxStyle := lipgloss.NewStyle().Foreground(ctxColor)
		rightParts = append(rightParts, ctxStyle.Render(fmt.Sprintf("%d%%", int(percent+0.5))))
	}

	// 鼠标模式（带切换提示）
	mouseLabel := cfg.MouseMode.Label()
	rightParts = append(rightParts, dim.Render(mouseLabel+" ^Y"))

	// 权限模式
	permLabels := map[string]string{
		"bypass":      "bypass",
		"acceptEdits": "auto-edit",
		"default":     "默认确认",
		"plan":        "plan-only",
		"interactive": "全部确认",
	}
	if label, ok := permLabels[cfg.PermMode]; ok {
		rightParts = append(rightParts, dim.Render(label))
	}

	right := strings.Join(rightParts, " · ")

	// 左右对齐：用空格填充
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := width - leftW - rightW - 2
	if gap < 1 {
		gap = 1
	}

	return " " + left + strings.Repeat(" ", gap) + right + " "
}
