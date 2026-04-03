package builtin

import (
	"context"

	agentPkg "github.com/xincode-ai/xin-code/internal/agent"
	"github.com/xincode-ai/xin-code/internal/cost"
	"github.com/xincode-ai/xin-code/internal/provider"
	"github.com/xincode-ai/xin-code/internal/tool"
)

// RegisterConfig 内置工具注册配置
type RegisterConfig struct {
	AskFunc     func(ctx context.Context, question string) (string, error)
	ConfirmFunc func(ctx context.Context, path string, diffText string) (bool, error)

	// SubAgent 相关依赖（Provider/Model 用 getter 动态读取，/resume 后自动跟随）
	CurrentProvider func() provider.Provider
	CurrentModel    func() string
	Permission      tool.PermissionChecker
	Tracker         *cost.Tracker
	MaxTokens       int
	SendMsg         func(interface{})
	SubAgentReg     *agentPkg.SubAgentRegistry
}

// RegisterAll 注册所有内置工具
func RegisterAll(reg *tool.Registry, cfg RegisterConfig) {
	// 读取类工具
	reg.Register(&ReadTool{})
	reg.Register(&GlobTool{})
	reg.Register(&GrepTool{})
	reg.Register(&WebFetchTool{})
	reg.Register(&WebSearchTool{})

	// 写入类工具
	reg.Register(&BashTool{})
	reg.Register(&WriteTool{})
	reg.Register(&EditTool{ConfirmFunc: cfg.ConfirmFunc})

	// 交互类工具
	reg.Register(&AskUserTool{AskFunc: cfg.AskFunc})
	reg.Register(&TaskTool{})

	// SubAgent 工具（Provider/Model 通过 getter 动态读取）
	reg.Register(&AgentTool{
		CurrentProvider: cfg.CurrentProvider,
		CurrentModel:    cfg.CurrentModel,
		Tools:           reg,
		Permission:      cfg.Permission,
		Tracker:         cfg.Tracker,
		MaxTokens:       cfg.MaxTokens,
		SendMsg:         cfg.SendMsg,
		SubAgentReg:     cfg.SubAgentReg,
	})
	reg.Register(&SendMessageTool{
		SubAgentReg: cfg.SubAgentReg,
	})
}
