package tui

import (
	"os"
	"runtime"

	"github.com/charmbracelet/lipgloss"
)

func init() {
	// 预设暗色背景，避免 Lipgloss 在运行时查询终端背景色（OSC 11），
	// 查询响应会被 Bubble Tea 的输入读取器误收到。
	lipgloss.SetHasDarkBackground(true)

	// 同时强制启用彩色输出，避免在部分终端里回退到纯文本样式。
	os.Setenv("CLICOLOR_FORCE", "1")
}

// BlackCircle 返回平台对应的实心圆符号（macOS 用 Unicode 录制键符号）
func BlackCircle() string {
	if runtime.GOOS == "darwin" {
		return "⏺"
	}
	return "●"
}

// CC 符号体系
const (
	SymThinking   = "∴"
	SymResponse   = "⎿"
	SymPause      = "⏸"
	SymUserPrompt = "❯"
)

// 颜色令牌（CC dark theme 精确 RGB）
var (
	// 品牌色
	ColorBrand      = lipgloss.Color("#D77757") // CC: rgb(215,119,87)
	ColorBrandDim   = lipgloss.Color("#EB9F7F") // CC: rgb(235,159,127) 闪烁态

	// 语义色
	ColorSuccess = lipgloss.Color("#2CB74F")
	ColorError   = lipgloss.Color("#CC3333")
	ColorWarning = lipgloss.Color("#DCA032")
	ColorPerm    = lipgloss.Color("#B1B9F9") // CC: rgb(177,185,249) 权限蓝紫

	// 文本色
	ColorText    = lipgloss.Color("#FFFFFF")
	ColorTextDim = lipgloss.Color("#A0A0A0")
	ColorSubtle  = lipgloss.Color("#6E6E6E")

	// 边框色
	ColorInputBorder = lipgloss.Color("#888888") // CC: rgb(136,136,136)

	// Diff 色
	ColorDiffAdd = lipgloss.Color("#4ADE80")
	ColorDiffDel = lipgloss.Color("#FB7185")

	// 上下文进度条
	ColorCtxLow  = lipgloss.Color("#34D399")
	ColorCtxMid  = lipgloss.Color("#F59E0B")
	ColorCtxHigh = lipgloss.Color("#F87171")
)

// 组件样式
var (
	// ⎿ 前缀及 assistant 响应区文本
	StyleMsgResponse = lipgloss.NewStyle().
				Foreground(ColorTextDim)

	// 用户输入文本
	StyleUserText = lipgloss.NewStyle().
			Foreground(ColorText).
			Bold(true)

	// ∴ 思考中文本
	StyleThinking = lipgloss.NewStyle().
			Foreground(ColorTextDim).
			Italic(true)

	// 工具名称
	StyleToolName = lipgloss.NewStyle().
			Bold(true)

	// 工具输出内容
	StyleToolOutput = lipgloss.NewStyle().
			Foreground(ColorTextDim)

	// 错误文本
	StyleError = lipgloss.NewStyle().
			Foreground(ColorError)

	// Spinner 动画
	StyleSpinner = lipgloss.NewStyle().
			Foreground(ColorBrand)

	// 通用 dim 文本
	StyleDim = lipgloss.NewStyle().
			Foreground(ColorTextDim)

	// 输入框提示符
	StyleInputPrompt = lipgloss.NewStyle().
				Foreground(ColorTextDim)

	// 权限请求标题
	StylePermTitle = lipgloss.NewStyle().
			Foreground(ColorPerm).
			Bold(true)

	// Diff 样式
	StyleDiffAdd = lipgloss.NewStyle().
			Foreground(ColorDiffAdd)

	StyleDiffDel = lipgloss.NewStyle().
			Foreground(ColorDiffDel)

	StyleDiffHeader = lipgloss.NewStyle().
				Foreground(ColorPerm).
				Bold(true)

	StyleDiffCtx = lipgloss.NewStyle().
			Foreground(ColorTextDim)

	// Footer 通用样式
	StyleFooter = lipgloss.NewStyle().
			Foreground(ColorTextDim)
)

