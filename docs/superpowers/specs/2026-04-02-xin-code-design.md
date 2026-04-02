# Xin Code - 设计规格文档

> 用 Go 构建的开源终端 AI Agent 工具，全面对标 Claude Code + CodeAny，并在此基础上做出差异化改进。

**版本**: v0.1.0 (MVP)
**日期**: 2026-04-02
**作者**: 洋哥 + 小灵犀

---

## 一、项目定位

### 1.1 一句话描述

Xin Code 是一个用 Go 语言构建的开源终端 AI Agent 工具，支持多模型 Provider，提供实时费用面板、上下文可视化、Diff 预览等差异化体验。

### 1.2 设计宗旨

1. Claude Code 和 CodeAny 有的功能，Xin Code 都要有
2. 在此基础上做出更好的功能和体验改进

### 1.3 核心差异化

| 差异化能力 | Claude Code | CodeAny | Xin Code |
|-----------|------------|---------|----------|
| 多模型 Provider | Anthropic only | 假多模型（配置换 URL） | 真 Provider 接口，原生支持 Anthropic + OpenAI |
| CC OAuth 复用 | 自己用 | 不支持 | 读取 CC 的 Keychain token，订阅用户零成本 |
| 实时费用面板 | token 数小字显示 | 无 | 状态栏常驻 ¥/$ 实时费用 |
| 上下文可视化 | 文字警告"快满了" | 无 | 进度条 + 颜色渐变（绿→黄→红） |
| Diff 预览 | 文字确认，无可视化 Diff | 直接改文件 | 编辑前弹出彩色 Diff 预览面板，确认后写入 |
| 启动速度 | ~500ms (Node.js) | ~50ms (Go) | ~50ms (Go) |
| 分发方式 | npm install + Node 运行时 | 二进制下载 | brew install / 二进制 / go install，零依赖 |

### 1.4 目标用户

- 使用终端进行日常开发的工程师
- 已有 Claude Pro/Max 订阅，想要更灵活的工具
- 想使用多种 AI 模型的开发者
- 对费用敏感，需要实时成本感知的用户

---

## 二、技术选型

### 2.1 语言：Go

- 编译为单二进制，零依赖分发
- 启动速度 ~50ms
- goroutine 天然适合并发工具执行
- Charm.sh 生态提供成熟的终端 UI 方案

### 2.2 核心依赖

```
CLI 框架
  github.com/spf13/cobra

TUI 全家桶（Charm.sh）
  github.com/charmbracelet/bubbletea      # 终端 UI 框架（Elm 架构）
  github.com/charmbracelet/bubbles        # 预制组件（输入框、Spinner、表格）
  github.com/charmbracelet/lipgloss       # 样式系统
  github.com/charmbracelet/glamour        # Markdown 终端渲染

AI Provider SDK
  github.com/anthropics/anthropic-sdk-go   # Anthropic Claude
  github.com/openai/openai-go             # OpenAI GPT + 兼容端点

MCP
  github.com/mark3labs/mcp-go             # Model Context Protocol

工具链
  github.com/alecthomas/chroma/v2         # 代码语法高亮
  github.com/sergi/go-diff                # Diff 计算
  gopkg.in/yaml.v3                        # 配置解析
```

### 2.3 构建与发布

- **GoReleaser**: 多平台交叉编译 + GitHub Releases + Homebrew Tap
- **GitHub Actions**: CI（构建/测试/lint）+ CD（自动发布）
- **golangci-lint**: ~20 条规则（对标 charmbracelet/mods 配置）

---

## 三、项目结构

