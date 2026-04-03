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