// ThemeMode 主题模式
type ThemeMode string

const (
	ThemeDark  ThemeMode = "dark"
	ThemeLight ThemeMode = "light"
)

// currentTheme 当前主题模式
var currentTheme ThemeMode = ThemeDark

// CurrentTheme 返回当前主题模式
func CurrentTheme() ThemeMode {
	return currentTheme
}

// SetTheme 切换主题并重建所有样式
func SetTheme(mode ThemeMode) {
	currentTheme = mode
	if mode == ThemeLight {
		lipgloss.SetHasDarkBackground(false)

		// 浅色主题颜色
		ColorBrand = lipgloss.Color("#C4603D")
		ColorBrandDim = lipgloss.Color("#D98A6C")
		ColorSuccess = lipgloss.Color("#1E8A3C")
		ColorError = lipgloss.Color("#CC3333")
		ColorWarning = lipgloss.Color("#B8860B")
		ColorPerm = lipgloss.Color("#6366F1")
		ColorText = lipgloss.Color("#1A1A1A")
		ColorTextDim = lipgloss.Color("#666666")
		ColorSubtle = lipgloss.Color("#999999")
		ColorInputBorder = lipgloss.Color("#CCCCCC")
		ColorDiffAdd = lipgloss.Color("#166534")
		ColorDiffDel = lipgloss.Color("#991B1B")
		ColorCtxLow = lipgloss.Color("#059669")
		ColorCtxMid = lipgloss.Color("#D97706")
		ColorCtxHigh = lipgloss.Color("#DC2626")
	} else {
		lipgloss.SetHasDarkBackground(true)

		// 恢复暗色默认
		ColorBrand = lipgloss.Color("#D77757")
		ColorBrandDim = lipgloss.Color("#EB9F7F")
		ColorSuccess = lipgloss.Color("#2CB74F")
		ColorError = lipgloss.Color("#CC3333")
		ColorWarning = lipgloss.Color("#DCA032")
		ColorPerm = lipgloss.Color("#B1B9F9")
		ColorText = lipgloss.Color("#FFFFFF")
		ColorTextDim = lipgloss.Color("#A0A0A0")
		ColorSubtle = lipgloss.Color("#6E6E6E")
		ColorInputBorder = lipgloss.Color("#888888")
		ColorDiffAdd = lipgloss.Color("#4ADE80")
		ColorDiffDel = lipgloss.Color("#FB7185")
		ColorCtxLow = lipgloss.Color("#34D399")
		ColorCtxMid = lipgloss.Color("#F59E0B")
		ColorCtxHigh = lipgloss.Color("#F87171")
	}
	rebuildStyles()
}

// rebuildStyles 颜色变量更新后重新赋值所有样式
func rebuildStyles() {
	StyleMsgResponse = lipgloss.NewStyle().Foreground(ColorTextDim)
	StyleUserText = lipgloss.NewStyle().Foreground(ColorText).Bold(true)
	StyleThinking = lipgloss.NewStyle().Foreground(ColorTextDim).Italic(true)
	StyleToolName = lipgloss.NewStyle().Bold(true)
	StyleToolOutput = lipgloss.NewStyle().Foreground(ColorTextDim)
	StyleError = lipgloss.NewStyle().Foreground(ColorError)
	StyleSpinner = lipgloss.NewStyle().Foreground(ColorBrand)
	StyleDim = lipgloss.NewStyle().Foreground(ColorTextDim)
	StyleInputPrompt = lipgloss.NewStyle().Foreground(ColorTextDim)
	StylePermTitle = lipgloss.NewStyle().Foreground(ColorPerm).Bold(true)
	StyleDiffAdd = lipgloss.NewStyle().Foreground(ColorDiffAdd)
	StyleDiffDel = lipgloss.NewStyle().Foreground(ColorDiffDel)
	StyleDiffHeader = lipgloss.NewStyle().Foreground(ColorPerm).Bold(true)
	StyleDiffCtx = lipgloss.NewStyle().Foreground(ColorTextDim)
	StyleFooter = lipgloss.NewStyle().Foreground(ColorTextDim)
}

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