```
xin-code/
├── main.go                        # 最小入口（mods 风格）
├── agent.go                       # Agent 核心循环
├── config.go                      # 配置加载与合并
├── version.go                     # 版本信息（ldflags 注入）
│
├── internal/
│   ├── provider/                  # 多模型 Provider 抽象
│   │   ├── provider.go            #   Provider 接口 + 统一事件类型
│   │   ├── anthropic.go           #   Claude Provider（含 CC OAuth）
│   │   ├── openai.go              #   OpenAI Provider（含兼容端点兜底）
│   │   ├── message.go             #   统一消息格式
│   │   └── registry.go            #   Provider 注册 + 自动选择
│   │
│   ├── tool/                      # 工具系统
│   │   ├── tool.go                #   Tool 接口 + Registry
│   │   ├── executor.go            #   并发执行器（读并发/写串行）
│   │   ├── permission.go          #   五档权限 + 规则匹配
│   │   └── builtin/               #   20+ 个内置工具
│   │       ├── read.go            #   P0 文件读取
│   │       ├── write.go           #   P0 文件写入
│   │       ├── edit.go            #   P0 文件编辑（含 Diff 预览）
│   │       ├── bash.go            #   P0 Shell 命令执行
│   │       ├── glob.go            #   P0 文件名模式匹配
│   │       ├── grep.go            #   P0 文件内容搜索
│   │       ├── webfetch.go        #   P0 网页抓取
│   │       ├── websearch.go       #   P0 网络搜索
│   │       ├── agent.go           #   P0 子 Agent 派生
│   │       ├── mcp.go             #   P0 MCP 工具桥接
│   │       ├── askuser.go         #   P0 向用户提问（Agent 循环关键）
│   │       ├── task.go            #   P0 任务管理（Create/Get/List/Update/Stop）
│   │       ├── sendmessage.go     #   P1 Agent 间通信
│   │       ├── plan.go            #   P1 EnterPlanMode / ExitPlanMode
│   │       ├── notebook.go        #   P2 Jupyter Notebook 编辑
│   │       └── skill.go           #   P1 技能调用工具
│   │
│   ├── mcp/                       # MCP 客户端
│   │   ├── client.go              #   连接管理 + 工具/资源发现
│   │   ├── stdio.go               #   stdio 传输
│   │   ├── sse.go                 #   SSE 传输
│   │   └── http.go                #   Streamable HTTP 传输
│   │
│   ├── auth/                      # 认证链
│   │   ├── auth.go                #   认证优先级逻辑
│   │   ├── keychain_darwin.go     #   CC OAuth 读取（macOS Keychain）
│   │   ├── keychain_linux.go      #   CC OAuth 读取（Linux 密钥环）
│   │   ├── oauth.go               #   OAuth token 刷新和过期处理
│   │   └── apikey.go              #   环境变量 / 配置文件 API Key
│   │
│   ├── session/                   # 会话管理
│   │   ├── session.go             #   会话生命周期（创建/恢复/导出）
│   │   ├── compact.go             #   自动压缩 + 微压缩
│   │   └── store.go               #   JSON 持久化
│   │
│   ├── context/                   # 项目上下文
│   │   ├── project.go             #   XINCODE.md 解析
│   │   ├── git.go                 #   Git 状态感知
│   │   └── prompt.go              #   System Prompt 组装
│   │
│   ├── slash/                     # 斜杠命令（36 个）
│   │   ├── handler.go             #   命令路由
│   │   └── commands/              #   各命令实现
│   │       ├── help.go
│   │       ├── session.go         #   /session, /resume, /export
│   │       ├── model.go           #   /model, /provider
│   │       ├── config.go          #   /config, /login, /logout
│   │       ├── cost.go            #   /cost 费用详情
│   │       ├── context.go         #   /context 上下文使用率
│   │       ├── compact.go         #   /compact, /clear
│   │       ├── git.go             #   /commit, /pr, /branch, /diff
│   │       ├── code.go            #   /review, /refactor, /test, /plan
│   │       ├── tools.go           #   /mcp, /skills, /plugins, /hooks
│   │       ├── team.go            #   /agents, /team
│   │       ├── memory.go          #   /memory
│   │       ├── init.go            #   /init
│   │       ├── permissions.go     #   /permissions
│   │       ├── status.go          #   /status, /env, /version
│   │       └── upgrade.go         #   /upgrade, /tips
│   │
│   ├── tui/                       # 终端 UI（Bubbletea）
│   │   ├── app.go                 #   主程序（Bubbletea Model + Update + View）
│   │   ├── chat.go                #   对话区域（流式 Markdown 渲染）
│   │   ├── input.go               #   输入框（多行/IME/历史/Tab补全）
│   │   ├── statusbar.go           #   状态栏（品牌/模型/费用/上下文进度条）
│   │   ├── diff.go                #   Diff 预览组件
│   │   ├── permission.go          #   权限确认对话框
│   │   ├── spinner.go             #   工具执行动画
│   │   ├── toolblock.go           #   工具调用状态展示
│   │   └── theme.go               #   主题与样式（Lipgloss）
│   │
│   └── cost/                      # 费用追踪
│       ├── tracker.go             #   实时计费引擎（多 Provider 汇总）
│       ├── pricing.go             #   各模型价格表（内嵌 + 远程更新）
│       └── budget.go              #   预算控制（达到上限时行为）
│
├── examples/                      # 使用示例
│   ├── basic/
│   └── advanced/
│
├── docs/                          # 文档
│
├── .github/
│   └── workflows/
│       ├── build.yml              # CI
│       ├── release.yml            # CD（GoReleaser）
│       └── lint.yml               # 代码检查
│
├── .goreleaser.yml
├── .golangci.yml
├── go.mod
├── go.sum
├── Makefile
├── XINCODE.md                     # 自举
├── README.md
├── CONTRIBUTING.md
└── LICENSE                        # MIT
```

