package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Layout 五层布局管理器
// 参考 Claude Code 的布局架构：
//   1. Main        — 主滚动区（transcript）
//   2. Bottom      — 底部固定区（输入框 + 状态栏）
//   3. BottomFloat — 底部浮层（权限确认、spinner）
//   4. Overlay     — 覆盖层（命令面板等，由 input.go 内部处理）
//   5. Modal       — 模态层（diff 预览，独占全屏）
type Layout struct {
	width  int
	height int
}

// LayerContent 各层内容
type LayerContent struct {
	Main        string // 主滚动区（transcript）
	Bottom      string // 底部固定区（输入框 + 状态栏）
	BottomFloat string // 底部浮层（权限确认等）
	Overlay     string // 覆盖层（命令面板等）
	Modal       string // 模态层（diff 等）
}

// Render 按层级合成最终视图
func (l *Layout) Render(content LayerContent) string {
	// Modal 独占全屏
	if content.Modal != "" {
		return content.Modal
	}

	// 组装：主区域 + 浮层 + 底部
	var parts []string
	parts = append(parts, content.Main)
	if content.BottomFloat != "" {
		parts = append(parts, content.BottomFloat)
	}
	parts = append(parts, content.Bottom)

	return strings.Join(parts, "\n")
}

// MainHeight 计算主区域可用高度
func (l *Layout) MainHeight(bottomContent string, floatContent string) int {
	bottomHeight := lipgloss.Height(bottomContent)
	floatHeight := 0
	if floatContent != "" {
		floatHeight = lipgloss.Height(floatContent)
	}

	mainHeight := l.height - bottomHeight - floatHeight - 1
	if mainHeight < 4 {
		mainHeight = 4
	}
	return mainHeight
}
