package tui

import "github.com/charmbracelet/lipgloss"

// 品牌色系
var (
	// 主色调
	ColorBrand   = lipgloss.Color("#7C3AED") // 紫色品牌色
	ColorAccent  = lipgloss.Color("#06B6D4") // 青色点缀

	// 语义色
	ColorSuccess = lipgloss.Color("#22C55E") // 绿色
	ColorWarning = lipgloss.Color("#EAB308") // 黄色
	ColorError   = lipgloss.Color("#EF4444") // 红色
	ColorInfo    = lipgloss.Color("#3B82F6") // 蓝色

	// 文本色
	ColorText     = lipgloss.Color("#E2E8F0") // 浅灰文本
	ColorTextDim  = lipgloss.Color("#64748B") // 暗灰辅助文本
	ColorTextBold = lipgloss.Color("#F8FAFC") // 高亮文本

	// 背景色
	ColorBg       = lipgloss.Color("#0F172A") // 深色背景
	ColorBgAlt    = lipgloss.Color("#1E293B") // 交替背景

	// 上下文进度条颜色
	ColorCtxLow  = lipgloss.Color("#22C55E") // <60% 绿色
	ColorCtxMid  = lipgloss.Color("#EAB308") // 60-80% 黄色
	ColorCtxHigh = lipgloss.Color("#EF4444") // >80% 红色
)

// 样式定义
var (
	// 状态栏
	StyleStatusBar = lipgloss.NewStyle().
		Background(lipgloss.Color("#1E293B")).
		Foreground(ColorText).
		Padding(0, 1)

	StyleBrand = lipgloss.NewStyle().
		Foreground(ColorBrand).
		Bold(true)

	StyleModel = lipgloss.NewStyle().
		Foreground(ColorAccent)

	StyleCost = lipgloss.NewStyle().
		Foreground(ColorWarning)

	// 对话区域
	StyleUserMsg = lipgloss.NewStyle().
		Foreground(ColorTextBold).
		Bold(true)

	StyleUserPrefix = lipgloss.NewStyle().
		Foreground(ColorBrand).
		Bold(true)

	StyleAIMsg = lipgloss.NewStyle().
		Foreground(ColorText)

	StyleThinking = lipgloss.NewStyle().
		Foreground(ColorTextDim).
		Italic(true)

	// 工具相关
	StyleToolName = lipgloss.NewStyle().
		Foreground(ColorAccent).
		Bold(true)

	StyleToolRunning = lipgloss.NewStyle().
		Foreground(ColorInfo)

	StyleToolSuccess = lipgloss.NewStyle().
		Foreground(ColorSuccess)

	StyleToolError = lipgloss.NewStyle().
		Foreground(ColorError)

	StyleToolOutput = lipgloss.NewStyle().
		Foreground(ColorTextDim)

	// 权限对话框
	StylePermBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorWarning).
		Padding(0, 1)

	StylePermTitle = lipgloss.NewStyle().
		Foreground(ColorWarning).
		Bold(true)

	// Diff 预览
	StyleDiffAdd = lipgloss.NewStyle().
		Foreground(ColorSuccess)

	StyleDiffDel = lipgloss.NewStyle().
		Foreground(ColorError)

	StyleDiffCtx = lipgloss.NewStyle().
		Foreground(ColorTextDim)

	StyleDiffHeader = lipgloss.NewStyle().
		Foreground(ColorInfo).
		Bold(true)

	// 输入框
	StyleInputBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBrand).
		Padding(0, 1)

	StyleInputPrompt = lipgloss.NewStyle().
		Foreground(ColorBrand).
		Bold(true)

	// 提示文本
	StyleHint = lipgloss.NewStyle().
		Foreground(ColorTextDim)

	// 错误消息
	StyleErrorMsg = lipgloss.NewStyle().
		Foreground(ColorError).
		Bold(true)
)

// ContextColor 根据上下文使用百分比返回对应颜色
func ContextColor(percent float64) lipgloss.Color {
	switch {
	case percent >= 80:
		return ColorCtxHigh
	case percent >= 60:
		return ColorCtxMid
	default:
		return ColorCtxLow
	}
}
