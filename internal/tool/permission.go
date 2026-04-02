package tool

// CheckResult 权限检查三态结果
type CheckResult int

const (
	ResultAllow   CheckResult = iota // 允许执行
	ResultDeny                       // 拒绝执行
	ResultNeedAsk                    // 需要询问用户
)

// PermissionChecker 权限检查接口
type PermissionChecker interface {
	Check(toolName string, isReadOnly bool) (CheckResult, string)
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

// SimplePermissionChecker 基于模式的权限检查器
type SimplePermissionChecker struct {
	Mode PermissionMode
}

func (c *SimplePermissionChecker) Check(toolName string, isReadOnly bool) (CheckResult, string) {
	switch c.Mode {
	case ModeBypass:
		return ResultAllow, ""

	case ModeAcceptEdits:
		// 只读工具 + 文件写入工具自动放行，Bash 等需要询问
		if isReadOnly || toolName == "Write" || toolName == "Edit" {
			return ResultAllow, ""
		}
		return ResultNeedAsk, "acceptEdits mode: non-file tool requires confirmation"

	case ModeDefault:
		if isReadOnly {
			return ResultAllow, ""
		}
		return ResultNeedAsk, "default mode: write tool requires confirmation"

	case ModePlan:
		if isReadOnly {
			return ResultAllow, ""
		}
		return ResultDeny, "plan mode: write operations are not allowed"

	case ModeInteractive:
		return ResultNeedAsk, "interactive mode: all tools require confirmation"

	default:
		return ResultAllow, ""
	}
}
