package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderStatusLine 渲染 CC 风格状态栏（单行，各项以 · 分隔）
func (a *App) renderStatusLine() string {
	parts := []string{}

	// 模型（简写）
	parts = append(parts, StyleDim.Render(shortModelName(a.model)))

	// 费用
	parts = append(parts, StyleDim.Render(a.tracker.CostString()))

	// 上下文使用量（带颜色分级）
	ctxPercent := a.contextPercent()
	ctxColor := ContextColor(float64(ctxPercent))
	ctxStyle := lipgloss.NewStyle().Foreground(ctxColor)
	parts = append(parts, ctxStyle.Render(fmt.Sprintf("%d%% 上下文", ctxPercent)))

	// 权限模式
	permLabels := map[string]string{
		"bypass":      "bypass",
		"acceptEdits": "auto-edit",
		"default":     "默认确认",
		"plan":        "plan-only",
		"interactive": "全部确认",
	}
	if label, ok := permLabels[a.permMode]; ok {
		parts = append(parts, StyleDim.Render(label))
	}

	return StyleFooter.Render(strings.Join(parts, " · "))
}
