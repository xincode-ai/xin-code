package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Layout 五层布局管理器
// 真 overlay 实现：主区不被 overlay/bottomFloat 挤压。
//   1. Main        — 主滚动区（transcript），占满剩余高度
//   2. Bottom      — 底部固定区（Composer）
//   3. BottomFloat — 底部浮层（权限确认、spinner），覆盖在 Main 底部
//   4. Overlay     — 覆盖层（ResumeSelector 等），居中覆盖在 Main 上
//   5. Modal       — 模态层（diff/panel），居中覆盖全屏，保留背景感
type Layout struct {
	width  int
	height int
}

// LayerContent 各层内容
type LayerContent struct {
	Main        string
	Bottom      string
	BottomFloat string
	Overlay     string
	Modal       string // 已含 lipgloss.Place 定位的完整帧
}

// Render 按层级合成最终视图
func (l *Layout) Render(content LayerContent) string {
	bottomH := lipgloss.Height(content.Bottom)
	mainH := l.height - bottomH
	if mainH < 4 {
		mainH = 4
	}

	// 主区渲染到精确行数
	mainRendered := padOrTruncateHeight(content.Main, l.width, mainH)
	// 拼接基础帧：主区 + 底部
	base := mainRendered + "\n" + content.Bottom

	// BottomFloat 覆盖在 Main 区底部（从底部向上）
	if content.BottomFloat != "" {
		floatH := lipgloss.Height(content.BottomFloat)
		startRow := mainH - floatH
		if startRow < 0 {
			startRow = 0
		}
		base = overlayAt(base, content.BottomFloat, -1, startRow)
	}

	// Overlay 覆盖在 Main 区中部（居中放置，只覆盖 box 区域，不抹背景）
	if content.Overlay != "" {
		overlayH := lipgloss.Height(content.Overlay)
		overlayW := lipgloss.Width(content.Overlay)
		// 垂直居中
		startRow := (mainH - overlayH) / 2
		if startRow < 1 {
			startRow = 1
		}
		// 水平居中（col > 0 触发 placeOnLine，只覆盖 box 区域）
		startCol := (l.width - overlayW) / 2
		if startCol < 0 {
			startCol = 0
		}
		base = overlayAt(base, content.Overlay, startCol, startRow)
	}

	// Modal 覆盖全屏（保留背景感：modal 组件自带 Place 定位）
	if content.Modal != "" {
		// modal 已经是全屏定位的帧，直接叠加到 base 上
		// 用 modal 的非空行覆盖 base 的对应行
		base = overlayNonEmpty(base, content.Modal, l.height)
	}

	return base
}

// MainHeight 返回主区域可用高度
func (l *Layout) MainHeight(bottomContent string) int {
	bottomH := lipgloss.Height(bottomContent)
	mainH := l.height - bottomH
	if mainH < 4 {
		mainH = 4
	}
	return mainH
}

// padOrTruncateHeight 将内容精确填充/截断到指定行数
func padOrTruncateHeight(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

// overlayAt 将 fg 覆盖到 bg 的指定位置
// col == -1: 整行替换（BottomFloat 等全宽覆盖场景）
// col >= 0: 列级放置（overlay/modal 局部覆盖，保留前后背景）
func overlayAt(bg, fg string, col, row int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	for i, fgLine := range fgLines {
		targetRow := row + i
		if targetRow < 0 || targetRow >= len(bgLines) {
			continue
		}
		if col < 0 {
			bgLines[targetRow] = fgLine
		} else {
			bgLines[targetRow] = placeOnLine(bgLines[targetRow], fgLine, col)
		}
	}
	return strings.Join(bgLines, "\n")
}

// overlayNonEmpty 将 modal 帧覆盖到 base 上，只替换 modal 中非空白行
// 这样 base（transcript + composer）在 modal 边框外仍然可见
func overlayNonEmpty(base, modal string, height int) string {
	baseLines := strings.Split(base, "\n")
	modalLines := strings.Split(modal, "\n")

	// 确保两者行数一致
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}
	for len(modalLines) < height {
		modalLines = append(modalLines, "")
	}

	for i := 0; i < height && i < len(modalLines); i++ {
		if strings.TrimSpace(modalLines[i]) != "" {
			if i < len(baseLines) {
				baseLines[i] = modalLines[i]
			}
		}
	}
	// 截断到 height
	if len(baseLines) > height {
		baseLines = baseLines[:height]
	}
	return strings.Join(baseLines, "\n")
}

// placeOnLine 将 fg 放置到 bg 行的指定列位置（ANSI-safe）
// 使用 ansi.Truncate/TruncateLeft 而非 []rune 切片，避免截断 ANSI 转义序列
func placeOnLine(bgLine, fgLine string, col int) string {
	bgWidth := lipgloss.Width(bgLine)
	fgWidth := lipgloss.Width(fgLine)

	// 确保背景行够宽
	if col > bgWidth {
		bgLine += strings.Repeat(" ", col-bgWidth)
		bgWidth = col
	}

	// ANSI-safe 截取前缀（前 col 个可见列）
	prefix := ansi.Truncate(bgLine, col, "")
	prefixWidth := lipgloss.Width(prefix)
	if prefixWidth < col {
		prefix += strings.Repeat(" ", col-prefixWidth)
	}

	// ANSI-safe 截取后缀（跳过前 col+fgWidth 个可见列）
	endCol := col + fgWidth
	suffix := ""
	if endCol < bgWidth {
		suffix = ansi.TruncateLeft(bgLine, endCol, "")
	}

	return prefix + fgLine + suffix
}
