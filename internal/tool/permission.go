package tool

// PermissionChecker 权限检查接口
type PermissionChecker interface {
	Check(toolName string, isReadOnly bool) (allowed bool, reason string)
}

// PermissionMode 权限模式
type PermissionMode string

const (
	ModeBypass      PermissionMode = "bypass"
	ModeAcceptEdits PermissionMode = "acceptEdits"
	ModeDefault     PermissionMode = "default"
	ModePlan        PermissionMode = "plan"
	ModeInteractive PermissionMode = "interactive"
)

// SimplePermissionChecker 基于模式的简单权限检查器
// Phase 1 使用简单实现，Phase 2 加入规则系统和用户交互
type SimplePermissionChecker struct {
	Mode PermissionMode
}

func (c *SimplePermissionChecker) Check(toolName string, isReadOnly bool) (bool, string) {
	switch c.Mode {
	case ModeBypass:
		return true, ""
	case ModeAcceptEdits:
		// 文件操作自动放行，Bash 等需要检查
		if isReadOnly || toolName == "Write" || toolName == "Edit" {
			return true, ""
		}
		// Phase 2: 这里会弹出 TUI 确认对话框
		// Phase 1: 暂时自动放行
		return true, ""
	case ModeDefault:
		if isReadOnly {
			return true, ""
		}
		// Phase 1: 暂时自动放行写入工具
		// Phase 2: 弹出 TUI 确认
		return true, ""
	case ModePlan:
		if isReadOnly {
			return true, ""
		}
		return false, "plan mode: write operations are not allowed"
	case ModeInteractive:
		// Phase 2: 所有工具都弹框
		// Phase 1: 暂时自动放行
		return true, ""
	default:
		return true, ""
	}
}