---

## 四、核心模块设计

### 4.1 Provider 系统

#### 接口定义

```go
type Provider interface {
    Name() string
    Stream(ctx context.Context, req *Request) (<-chan Event, error)
    Capabilities() Capabilities
}

type Request struct {
    Model       string
    System      string
    Messages    []Message
    Tools       []ToolDef
    MaxTokens   int
    Temperature float64
}

type Event struct {
    Type     EventType   // TextDelta | Thinking | ToolUse | Usage | Done | Error
    Text     string
    Thinking *ThinkingBlock  // Extended Thinking 内容
    ToolCall *ToolCall
    Usage    *Usage          // input_tokens, output_tokens, cache_*
    Error    error
}

type Capabilities struct {
    Thinking   bool
    Vision     bool
    ToolUse    bool
    Streaming  bool
    MaxContext int
}
```

#### 认证链（优先级从高到低）

1. CLI 参数 `--api-key`
2. 环境变量 `XINCODE_API_KEY` / `ANTHROPIC_API_KEY` / `OPENAI_API_KEY`
3. 配置文件 `~/.xincode/settings.json`
4. CC OAuth Token（macOS Keychain 读取）
5. 交互式引导（首次运行）

#### Provider 路由

```
模型名称 → Provider 匹配：
  claude-*、sonnet-*、opus-*、haiku-*  → AnthropicProvider
  gpt-*、o1-*、o3-*                    → OpenAIProvider
  其他                                  → OpenAIProvider（兼容模式，通过 BASE_URL）
```

#### AnthropicProvider 特有能力

- Extended Thinking（budget_tokens 控制）
- Prompt Caching（cache_control 标记）
- CC OAuth 复用

#### OpenAIProvider 兼容范围

- OpenAI 官方（GPT-4o、o1、o3 等）
- OpenRouter（通过 BASE_URL 配置，覆盖数百个模型）
- 任何 OpenAI chat/completions 兼容端点

### 4.2 Agent 循环引擎

```
agent.go 核心流程：

func (a *Agent) Run(ctx context.Context, userMessage string) {
    1. 解析输入（斜杠命令 / shell 命令 / 普通消息）
    2. 组装 System Prompt（含 Prompt Cache 标记）
    3. 选择 Provider + 构建请求
    4. 流式调用 API（含重试和错误恢复，见 4.2.1）
       ├─ 实时渲染文本到 tui/chat
       ├─ Thinking 内容 → 可折叠展示
       ├─ 实时更新费用到 tui/statusbar
       └─ 实时更新上下文到 tui/statusbar
    5. 检测 tool_use → tool/executor 执行
       ├─ 权限检查
       ├─ Edit 工具 → Diff 预览
       ├─ 读工具并发 / 写工具串行
       └─ 结果追加到消息历史
    6. 工具结果处理
       ├─ 微压缩：单个工具输出 > 50KB → 截断 + 摘要（见 4.2.2）
       └─ 继续循环 → 回到步骤 4
    7. 终止条件检查
       ├─ end_turn → 等待下次输入
       ├─ max_turns → 提示用户
       └─ ctx.Done() / Ctrl+C → 优雅中断（见 4.2.3）
    8. 上下文管理
       ├─ > 60% → 状态栏变黄
       ├─ > 80% → 状态栏变红
       ├─ > 90% → 自动压缩
       └─ 保留 system + 最近 N 轮，旧消息压缩为摘要
    9. 会话持久化（每轮自动保存）
}
```

#### 4.2.1 重试与错误恢复

```
API 调用失败处理：
├─ 429 (Rate Limit) → 指数退避重试（1s → 2s → 4s → 8s，最多 4 次）
├─ 500/502/503 (Server Error) → 重试最多 3 次
├─ 网络中断 → 重试 2 次，仍失败则提示用户
├─ 401 (Unauthorized) → 不重试，提示重新认证
└─ 其他错误 → 显示错误信息，等待用户下一条输入
```

#### 4.2.2 微压缩（工具输出截断）

```
单个工具返回结果过大时：
├─ 文本输出 > 50KB → 保留前 10KB + 后 5KB + "... 中间省略 N 字节"
├─ 二进制输出 → 保存到临时文件，返回文件路径
└─ 图片输出 → 缩放到合理尺寸后 base64 编码
```

#### 4.2.3 中断恢复

