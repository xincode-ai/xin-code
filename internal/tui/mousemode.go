package tui

// MouseMode 鼠标模式
type MouseMode int

const (
	// MouseModeBrowse 浏览模式：启用鼠标报告，支持滚轮滚动和点击交互
	MouseModeBrowse MouseMode = iota
	// MouseModeSelect 复制模式：关闭鼠标报告，允许终端原生拖拽选择复制
	MouseModeSelect
)

// Label 返回模式的中文标签（状态栏显示用）
func (m MouseMode) Label() string {
	switch m {
	case MouseModeSelect:
		return "复制"
	default:
		return "浏览"
	}
}
