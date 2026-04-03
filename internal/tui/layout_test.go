package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// ansiFragmentRe 匹配被截断的 ANSI 残片：
// - 裸数字+m（如 55m、;255m）不在 ESC[ 序列内
// - 不完整的 ESC[ 序列（如 \x1b[38;2 后没有 m）
var ansiFragmentRe = regexp.MustCompile(
	// 匹配不以 ESC[ 开头的裸 SGR 残片：数字/分号 + m
	`(?:^|[^\x1b])(?:\x1b\[)?[\d;]*m` +
		`|` +
		// 匹配 ESC[ 开了但没闭合的序列（到行尾都没有字母终止）
		`\x1b\[[\d;]*$`,
)

// hasANSIFragment 检测字符串中是否有被截断的 ANSI 残片
func hasANSIFragment(s string) bool {
	// 更简单的检测方法：把所有合法的 ANSI 序列剥掉，看剩下的有没有残片
	// 合法序列格式：ESC [ (数字/分号)* 字母
	validCSI := regexp.MustCompile(`\x1b\[[\d;]*[A-Za-z]`)
	stripped := validCSI.ReplaceAllString(s, "")
	// 剥掉后不应该残留任何 ESC 或孤立的 SGR 终止符 m
	if strings.Contains(stripped, "\x1b") {
		return true // 不完整的 ESC 序列
	}
	// 检查是否有裸露的 ;数字m 或 数字m 残片（不在 ESC[ 序列内）
	// 这些是 ANSI 序列被从中间切断后的典型表现
	bareFragment := regexp.MustCompile(`(?:^|[^[\x1b])(\d+m|;\d+m)`)
	return bareFragment.MatchString(stripped)
}

// TestPlaceOnLine_ANSISafe 验证 placeOnLine 不会截断 ANSI 转义序列
func TestPlaceOnLine_ANSISafe(t *testing.T) {
	// 构造带丰富 ANSI 样式的背景行
	bgStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF6437")).
		Background(lipgloss.Color("#1A1A2E")).
		Bold(true)
	bg := bgStyle.Render("████████████████████████████████████████████████████████████████████████████████")

	// 构造带边框样式的前景行（模拟 overlay box 的一行）
	fgStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00FF88")).
		BorderForeground(lipgloss.Color("#7B61FF"))
	fg := fgStyle.Render("│ 恢复会话  ↑/↓ 选择 │")

	bgWidth := lipgloss.Width(bg)
	fgWidth := lipgloss.Width(fg)

	// 居中放置
	col := (bgWidth - fgWidth) / 2
	if col < 0 {
		col = 0
	}

	result := placeOnLine(bg, fg, col)

	// 验证 1：输出中不应有 ANSI 残片
	if hasANSIFragment(result) {
		t.Errorf("placeOnLine 产生了 ANSI 残片:\n%q", result)
	}

	// 验证 2：可见宽度应该正确
	resultWidth := lipgloss.Width(result)
	if resultWidth < bgWidth {
		t.Errorf("结果可见宽度 %d < 背景宽度 %d", resultWidth, bgWidth)
	}
}

// TestPlaceOnLine_EdgePositions 验证边界情况
func TestPlaceOnLine_EdgePositions(t *testing.T) {
	bg := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("ABCDEFGHIJ")
	fg := lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("XY")

	tests := []struct {
		name string
		col  int
	}{
		{"左边界", 0},
		{"中间", 4},
		{"右侧", 8},
		{"超出背景", 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := placeOnLine(bg, fg, tt.col)
			if hasANSIFragment(result) {
				t.Errorf("col=%d 产生了 ANSI 残片:\n%q", tt.col, result)
			}
		})
	}
}

// TestPlaceOnLine_Complex256Color 验证 256 色和 RGB 色不会被截断
func TestPlaceOnLine_Complex256Color(t *testing.T) {
	// 用 lipgloss 构造 True Color (24-bit) 样式，产生 \x1b[38;2;R;G;Bm 序列
	bg := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF6437")). // 产生 38;2;255;100;55m 类似序列
		Background(lipgloss.Color("#1E1E3F")).
		Render("The quick brown fox jumps over the lazy dog and some more text here")

	fg := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7B61FF")).
		Bold(true).
		Render("OVERLAY")

	// 尝试多个放置位置
	for col := 0; col < 50; col += 3 {
		result := placeOnLine(bg, fg, col)
		if hasANSIFragment(result) {
			t.Errorf("col=%d 产生了 ANSI 残片:\n%q", col, result)
		}
	}
}

// TestPlaceOnLine_PreservesContent 验证内容正确保留
func TestPlaceOnLine_PreservesContent(t *testing.T) {
	bg := "Hello, World! How are you?"
	fg := "XXX"

	result := placeOnLine(bg, fg, 7)
	// 前缀 "Hello, " (7 chars) + "XXX" + "ld! How are you?" (从位置10开始)
	expected := "Hello, XXXld! How are you?"
	if result != expected {
		t.Errorf("期望 %q，得到 %q", expected, result)
	}
}

// TestPlaceOnLine_ChineseChars 验证中文（双宽字符）处理
func TestPlaceOnLine_ChineseChars(t *testing.T) {
	bg := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("你好世界测试文本")
	fg := lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("OK")

	// 中文字符宽度为 2，在偶数列放置
	result := placeOnLine(bg, fg, 4)
	if hasANSIFragment(result) {
		t.Errorf("中文背景 + overlay 产生了 ANSI 残片:\n%q", result)
	}
}

// TestOverlayAt_FullIntegration 模拟真实的 overlay 合成场景
func TestOverlayAt_FullIntegration(t *testing.T) {
	// 模拟一个有丰富样式的 transcript 背景
	lines := make([]string, 20)
	styles := []lipgloss.Style{
		lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6437")).Bold(true),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF88")).Italic(true),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#7B61FF")).Background(lipgloss.Color("#1A1A2E")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")),
	}
	for i := range lines {
		s := styles[i%len(styles)]
		lines[i] = s.Render(strings.Repeat("█", 80))
	}
	bg := strings.Join(lines, "\n")

	// 模拟一个 overlay box（类似 ResumeSelector）
	boxStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7B61FF")).
		Padding(1, 2).
		Width(40)

	boxContent := lipgloss.NewStyle().Bold(true).Render("恢复会话") + "\n" +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF88")).Render("❯ session-1  gpt-4  5轮") + "\n" +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("  session-2  claude  3轮")

	overlay := boxStyle.Render(boxContent)

	// 居中放置
	overlayW := lipgloss.Width(overlay)
	col := (80 - overlayW) / 2
	row := 5

	result := overlayAt(bg, overlay, col, row)

	// 逐行检查无 ANSI 残片
	resultLines := strings.Split(result, "\n")
	for i, line := range resultLines {
		if hasANSIFragment(line) {
			t.Errorf("第 %d 行有 ANSI 残片:\n%q", i, line)
		}
	}
}