```
Ctrl+C 中断处理（通过 context.Context 传播）：
├─ 流式响应中 → 取消 HTTP 请求，保留已接收内容
├─ 工具执行中 → 发送 SIGTERM 给子进程，等待 3s 后强制 kill
├─ 文件写入中 → 等待当前原子写入完成，不中断半截
└─ 中断后状态 → 已执行的工具结果保留在消息历史中，用户可继续对话
```

#### 4.2.4 Prompt Cache 管理（Anthropic Provider）

```
缓存策略（减少 token 费用）：
├─ System Prompt → 标记 cache_control: ephemeral
├─ XINCODE.md 内容 → 标记 cache_control: ephemeral
├─ 工具定义列表 → 标记 cache_control: ephemeral
└─ 监控 cache hit/miss：
    ├─ Usage 事件中的 cache_creation_input_tokens
    ├─ Usage 事件中的 cache_read_input_tokens
    └─ /cost 命令中显示缓存命中率
```

#### 消息格式统一

```go
// internal/provider/message.go
// 统一消息格式，屏蔽不同 Provider 的差异

type Message struct {
    Role    Role           // user | assistant | system
    Content []ContentBlock // 支持混合内容
}

type ContentBlock struct {
    Type       BlockType // text | thinking | image | tool_use | tool_result
    Text       string
    Thinking   string    // Extended Thinking 思考内容
    ImageURL   string
    ToolCall   *ToolCall
    ToolResult *ToolResult
}

// AnthropicProvider 内部转换：Message → Anthropic Messages API 格式
// OpenAIProvider 内部转换：Message → OpenAI Chat Completions 格式
//   tool_use → function_calling 格式映射
```

### 4.3 工具系统

#### Tool 接口

```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() map[string]any  // JSON Schema
    IsReadOnly() bool             // 决定并发策略
    Execute(ctx context.Context, input json.RawMessage) (*Result, error)
}
```

#### 内置工具（21 个，按优先级分级）

**P0 — 核心工具（v1 必须，12 个）**

| 工具 | 只读 | 描述 | 对标 CC |
|------|------|------|---------|
| Read | ✓ | 读取文件内容（支持图片、PDF） | FileRead |
| Write | ✗ | 写入/创建文件 | FileWrite |
| Edit | ✗ | 编辑文件（含 Diff 预览） | FileEdit |
| Bash | ✗ | 执行 shell 命令（流式输出） | Bash |
| Glob | ✓ | 文件名模式匹配 | Glob |
| Grep | ✓ | 文件内容搜索（ripgrep 语法） | Grep |
| WebFetch | ✓ | 抓取网页内容 | WebFetch |
| WebSearch | ✓ | 网络搜索 | WebSearch |
| Agent | ✗ | 派生子 Agent（独立上下文） | Agent |
| MCP | 取决于 MCP 工具 | 调用 MCP 服务器工具 | MCPTool |
| AskUser | ✓ | 向用户提问/选择（Agent 循环关键） | AskUserQuestion |
| Task | ✗ | 任务管理（Create/Get/List/Update/Stop） | Task* 系列 |

**P1 — 重要工具（v1 实现，5 个）**

| 工具 | 只读 | 描述 | 对标 CC |
|------|------|------|---------|
| SendMessage | ✗ | Agent 间通信 | SendMessage |
| PlanMode | ✗ | 进入/退出计划模式 | EnterPlanMode/ExitPlanMode |
| Skill | ✓ | 调用已注册的技能 | SkillTool |
| Worktree | ✗ | Git worktree 隔离工作区 | EnterWorktree/ExitWorktree |
| NotebookEdit | ✗ | Jupyter Notebook 编辑 | NotebookEdit |

**P2 — 扩展工具（v1.1，4 个）**

| 工具 | 只读 | 描述 | 对标 CC |
|------|------|------|---------|
| Cron | ✗ | 定时任务管理 | CronCreate/CronDelete/CronList |
| REPL | ✗ | 交互式代码执行 | REPLTool |
| LSP | ✓ | 语言服务协议 | LSPTool |
| Doctor | ✓ | 环境诊断 | Xin Code 独有 |

#### 并发执行策略

```
一批工具调用到达时：
├─ 按 IsReadOnly() 分组
├─ 只读组 → goroutine 并发执行（max 10）
├─ 写入组 → 顺序执行
└─ 结果按原始顺序收集返回
```

### 4.4 权限系统

#### 五档模式（对标 CC）

```
bypass:       所有工具自动放行，不询问
acceptEdits:  文件读写自动放行，Bash 等执行类工具询问用户
default:      只读工具自动放行，写入工具询问用户（推荐默认）
plan:         只允许只读工具，所有写入操作仅展示计划不执行
interactive:  所有工具都询问用户
```

