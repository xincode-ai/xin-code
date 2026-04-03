package tui

import "github.com/xincode-ai/xin-code/internal/provider"

// 模型代理发往终端界面的消息类型

// MsgTextDelta 模型回复文本增量
type MsgTextDelta struct {
	Text string
}

// MsgThinking 模型思考提示
type MsgThinking struct {
	Text string
}

// MsgToolStart 工具开始执行
type MsgToolStart struct {
	ID    string
	Name  string
	Input string
}

// MsgToolDone 工具执行完成
type MsgToolDone struct {
	ID      string
	Name    string
	Output  string
	IsError bool
}

// MsgUsage Token 使用量更新
type MsgUsage struct {
	Usage *provider.Usage
}

// MsgResponseDone 一轮 API 响应完成
type MsgResponseDone struct{}

// MsgAgentDone 模型代理循环完成（无更多工具调用）
type MsgAgentDone struct {
	Err error
}

// MsgPermissionRequest 权限请求
type MsgPermissionRequest struct {
	ID       string
	ToolName string
	Input    string
	Response chan PermissionResponse
}

// PermissionResponse 权限响应
type PermissionResponse int

const (
	PermAllow PermissionResponse = iota
	PermDeny
	PermAlways
	PermNever
)

// MsgAskUser 询问工具请求用户输入
type MsgAskUser struct {
	Question string
	Response chan string
}

// MsgDiffPreview 差异预览确认请求
type MsgDiffPreview struct {
	Path     string
	DiffText string
	Response chan bool
}

// MsgError 错误消息
type MsgError struct {
	Err error
}

// MsgSystemNotice 系统通知（如自动压缩提示）
type MsgSystemNotice struct {
	Text string
}

// MsgSubAgentStart 子 agent 开始执行
type MsgSubAgentStart struct {
	ID          string
	Description string
}

// MsgSubAgentDone 子 agent 执行完成
type MsgSubAgentDone struct {
	ID          string
	Description string
	Result      string // 摘要（截取前 200 字符）
}

// MsgWindowSize 窗口大小变化
type MsgWindowSize struct {
	Width  int
	Height int
}