#### 规则配置（~/.xincode/settings.json）

```json
{
  "permissions": {
    "mode": "default",
    "rules": [
      { "tool": "Read",           "behavior": "allow", "source": "settings" },
      { "tool": "Glob",           "behavior": "allow", "source": "settings" },
      { "tool": "Grep",           "behavior": "allow", "source": "settings" },
      { "tool": "Bash(rm -rf *)", "behavior": "deny",  "source": "settings" },
      { "tool": "Bash(sudo *)",   "behavior": "deny",  "source": "settings" }
    ]
  }
}
```

#### 权限规则 source 字段

```
source 标识规则来源，决定优先级和生命周期：
├─ settings:  来自 settings.json，持久化
├─ project:   来自 .xincode/settings.json，项目级
├─ session:   用户本次会话中选择 always/never 产生，退出即丢
└─ cli:       来自 CLI 参数（如 --yes），最高优先级
```

#### 权限检查流程

```
工具调用到达
├─ 检查 deny 规则（按 source 优先级）→ 匹配则拒绝
├─ 检查 allow 规则（按 source 优先级）→ 匹配则放行
├─ 按当前模式决定
│   ├─ bypass → 放行
│   ├─ acceptEdits → Read/Write/Edit 放行，其他询问
│   ├─ default → IsReadOnly() ? 放行 : 询问用户
│   ├─ plan → IsReadOnly() ? 放行 : 显示"将会执行..."但不执行
│   └─ interactive → 询问用户
└─ 用户选择：
    ├─ yes → 本次放行
    ├─ no → 本次拒绝
    ├─ always → 加入 allow 规则（source: session）
    └─ never → 加入 deny 规则（source: session）
```

### 4.5 会话管理

#### 持久化

```
~/.xincode/
├── settings.json          # 全局配置
├── sessions/              # 会话存储
│   ├── <session-id>.json  # 完整消息历史
│   └── index.json         # 会话索引（ID/名称/目录/时间/费用）
└── auth/                  # 认证缓存
```

#### 会话恢复

```
/resume 流程：
1. 列出当前工作目录下的历史会话（按时间倒序）
2. 用户选择（或自动恢复最近一次）
3. 加载消息历史
4. 恢复 token 计数和费用
```

#### 自动压缩

```
触发条件：token 使用超过 max_context 的 90%

压缩策略：
1. 保留 System Prompt（不压缩）
2. 保留最近 5 轮对话（不压缩）
3. 中间消息 → 调用模型生成摘要（用便宜模型）
4. 替换原始消息为摘要
5. 状态栏短暂显示 "⚡ 已自动压缩"
```

### 4.6 System Prompt 组装

```
组装顺序（context/prompt.go）：

1. 基础身份
   "你是 Xin Code，一个 AI 驱动的终端编程助手。"

2. 能力描述
   "你可以读写文件、执行命令、搜索代码..."

3. 工具定义列表
   将所有可用工具（内置 + MCP）的 schema 注入

4. 项目指令
   读取 XINCODE.md 内容（如果存在）

5. 权限规则摘要
   当前权限模式和规则说明

6. 环境信息
   - 工作目录
   - 操作系统
   - Git 分支和状态
   - 当前日期

7. 记忆上下文（如果有）
```

---

## 五、TUI 设计

### 5.1 整体布局

```
┌──────────────────────────────────────────────────────────┐
│  ✖ XIN CODE  claude/sonnet-4-6          ¥0.32  78% ████░░│  状态栏
├──────────────────────────────────────────────────────────┤
│                                                          │
│  [对话区域 - 可滚动]                                      │  聊天区
│  用户消息、AI 回复、工具调用状态、Diff 预览                  │
│                                                          │
├──────────────────────────────────────────────────────────┤
│  ⏎ send  ⇧⏎ newline  /help commands  ctrl+c cancel      │  快捷键
│  > _                                                     │  输入框
└──────────────────────────────────────────────────────────┘
```

### 5.2 状态栏

```
组成元素（从左到右）：
[品牌标识] [当前模型] [费用] [上下文进度条]

费用显示：
- 默认人民币 ¥（可配置 $）
- 按模型实时计算（pricing.go 维护价格表）

上下文进度条颜色：
- < 60%:  绿色
- 60-80%: 黄色
- > 80%:  红色
```

### 5.3 Diff 预览

```
Edit 工具执行流程：
1. 计算 unified diff（go-diff）
2. 语法高亮（chroma）+ 红绿着色
3. 弹出 Diff 预览框
4. 等待用户选择：[y]es / [n]o
   （v1.1 加入 [e]dit 选项，需处理 Bubbletea 终端控制权交接）
5. 确认后原子写入文件
```

### 5.4 对话区域

```
消息类型渲染：

用户消息:    ">" 前缀 + 原文 + 分隔线
AI 思考:     灰色折叠区域 "💭 Thinking..." + Ctrl+O 展开完整思考内容
AI 回复:     Glamour Markdown 渲染（代码块语法高亮）
工具调用:    图标 + 工具名 + 参数摘要 + 耗时
             ⠋ 执行中 / ✓ 成功 / ✗ 失败 / ○ 跳过
工具输出:    折叠显示（> 20 行折叠，Ctrl+O 展开）
系统消息:    灰色斜体
错误:        红色高亮
```

### 5.5 输入框

```
功能：
- Enter:       发送
- Shift+Enter: 换行
- ↑/↓:         历史导航（100 条）
- Tab:         / 开头时命令补全
- Ctrl+C:      取消/退出
- Ctrl+L:      清屏
- IME:         中文输入法支持

前缀识别：
- /xxx  → 斜杠命令
- !xxx  → 直接执行 shell
- 其他  → 发送给 Agent
```

---

## 六、命令体系

### 6.1 完整命令列表（36 个）

#### 会话管理（7 个）
| 命令 | 描述 | 对标 |
|------|------|------|
| /help | 帮助信息 | CC + CodeAny |
| /session | 当前会话信息 | CC + CodeAny |
| /resume | 恢复历史会话 | CC + CodeAny |
| /compact | 手动压缩上下文 | CC + CodeAny |
| /clear | 清空当前对话 | CC + CodeAny |
| /export | 导出会话为 Markdown | CodeAny |
| /quit | 退出 | CC + CodeAny |

#### 模型与配置（8 个）
| 命令 | 描述 | 对标 |
|------|------|------|
| /model | 切换模型 | CC + CodeAny |
| /provider | 切换 Provider | Xin Code 独有 |
| /config | 运行时配置 | CC + CodeAny |
| /login | 配置认证 | CC + CodeAny |
| /logout | 清除认证 | CC + CodeAny |
| /permissions | 权限管理 | CC + CodeAny |
| /cost | 本次会话费用详情 | Xin Code 独有 |
| /status | 环境信息 | CC + CodeAny |

#### 开发工作流（8 个）
| 命令 | 描述 | 对标 |
|------|------|------|
| /commit | 生成 commit 并提交 | CC + CodeAny |
| /pr | 创建 Pull Request | CC + CodeAny |
| /review | 代码审查 | CC + CodeAny |
| /branch | 分支管理 | CC + CodeAny |
| /diff | 查看变更 | CC + CodeAny |
| /plan | 计划模式 | CC + CodeAny |
| /test | 运行测试 | CodeAny |
| /refactor | 重构建议 | CodeAny |

#### 工具与扩展（6 个）
| 命令 | 描述 | 对标 |
|------|------|------|
| /mcp | MCP 服务器管理 | CC + CodeAny |
| /skills | 技能管理 | CC + CodeAny |
| /plugins | 插件管理 | CC + CodeAny |
| /hooks | 钩子管理 | CC + CodeAny |
| /agents | 子 Agent 管理 | CC + CodeAny |
| /team | 多 Agent 协作 | CC + CodeAny |

#### 上下文与系统（7 个）
| 命令 | 描述 | 对标 |
|------|------|------|
| /init | 初始化 XINCODE.md | CC + CodeAny |
| /memory | 记忆管理 | CC + CodeAny |
| /context | 上下文使用率详情 | Xin Code 独有 |
| /env | 环境变量 | CC |
| /version | 版本信息 | CC + CodeAny |
| /upgrade | 检查更新 | CC |
| /tips | 使用技巧 | CodeAny |

---

## 七、配置系统

### 7.1 配置层级（优先级从高到低）

1. CLI 参数（`--model`, `--yes` 等）
2. 环境变量（`XINCODE_*`）
3. 项目配置（`.xincode/settings.json`）
4. 全局配置（`~/.xincode/settings.json`）
5. 默认值

### 7.2 配置文件结构

```json
{
  "model": "claude/sonnet-4-6",
  "provider": "anthropic",
  "permissions": {
    "mode": "default",
    "rules": [
      { "tool": "Bash(rm -rf *)", "behavior": "deny" },
      { "tool": "Bash(sudo *)",   "behavior": "deny" }
    ]
  },
  "cost": {
    "currency": "CNY",
    "budget": 10.0,
    "budgetAction": "warn"
  },
  "mcp": {
    "servers": {
      "filesystem": {
        "command": "mcp-server-filesystem",
        "args": ["/path/to/dir"]
      }
    }
  },
  "theme": "default"
}
```

### 7.3 环境变量

```bash
# 认证
XINCODE_API_KEY          # 通用 API Key
ANTHROPIC_API_KEY        # Anthropic 专用
OPENAI_API_KEY           # OpenAI 专用

# Provider 配置
XINCODE_MODEL            # 默认模型
XINCODE_BASE_URL         # 自定义 API 端点（OpenRouter 等）

# 行为
XINCODE_PERMISSION_MODE  # bypass / acceptEdits / default / plan / interactive
XINCODE_MAX_TURNS        # 最大轮次
```

---

## 八、MCP 集成

### 8.1 支持的传输方式

- **stdio**: 本地子进程（主要方式）
- **SSE**: HTTP Server-Sent Events
- **Streamable HTTP**: HTTP POST 流式响应（MCP 最新标准）

### 8.2 MCP 工具桥接

```
MCP 服务器工具 → 注册为 Xin Code 内部工具
├─ 工具名: mcp__<server>__<tool>
├─ schema: 从 MCP 服务器获取
├─ 权限: 同内置工具一样的权限检查
└─ 执行: 通过 MCP 客户端调用
```

### 8.3 MCP 资源（Resources）

```
MCP 服务器可以暴露资源供 Agent 读取：
├─ 资源发现: client.ListResources()
├─ 资源读取: client.ReadResource(uri)
├─ 资源类型: 文本(markdown/code)、图片、二进制
└─ 对应工具: 通过内置 MCP 工具访问
```

### 8.4 MCP 认证

```
远程 MCP 服务器可能需要 OAuth 认证：
├─ 服务器声明需要认证 → 发起 OAuth flow
├─ 用户在浏览器中完成授权
├─ Token 缓存到 ~/.xincode/auth/mcp_<server>.json
└─ 后续请求自动携带 token
```

### 8.5 MCP 配置

```json
{
  "mcp": {
    "servers": {
      "local-server": {
        "transport": "stdio",
        "command": "path/to/server",
        "args": ["--flag"],
        "env": { "KEY": "value" }
      },
      "remote-server": {
        "transport": "sse",
        "url": "https://mcp.example.com/sse",
        "auth": "oauth"
      }
    }
  }
}
```

---

## 九、CI/CD

### 9.1 GitHub Actions

#### build.yml（push / PR 触发）

```
- 跨平台并行：ubuntu-latest, macos-latest, windows-latest
- Go latest
- go build -v ./...
- go test -v -cover -race -timeout=60s ./...
```

#### lint.yml（PR 触发）

```
- golangci-lint run（~20 规则）
```

#### release.yml（git tag v* 触发）

```
- GoReleaser 多平台构建
  ├─ darwin-arm64 / darwin-amd64
  ├─ linux-arm64 / linux-amd64
  └─ windows-arm64 / windows-amd64
- GitHub Releases + 自动 changelog
- Homebrew Tap 自动更新
- SHA256 校验和
```

### 9.2 安装方式

```bash
# Homebrew (macOS/Linux)
brew install xincode-ai/tap/xin-code

# Go install
go install github.com/xincode-ai/xin-code@latest

# 直接下载
curl -fsSL https://github.com/xincode-ai/xin-code/releases/latest/download/xin-code_$(uname -s)_$(uname -m).tar.gz | tar xz

# 验证
xin-code --version
```

---

## 十、文件系统布局

### 10.1 用户目录

```
~/.xincode/
├── settings.json          # 全局配置
├── sessions/              # 会话存储
│   ├── <uuid>.json        # 消息历史
│   └── index.json         # 会话索引
├── auth/                  # 认证信息
│   └── credentials.json   # API Keys（加密存储）
├── memory/                # 记忆文件
├── skills/                # 用户级技能
├── plugins/               # 用户级插件
└── hooks/                 # 用户级钩子
```

### 10.2 项目目录

```
<project>/
├── XINCODE.md             # 项目指令文件
└── .xincode/
    ├── settings.json      # 项目级配置
    ├── skills/            # 项目级技能
    └── plugins/           # 项目级插件
```

---

## 十一、扩展机制

> v1 实现基础版本，架构上支持扩展，细节后续迭代。

### 11.1 Skills（技能）

```
两种调用方式：

1. AI 主动调用（通过 SkillTool）
   Agent 根据上下文判断需要调用某个 skill → 调用 Skill 工具
   → 加载 skill 内容 → 作为额外上下文注入当前轮
   注意：不是全部注入 system prompt，避免撑爆上下文

2. 用户手动调用
   用户输入 /skill-name → 加载对应 skill

约定优于配置：
~/.xincode/skills/<skill-name>/SKILL.md
.xincode/skills/<skill-name>/SKILL.md

SKILL.md 格式：
---
name: skill-name
description: 技能描述（给 AI 判断是否调用）
whenToUse: 触发条件描述
---

技能内容（Markdown，按需注入为上下文）
```

### 11.2 Plugins（插件）

```
<plugin-name>/
├── plugin.json     # 元数据（name, version, description）
├── skills/         # 插件提供的技能
└── hooks.json      # 事件钩子

加载位置：
- ~/.xincode/plugins/   （用户级）
- .xincode/plugins/     （项目级）

生命周期：
- 启动时自动发现和加载
- 加载失败 → 跳过并警告，不影响主程序
- /plugins 命令查看已加载插件和状态
```

### 11.3 Hooks（钩子）

```
支持的事件类型（对标 CC）：

会话级：
- sessionStart:    会话启动时
- sessionEnd:      会话结束时

查询级：
- preQuery:        API 调用前（可修改 system prompt）
- postQuery:       API 响应完成后

工具级：
- preToolUse:      工具执行前（可拦截/修改参数）
- postToolUse:     工具执行后（可修改结果）

上下文级：
- preCompact:      自动压缩前
- postCompact:     自动压缩后

配置格式（settings.json 或 hooks.json）：
{
  "hooks": {
    "preToolUse": [
      {
        "match": "Bash",
        "command": "echo 'About to run: $TOOL_INPUT'"
      }
    ],
    "sessionStart": [
      {
        "command": "echo 'Session started at $WORK_DIR'"
      }
    ]
  }
}

Hook 执行规则：
- 同一事件多个 hook → 按配置顺序执行
- hook 返回非 0 → 阻止操作（preToolUse 可拒绝工具执行）
- hook 超时 10s → 强制终止，操作继续
```

---

## 十二、非功能需求

### 12.1 性能

- 启动时间 < 100ms
- 流式响应首字节 < 500ms（取决于网络）
- 工具执行不阻塞 UI 渲染

### 12.2 安全

- API Key 不明文存储（使用系统 keychain 或加密文件）
- Bash 工具默认需要用户确认
- deny 规则阻止危险命令（rm -rf、sudo 等）
- 不自动提交包含敏感文件的 commit

### 12.3 兼容性

- Go 1.23+（确保兼容所有核心依赖最低版本要求）
- macOS（主要）、Linux、Windows
- 终端：iTerm2、Terminal.app、Windows Terminal、常见 Linux 终端

---

## 十三、v1 范围边界

### 包含（按优先级）

**P0 — v1 必须**
- 完整的 Agent 循环引擎（含重试、中断恢复、微压缩、Prompt Cache）
- Anthropic + OpenAI 两个 Provider
- CC OAuth 复用（macOS + Linux）
- 17 个内置工具（P0 12 个 + P1 5 个）
- 36 个斜杠命令
- Bubbletea 交互式 TUI（全家桶）
- 实时费用面板 + 上下文进度条 + Diff 预览
- 会话持久化 + 自动压缩 + resume
- MCP 客户端（stdio + SSE + Streamable HTTP）
- 权限系统（五档 + 可配置规则 + source 字段）
- GoReleaser + GitHub Actions CI/CD
- Homebrew Tap 分发

**P1 — v1 基础实现**
- Skills 扩展（SkillTool 调用 + 文件约定发现）
- Plugins 扩展（加载 + 基础生命周期）
- Hooks 扩展（8 种事件类型）
- 费用预算控制（达到上限时警告）

### 不包含（v1.1+）

- Gemini / Ollama / DeepSeek Provider
- 智能模型路由（按任务复杂度自动选模型）
- P2 工具（Cron、REPL、LSP、Doctor）
- Diff 预览 [e]dit 选项（$EDITOR 集成）
- Web UI
- 团队协作服务端
- Windows Keychain 支持
- 代码签名与公证
- 匿名使用统计（opt-in telemetry）

---

## 十四、参考项目

| 项目 | 参考维度 |
|------|---------|
| Claude Code v2.1.88 源码 | Agent 循环、工具系统、权限系统、Auto Compact、System Prompt 组装 |
| codeany-ai/codeany | Go 项目结构、TUI 实现、Slash 命令、Skills/Plugins 扩展机制 |
| charmbracelet/mods | Go 工程最佳实践、GoReleaser 配置、多 Provider 模式、golangci-lint |
| charmbracelet/gum | Bubbletea 组件设计、Shell 补全 |
| cli/cli | 企业级 Go 项目结构、GitHub Actions 工作流、贡献指南 |
