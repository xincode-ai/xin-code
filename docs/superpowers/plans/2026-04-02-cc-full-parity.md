# Xin Code → Claude Code 100% 功能对标实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Xin Code 从当前 ~60% 完成度提升到 100% Claude Code 功能对标，覆盖 Context 机制、Memory 系统、Agent 循环、TUI 交互四大核心系统。

**Architecture:** 分 4 个阶段实施，每个阶段可独立交付测试。Phase 1（Context）是基础，Phase 2（Agent Loop）是核心引擎，Phase 3（Memory）依赖前两者，Phase 4（TUI）是最终体验层。所有设计直接翻译自 Claude Code TypeScript 源码，保持接口和行为一致。

**Tech Stack:** Go 1.22+, Bubbletea/Lipgloss (TUI), Glamour (Markdown), Anthropic/OpenAI SDK

**CC 源码参考路径:** `/Users/ocean/Studio/03-lab/03-explore/claude-code源码/claude-code-sourcemap/restored-src/src/`

---

## Phase 1: Context 机制

CC 的 context 系统是多层级的：managed → user → project → local，支持 `@include` 指令，自动注入 git status 和当前日期。Xin Code 当前只从 cwd 读取单个 XINCODE.md。

### File Structure (Phase 1)

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/context/claudemd.go` | Create | 多层级 CLAUDEMD/XINCODE.md 发现和加载 |
| `internal/context/claudemd_test.go` | Create | 发现逻辑测试 |
| `internal/context/prompt.go` | Modify | 重构系统提示词，对标 CC 结构 |
| `internal/context/prompt_test.go` | Create | 提示词组装测试 |
| `internal/context/git.go` | Create | Git 状态采集（CC 格式） |
| `internal/context/git_test.go` | Create | Git 状态测试 |

---

### Task 1: 多层级 XINCODE.md 发现与加载

CC 按优先级从 4 个层级加载配置文件（`src/utils/claudemd.ts`），支持 `@include` 指令和 frontmatter globs。Xin Code 需要相同机制，但文件名用 `XINCODE.md`。

**Files:**
- Create: `internal/context/claudemd.go`
- Create: `internal/context/claudemd_test.go`

- [ ] **Step 1: 写测试 — 发现逻辑**

```go
// internal/context/claudemd_test.go
package context

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverMemoryFiles(t *testing.T) {
	// 创建临时目录模拟项目结构
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	projectDir := filepath.Join(tmpDir, "project")

	os.MkdirAll(filepath.Join(homeDir, ".xincode"), 0755)
	os.MkdirAll(filepath.Join(projectDir, ".xincode", "rules"), 0755)

	// 写入各层级文件
	os.WriteFile(filepath.Join(homeDir, ".xincode", "XINCODE.md"), []byte("# User Config"), 0644)
	os.WriteFile(filepath.Join(projectDir, "XINCODE.md"), []byte("# Project Root"), 0644)
	os.WriteFile(filepath.Join(projectDir, ".xincode", "XINCODE.md"), []byte("# Project Dir"), 0644)
	os.WriteFile(filepath.Join(projectDir, ".xincode", "rules", "rule1.md"), []byte("# Rule 1"), 0644)
	os.WriteFile(filepath.Join(projectDir, "XINCODE.local.md"), []byte("# Local Only"), 0644)

	files := DiscoverMemoryFiles(projectDir, homeDir)

	// 验证加载顺序：user → project root → project dir → rules → local
	if len(files) != 5 {
		t.Fatalf("expected 5 files, got %d", len(files))
	}
	if files[0].Type != MemoryTypeUser {
		t.Errorf("first file should be User type, got %s", files[0].Type)
	}
	if files[len(files)-1].Type != MemoryTypeLocal {
		t.Errorf("last file should be Local type, got %s", files[len(files)-1].Type)
	}
}

func TestProcessIncludeDirectives(t *testing.T) {
	tmpDir := t.TempDir()
	includedFile := filepath.Join(tmpDir, "included.md")
	os.WriteFile(includedFile, []byte("# Included Content"), 0644)

	mainContent := "@" + includedFile + "\n# Main Content"
	result := processIncludes(mainContent, tmpDir, 0)

	if !contains(result, "Included Content") {
		t.Error("should include referenced file content")
	}
	if !contains(result, "Main Content") {
		t.Error("should preserve main content")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go test ./internal/context/ -run TestDiscover -v`
Expected: FAIL — `DiscoverMemoryFiles` undefined

- [ ] **Step 3: 实现 claudemd.go**

```go
// internal/context/claudemd.go
package context

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MemoryType 配置文件来源层级
type MemoryType string

const (
	MemoryTypeManaged MemoryType = "managed"  // /etc/xincode/XINCODE.md
	MemoryTypeUser    MemoryType = "user"     // ~/.xincode/XINCODE.md
	MemoryTypeProject MemoryType = "project"  // ./XINCODE.md, .xincode/XINCODE.md, .xincode/rules/*.md
	MemoryTypeLocal   MemoryType = "local"    // ./XINCODE.local.md
	MemoryTypeAutoMem MemoryType = "automem"  // ~/.xincode/projects/{hash}/memory/
)

// MemoryFileInfo 表示一个已发现的配置/记忆文件
type MemoryFileInfo struct {
	Path    string     // 绝对路径
	Type    MemoryType // 来源层级
	Content string     // 文件内容（含 @include 展开后）
}

// 最大 @include 深度，防止循环引用
const maxIncludeDepth = 5

// 单个文件最大字符数
const maxFileChars = 40000

// includeRegex 匹配 @path、@./path、@~/path 格式的引用
var includeRegex = regexp.MustCompile(`(?m)^@(.+)$`)

// DiscoverMemoryFiles 按优先级发现所有配置文件
// 加载顺序（低 → 高优先级）：managed → user → project → local
// CC 参考：src/utils/claudemd.ts
func DiscoverMemoryFiles(projectDir, homeDir string) []MemoryFileInfo {
	var files []MemoryFileInfo

	// 1. Managed: /etc/xincode/XINCODE.md（系统级，通常不存在）
	if info := loadIfExists("/etc/xincode/XINCODE.md", MemoryTypeManaged, projectDir); info != nil {
		files = append(files, *info)
	}

	// 2. User: ~/.xincode/XINCODE.md
	if homeDir != "" {
		userPath := filepath.Join(homeDir, ".xincode", "XINCODE.md")
		if info := loadIfExists(userPath, MemoryTypeUser, projectDir); info != nil {
			files = append(files, *info)
		}
	}

	// 3. Project: XINCODE.md（根目录）
	if info := loadIfExists(filepath.Join(projectDir, "XINCODE.md"), MemoryTypeProject, projectDir); info != nil {
		files = append(files, *info)
	}

	// 4. Project: .xincode/XINCODE.md
	if info := loadIfExists(filepath.Join(projectDir, ".xincode", "XINCODE.md"), MemoryTypeProject, projectDir); info != nil {
		files = append(files, *info)
	}

	// 5. Project: .xincode/rules/*.md（按文件名排序）
	rulesDir := filepath.Join(projectDir, ".xincode", "rules")
	if entries, err := os.ReadDir(rulesDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
				rulePath := filepath.Join(rulesDir, entry.Name())
				if info := loadIfExists(rulePath, MemoryTypeProject, projectDir); info != nil {
					files = append(files, *info)
				}
			}
		}
	}

	// 6. Local: XINCODE.local.md（不提交到 git 的个人配置）
	if info := loadIfExists(filepath.Join(projectDir, "XINCODE.local.md"), MemoryTypeLocal, projectDir); info != nil {
		files = append(files, *info)
	}

	return files
}

// loadIfExists 尝试读取文件并展开 @include 指令
func loadIfExists(path string, memType MemoryType, baseDir string) *MemoryFileInfo {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	content := string(data)
	if len(content) > maxFileChars {
		content = content[:maxFileChars]
	}

	// 展开 @include 指令
	content = processIncludes(content, filepath.Dir(path), 0)

	return &MemoryFileInfo{
		Path:    path,
		Type:    memType,
		Content: content,
	}
}

// processIncludes 递归展开 @include 指令
// 支持格式：@/absolute/path、@./relative/path、@~/home/path
// CC 参考：extractIncludePathsFromTokens()
func processIncludes(content, baseDir string, depth int) string {
	if depth >= maxIncludeDepth {
		return content
	}

	return includeRegex.ReplaceAllStringFunc(content, func(match string) string {
		refPath := strings.TrimSpace(match[1:]) // 去掉 @ 前缀

		// 解析路径
		var absPath string
		switch {
		case strings.HasPrefix(refPath, "~/"):
			home, _ := os.UserHomeDir()
			absPath = filepath.Join(home, refPath[2:])
		case strings.HasPrefix(refPath, "./"):
			absPath = filepath.Join(baseDir, refPath)
		case filepath.IsAbs(refPath):
			absPath = refPath
		default:
			absPath = filepath.Join(baseDir, refPath)
		}

		data, err := os.ReadFile(absPath)
		if err != nil {
			return match // 文件不存在，保留原文
		}

		included := string(data)
		if len(included) > maxFileChars {
			included = included[:maxFileChars]
		}

		// 递归展开
		return processIncludes(included, filepath.Dir(absPath), depth+1)
	})
}

// FormatMemoryForPrompt 将所有发现的文件格式化为系统提示词的一部分
// CC 参考：getUserContext() 中 claudeMd 的格式
func FormatMemoryForPrompt(files []MemoryFileInfo, projectDir string) string {
	if len(files) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Codebase and user instructions are shown below. Be sure to adhere to these instructions. ")
	sb.WriteString("IMPORTANT: These instructions OVERRIDE any default behavior and you MUST follow them exactly as written.\n\n")

	typeLabels := map[MemoryType]string{
		MemoryTypeManaged: "system-managed global instructions",
		MemoryTypeUser:    "user's private global instructions for all projects",
		MemoryTypeProject: "project instructions, checked into the codebase",
		MemoryTypeLocal:   "user's private project-specific instructions",
		MemoryTypeAutoMem: "user's auto-memory, persists across conversations",
	}

	for _, f := range files {
		relPath := f.Path
		label := typeLabels[f.Type]
		sb.WriteString("Contents of ")
		sb.WriteString(relPath)
		sb.WriteString(" (")
		sb.WriteString(label)
		sb.WriteString("):\n\n")
		sb.WriteString(f.Content)
		sb.WriteString("\n\n")
	}

	return sb.String()
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go test ./internal/context/ -run TestDiscover -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code
git add internal/context/claudemd.go internal/context/claudemd_test.go
git commit -m "feat(context): 多层级 XINCODE.md 发现与加载，对标 CC claudemd.ts"
```

---

### Task 2: Git 状态采集（CC 格式）

CC 在 `src/context.ts` 的 `getGitStatus()` 中采集 branch、main branch、user、status、recent commits，格式化为固定模板注入系统提示词。

**Files:**
- Create: `internal/context/git.go`
- Create: `internal/context/git_test.go`

- [ ] **Step 1: 写测试**

```go
// internal/context/git_test.go
package context

import (
	"strings"
	"testing"
)

func TestGetGitStatus_Format(t *testing.T) {
	// 在实际 git 仓库中测试（xin-code 本身就是 git 仓库）
	status := GetGitStatus("/Users/ocean/Studio/01-workshop/06-开源项目/xin-code")
	if status == "" {
		t.Skip("not in a git repository")
	}

	// 验证 CC 格式的关键字段
	if !strings.Contains(status, "Current branch:") {
		t.Error("should contain 'Current branch:'")
	}
	if !strings.Contains(status, "Main branch") {
		t.Error("should contain 'Main branch'")
	}
	if !strings.Contains(status, "Recent commits:") {
		t.Error("should contain 'Recent commits:'")
	}
}

func TestGetGitStatus_NonGitDir(t *testing.T) {
	status := GetGitStatus("/tmp")
	if status != "" {
		t.Error("should return empty for non-git directory")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go test ./internal/context/ -run TestGetGitStatus -v`
Expected: FAIL

- [ ] **Step 3: 实现 git.go**

```go
// internal/context/git.go
package context

import (
	"fmt"
	"os/exec"
	"strings"
)

// maxGitStatusLen Git 状态最大字符数（CC: 2000）
const maxGitStatusLen = 2000

// GetGitStatus 采集 git 状态，格式对标 CC context.ts 的 getGitStatus()
// 返回空字符串表示不在 git 仓库中
func GetGitStatus(workDir string) string {
	// 检查是否是 git 仓库
	if _, err := gitCmd(workDir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return ""
	}

	branch, _ := gitCmd(workDir, "branch", "--show-current")
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "(detached HEAD)"
	}

	// 检测主分支名称（main 或 master）
	mainBranch := detectMainBranch(workDir)

	// git user
	userName, _ := gitCmd(workDir, "config", "user.name")
	userName = strings.TrimSpace(userName)

	// git status --short
	status, _ := gitCmd(workDir, "status", "--short")
	status = strings.TrimSpace(status)
	if status == "" {
		status = "(clean)"
	}

	// 最近提交
	log, _ := gitCmd(workDir, "log", "--oneline", "-10")
	log = strings.TrimSpace(log)

	// CC 格式组装
	var sb strings.Builder
	sb.WriteString("This is the git status at the start of the conversation. ")
	sb.WriteString("Note that this status is a snapshot in time, and will not update during the conversation.\n\n")
	sb.WriteString(fmt.Sprintf("Current branch: %s\n", branch))
	sb.WriteString(fmt.Sprintf("Main branch (you will usually use this for PRs): %s\n", mainBranch))
	if userName != "" {
		sb.WriteString(fmt.Sprintf("Git user: %s\n", userName))
	}
	sb.WriteString(fmt.Sprintf("Status:\n%s\n\n", status))
	sb.WriteString(fmt.Sprintf("Recent commits:\n%s", log))

	result := sb.String()
	if len(result) > maxGitStatusLen {
		result = result[:maxGitStatusLen]
	}
	return result
}

// detectMainBranch 检测主分支名称
func detectMainBranch(workDir string) string {
	// 尝试 main
	if _, err := gitCmd(workDir, "rev-parse", "--verify", "main"); err == nil {
		return "main"
	}
	// 尝试 master
	if _, err := gitCmd(workDir, "rev-parse", "--verify", "master"); err == nil {
		return "master"
	}
	// 回退：当前分支
	branch, _ := gitCmd(workDir, "branch", "--show-current")
	return strings.TrimSpace(branch)
}

// gitCmd 在指定目录执行 git 命令
func gitCmd(workDir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workDir
	out, err := cmd.Output()
	return string(out), err
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go test ./internal/context/ -run TestGetGitStatus -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code
git add internal/context/git.go internal/context/git_test.go
git commit -m "feat(context): CC 格式 git 状态采集"
```

---

### Task 3: 重构系统提示词，对标 CC 结构

CC 的系统提示词有严格的分层结构（`src/constants/prompts.ts`）：Identity → System → Doing Tasks → Tool Instructions → Environment。当前 Xin Code 的 prompt.go 是单一大字符串，需要重构。

**Files:**
- Modify: `internal/context/prompt.go`
- Modify: `agent.go` — 调用新的 context API

- [ ] **Step 1: 重构 prompt.go — 系统提示词模板**

```go
// internal/context/prompt.go
package context

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

// SystemPromptConfig 构建系统提示词所需的配置
type SystemPromptConfig struct {
	WorkDir    string
	HomeDir    string
	Model      string
	Provider   string
	Version    string
	ToolCount  int
	PermMode   string
	MaxContext int
	ToolDescs  []ToolDescription
}

// ToolDescription 工具描述（用于注入系统提示词）
type ToolDescription struct {
	Name        string
	Description string
	ReadOnly    bool
}

// BuildSystemPrompt 构建完整系统提示词
// 结构对标 CC src/constants/prompts.ts
func BuildSystemPrompt(cfg SystemPromptConfig) string {
	var sections []string

	// 1. Identity（CC: getCLISyspromptPrefix）
	sections = append(sections, buildIdentitySection(cfg))

	// 2. System（CC: SYSTEM_PROMPT 的 System 部分）
	sections = append(sections, buildSystemSection())

	// 3. Doing Tasks（CC: SYSTEM_PROMPT 的 Doing tasks 部分）
	sections = append(sections, buildDoingTasksSection())

	// 4. Tool Usage（CC: Using your tools 部分）
	sections = append(sections, buildToolUsageSection(cfg.ToolDescs))

	// 5. Tone and Style
	sections = append(sections, buildToneSection())

	// 6. Environment（CC: 动态注入部分）
	sections = append(sections, buildEnvironmentSection(cfg))

	return strings.Join(sections, "\n\n")
}

// BuildUserContext 构建用户上下文（注入为 system-reminder）
// CC 参考：getUserContext() + prependUserContext()
func BuildUserContext(cfg SystemPromptConfig) string {
	var parts []string

	// XINCODE.md / CLAUDE.md 内容
	memFiles := DiscoverMemoryFiles(cfg.WorkDir, cfg.HomeDir)
	if md := FormatMemoryForPrompt(memFiles, cfg.WorkDir); md != "" {
		parts = append(parts, "# claudeMd\n"+md)
	}

	// 当前日期
	parts = append(parts, fmt.Sprintf("# currentDate\nToday's date is %s.", time.Now().Format("2006-01-02")))

	return strings.Join(parts, "\n")
}

// BuildSystemContext 构建系统上下文（git status 等）
// CC 参考：getSystemContext()
func BuildSystemContext(cfg SystemPromptConfig) string {
	gitStatus := GetGitStatus(cfg.WorkDir)
	if gitStatus == "" {
		return ""
	}
	return "# gitStatus\n" + gitStatus
}

func buildIdentitySection(cfg SystemPromptConfig) string {
	return fmt.Sprintf(`You are Xin Code, an AI-powered terminal coding assistant.
You are an interactive agent that helps users with software engineering tasks. Use the instructions below and the tools available to you to assist the user.`)
}

func buildSystemSection() string {
	return `# System
 - All text you output outside of tool use is displayed to the user. Output text to communicate with the user. You can use Github-flavored markdown for formatting.
 - Tools are executed in a user-selected permission mode. When you attempt to call a tool that is not automatically allowed, the user will be prompted to approve or deny.
 - Tool results may include <system-reminder> tags containing information from the system.
 - Be careful not to introduce security vulnerabilities such as command injection, XSS, SQL injection, and other OWASP top 10 vulnerabilities.`
}

func buildDoingTasksSection() string {
	return `# Doing tasks
 - The user will primarily request you to perform software engineering tasks.
 - In general, do not propose changes to code you haven't read. If a user asks about or wants you to modify a file, read it first.
 - Do not create files unless they're absolutely necessary for achieving your goal.
 - Don't add features, refactor code, or make "improvements" beyond what was asked.
 - Don't add error handling, fallbacks, or validation for scenarios that can't happen.
 - Don't create helpers, utilities, or abstractions for one-time operations.
 - If an approach fails, diagnose why before switching tactics.
 - Avoid giving time estimates or predictions for how long tasks will take.`
}

func buildToolUsageSection(tools []ToolDescription) string {
	var sb strings.Builder
	sb.WriteString("# Using your tools\n")
	sb.WriteString(" - Do NOT use the Bash to run commands when a relevant dedicated tool is provided.\n")
	sb.WriteString("   - To read files use Read instead of cat/head/tail\n")
	sb.WriteString("   - To edit files use Edit instead of sed/awk\n")
	sb.WriteString("   - To create files use Write instead of echo/cat\n")
	sb.WriteString("   - To search files use Glob instead of find\n")
	sb.WriteString("   - To search content use Grep instead of grep/rg\n")
	sb.WriteString(" - You can call multiple tools in a single response. Make independent calls in parallel.\n")

	if len(tools) > 0 {
		sb.WriteString("\n## Available tools\n")
		for _, t := range tools {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", t.Name, t.Description))
		}
	}

	return sb.String()
}

func buildToneSection() string {
	return `# Tone and style
 - Be concise. Lead with the answer, not the reasoning.
 - When referencing code, include file_path:line_number to allow easy navigation.
 - Do not restate what the user said.
 - Keep responses short and direct. If you can say it in one sentence, don't use three.`
}

func buildEnvironmentSection(cfg SystemPromptConfig) string {
	hostname, _ := os.Hostname()
	return fmt.Sprintf(`# Environment
 - Working directory: %s
 - Platform: %s
 - Architecture: %s
 - Shell: %s
 - Model: %s (via %s)
 - Max context: %d tokens
 - Version: %s`,
		cfg.WorkDir,
		runtime.GOOS,
		runtime.GOARCH,
		os.Getenv("SHELL"),
		cfg.Model,
		cfg.Provider,
		cfg.MaxContext,
		cfg.Version,
	)
}
```

- [ ] **Step 2: 更新 agent.go 调用新 API**

在 `agent.go` 中，将原来的 `context.BuildSystemPrompt()` 调用改为传入 `SystemPromptConfig`，并将 `BuildUserContext()` 和 `BuildSystemContext()` 作为 system-reminder 注入到消息列表。

修改 `agent.go` 中 `Run()` 方法的系统提示词构建部分：

```go
// agent.go — Run() 方法中构建提示词部分
homeDir, _ := os.UserHomeDir()
promptCfg := context.SystemPromptConfig{
	WorkDir:    a.session.WorkDir,
	HomeDir:    homeDir,
	Model:      a.session.Model,
	Provider:   a.provider.Name(),
	Version:    a.version,
	ToolCount:  len(a.tools.All()),
	PermMode:   a.permMode,
	MaxContext: a.provider.Capabilities().MaxContext,
	ToolDescs:  buildToolDescs(a.tools),
}
systemPrompt := context.BuildSystemPrompt(promptCfg)

// 用户上下文作为首条 system-reminder 注入
userCtx := context.BuildUserContext(promptCfg)
sysCtx := context.BuildSystemContext(promptCfg)
contextReminder := ""
if userCtx != "" || sysCtx != "" {
	contextReminder = userCtx
	if sysCtx != "" {
		contextReminder += "\n" + sysCtx
	}
}
```

- [ ] **Step 3: 编译验证**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 4: 提交**

```bash
cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code
git add internal/context/prompt.go agent.go
git commit -m "refactor(context): 系统提示词对标 CC 分层结构"
```

---

## Phase 2: Agent 循环增强

CC 的 agent loop 有 7 个 state transition、10 次重试、429/529 指数退避、自动压缩、prompt-too-long 恢复。Xin Code 当前是简单循环，需要大幅增强。

### File Structure (Phase 2)

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/agent/retry.go` | Create | 重试逻辑（指数退避、429/529 处理） |
| `internal/agent/retry_test.go` | Create | 重试逻辑测试 |
| `internal/agent/compact.go` | Create | 自动压缩（阈值检测 + 执行） |
| `internal/agent/compact_test.go` | Create | 压缩逻辑测试 |
| `agent.go` | Modify | 重构主循环，集成重试和压缩 |
| `internal/tool/executor.go` | Modify | StreamingToolExecutor 并发控制 |

---

### Task 4: 重试机制 — 指数退避 + 429/529 处理

CC 在 `src/services/api/withRetry.ts` 中实现了完整的重试逻辑。Xin Code 当前没有任何重试。

**Files:**
- Create: `internal/agent/retry.go`
- Create: `internal/agent/retry_test.go`

- [ ] **Step 1: 写测试**

```go
// internal/agent/retry_test.go
package agent

import (
	"testing"
	"time"
)

func TestRetryDelay_ExponentialBackoff(t *testing.T) {
	// attempt 1: 500ms base
	d1 := calcRetryDelay(1, "")
	if d1 < 375*time.Millisecond || d1 > 625*time.Millisecond {
		t.Errorf("attempt 1 delay should be ~500ms±25%%, got %v", d1)
	}

	// attempt 3: 2000ms base
	d3 := calcRetryDelay(3, "")
	if d3 < 1500*time.Millisecond || d3 > 2500*time.Millisecond {
		t.Errorf("attempt 3 delay should be ~2000ms±25%%, got %v", d3)
	}

	// cap at 32s
	d10 := calcRetryDelay(10, "")
	if d10 > 40*time.Second {
		t.Errorf("delay should be capped, got %v", d10)
	}
}

func TestRetryDelay_RetryAfterHeader(t *testing.T) {
	d := calcRetryDelay(1, "5")
	if d < 5*time.Second {
		t.Errorf("should respect Retry-After header, got %v", d)
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		code      int
		retryable bool
	}{
		{429, true},
		{529, true},
		{500, true},
		{502, true},
		{503, true},
		{408, true},
		{400, false},
		{404, false},
		{200, false},
	}
	for _, tt := range tests {
		if isRetryableStatusCode(tt.code) != tt.retryable {
			t.Errorf("status %d: expected retryable=%v", tt.code, tt.retryable)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go test ./internal/agent/ -run TestRetry -v`
Expected: FAIL

- [ ] **Step 3: 实现 retry.go**

```go
// internal/agent/retry.go
package agent

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"time"
)

// CC 对标常量：src/services/api/withRetry.ts
const (
	defaultMaxRetries  = 10
	baseDelayMs        = 500
	maxDelayMs         = 32000 // 32s cap
	max529Retries      = 3
	jitterFraction     = 0.25 // ±25%
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries int
	OnRetry    func(attempt int, err error, delay time.Duration) // 通知 TUI
}

// DefaultRetryConfig 返回 CC 默认重试配置
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: defaultMaxRetries,
	}
}

// APIError 包含 HTTP 状态码和重试头的 API 错误
type APIError struct {
	StatusCode int
	Message    string
	RetryAfter string // Retry-After 头值
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Message)
}

// WithRetry 包装一个操作，添加重试逻辑
// CC 参考：src/services/api/withRetry.ts
func WithRetry[T any](
	ctx context.Context,
	cfg RetryConfig,
	operation func(ctx context.Context, attempt int) (T, error),
) (T, error) {
	var zero T
	consecutive529 := 0

	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		result, err := operation(ctx, attempt)
		if err == nil {
			return result, nil
		}

		// 检查上下文取消
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}

		// 提取 API 错误信息
		apiErr, isAPIErr := err.(*APIError)
		if !isAPIErr || !isRetryableStatusCode(apiErr.StatusCode) {
			return zero, err // 不可重试的错误
		}

		// 529 连续计数
		if apiErr.StatusCode == 529 {
			consecutive529++
			if consecutive529 >= max529Retries {
				return zero, fmt.Errorf("API 过载（连续 %d 次 529 错误）: %w", consecutive529, err)
			}
		} else {
			consecutive529 = 0
		}

		// 最后一次重试失败
		if attempt >= cfg.MaxRetries {
			return zero, fmt.Errorf("重试 %d 次后仍失败: %w", cfg.MaxRetries, err)
		}

		// 计算退避延迟
		delay := calcRetryDelay(attempt, apiErr.RetryAfter)

		// 通知回调
		if cfg.OnRetry != nil {
			cfg.OnRetry(attempt, err, delay)
		}

		// 等待
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(delay):
		}
	}

	return zero, fmt.Errorf("exceeded max retries")
}

// calcRetryDelay 计算重试延迟
// CC 公式：base * 2^(attempt-1) + jitter，cap at 32s
func calcRetryDelay(attempt int, retryAfter string) time.Duration {
	// 指数退避
	baseMs := float64(baseDelayMs) * math.Pow(2, float64(attempt-1))
	if baseMs > float64(maxDelayMs) {
		baseMs = float64(maxDelayMs)
	}

	// ±25% jitter
	jitter := baseMs * jitterFraction * (2*rand.Float64() - 1)
	delayMs := baseMs + jitter

	// Retry-After 头优先
	if retryAfter != "" {
		if seconds, err := strconv.ParseFloat(retryAfter, 64); err == nil {
			headerMs := seconds * 1000
			if headerMs > delayMs {
				delayMs = headerMs
			}
		}
	}

	return time.Duration(delayMs) * time.Millisecond
}

// isRetryableStatusCode 判断 HTTP 状态码是否可重试
func isRetryableStatusCode(code int) bool {
	switch code {
	case 408, 409, 429, 529:
		return true
	default:
		return code >= 500
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go test ./internal/agent/ -run TestRetry -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code
git add internal/agent/retry.go internal/agent/retry_test.go
git commit -m "feat(agent): 重试机制 — 指数退避 + 429/529 处理，对标 CC withRetry.ts"
```

---

### Task 5: 自动压缩（Auto-Compact）

CC 在 `src/services/compact/autoCompact.ts` 中实现自动压缩：当 token 使用量达到上下文窗口的 ~87% 时自动触发。

**Files:**
- Create: `internal/agent/compact.go`
- Create: `internal/agent/compact_test.go`

- [ ] **Step 1: 写测试**

```go
// internal/agent/compact_test.go
package agent

import (
	"testing"
)

func TestAutoCompactThreshold(t *testing.T) {
	// 200k context window: effective = 200000 - 20000 = 180000
	// threshold = 180000 - 13000 = 167000
	threshold := getAutoCompactThreshold(200000)
	if threshold != 167000 {
		t.Errorf("expected 167000, got %d", threshold)
	}
}

func TestTokenWarningState(t *testing.T) {
	maxCtx := 200000

	// 50% 使用：安全
	state := CalculateTokenWarningState(100000, maxCtx)
	if state.Level != TokenLevelSafe {
		t.Errorf("50%% should be safe, got %s", state.Level)
	}

	// 80% 使用：警告
	state = CalculateTokenWarningState(160000, maxCtx)
	if state.Level != TokenLevelWarning {
		t.Errorf("80%% should be warning, got %s", state.Level)
	}

	// 95% 使用：错误
	state = CalculateTokenWarningState(190000, maxCtx)
	if state.Level != TokenLevelError {
		t.Errorf("95%% should be error, got %s", state.Level)
	}
}

func TestShouldAutoCompact(t *testing.T) {
	maxCtx := 200000
	threshold := getAutoCompactThreshold(maxCtx)

	// 低于阈值：不压缩
	if shouldAutoCompact(threshold-1000, maxCtx) {
		t.Error("should not compact below threshold")
	}

	// 超过阈值：压缩
	if !shouldAutoCompact(threshold+1000, maxCtx) {
		t.Error("should compact above threshold")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go test ./internal/agent/ -run TestAutoCompact -v && go test ./internal/agent/ -run TestTokenWarning -v && go test ./internal/agent/ -run TestShouldAutoCompact -v`
Expected: FAIL

- [ ] **Step 3: 实现 compact.go**

```go
// internal/agent/compact.go
package agent

import (
	"fmt"
	"strings"

	"github.com/xincode-ai/xin-code/internal/provider"
)

// CC 对标常量：src/services/compact/autoCompact.ts
const (
	autoCompactBufferTokens   = 13000 // 自动压缩触发缓冲
	warningThresholdBuffer    = 20000 // 警告阈值缓冲
	errorThresholdBuffer      = 20000 // 错误阈值缓冲
	maxOutputTokensForSummary = 20000 // 摘要生成保留的输出 token
	maxConsecutiveFailures    = 3     // 熔断器：连续失败上限
)

// TokenLevel 上下文使用量级别
type TokenLevel string

const (
	TokenLevelSafe    TokenLevel = "safe"
	TokenLevelWarning TokenLevel = "warning"
	TokenLevelError   TokenLevel = "error"
)

// TokenWarningState 上下文使用状态
type TokenWarningState struct {
	Level       TokenLevel
	PercentUsed int // 0-100
	TokensUsed  int
	TokensMax   int
}

// AutoCompactState 自动压缩跟踪状态
type AutoCompactState struct {
	Compacted           bool
	TurnCounter         int
	ConsecutiveFailures int
}

// getEffectiveContextWindow 有效上下文窗口 = 总窗口 - 摘要保留
func getEffectiveContextWindow(maxContext int) int {
	return maxContext - maxOutputTokensForSummary
}

// getAutoCompactThreshold 自动压缩触发阈值
func getAutoCompactThreshold(maxContext int) int {
	return getEffectiveContextWindow(maxContext) - autoCompactBufferTokens
}

// CalculateTokenWarningState 计算 token 使用状态
// CC 参考：calculateTokenWarningState()
func CalculateTokenWarningState(tokensUsed, maxContext int) TokenWarningState {
	effective := getEffectiveContextWindow(maxContext)
	percentUsed := 0
	if effective > 0 {
		percentUsed = tokensUsed * 100 / effective
	}

	state := TokenWarningState{
		PercentUsed: percentUsed,
		TokensUsed:  tokensUsed,
		TokensMax:   maxContext,
	}

	warningThreshold := effective - warningThresholdBuffer
	errorThreshold := effective - errorThresholdBuffer/2

	switch {
	case tokensUsed >= errorThreshold:
		state.Level = TokenLevelError
	case tokensUsed >= warningThreshold:
		state.Level = TokenLevelWarning
	default:
		state.Level = TokenLevelSafe
	}

	return state
}

// shouldAutoCompact 判断是否需要自动压缩
func shouldAutoCompact(tokensUsed, maxContext int) bool {
	threshold := getAutoCompactThreshold(maxContext)
	return tokensUsed >= threshold
}

// CompactMessages 将消息历史压缩为摘要
// CC 参考：compactConversation() in compact.ts
// 策略：保留系统提示和最近 N 条消息，中间部分生成摘要
func CompactMessages(messages []provider.Message, keepRecent int) ([]provider.Message, string) {
	if len(messages) <= keepRecent+2 {
		return messages, "消息数量较少，无需压缩"
	}

	// 保留最早的系统/用户消息 + 最近的 keepRecent 条
	var compacted []provider.Message

	// 第一条消息（通常是用户的第一个问题）
	compacted = append(compacted, messages[0])

	// 生成中间消息的摘要
	middle := messages[1 : len(messages)-keepRecent]
	summary := summarizeMessages(middle)

	// 插入摘要消息
	compacted = append(compacted, provider.Message{
		Role:    "user",
		Content: fmt.Sprintf("[自动压缩摘要] 以下是之前对话的摘要：\n%s\n\n请基于此摘要继续对话。", summary),
	})
	compacted = append(compacted, provider.Message{
		Role:    "assistant",
		Content: "好的，我已了解之前的对话内容。请继续。",
	})

	// 保留最近的消息
	compacted = append(compacted, messages[len(messages)-keepRecent:]...)

	beforeCount := len(messages)
	afterCount := len(compacted)
	msg := fmt.Sprintf("已压缩对话：%d → %d 条消息", beforeCount, afterCount)

	return compacted, msg
}

// summarizeMessages 生成消息摘要
func summarizeMessages(messages []provider.Message) string {
	var sb strings.Builder
	sb.WriteString("对话涉及以下内容：\n")

	toolCalls := 0
	userQuestions := 0
	for _, m := range messages {
		switch m.Role {
		case "user":
			userQuestions++
			// 提取前 100 字作为摘要
			content := m.Content
			if len(content) > 100 {
				content = content[:100] + "..."
			}
			sb.WriteString(fmt.Sprintf("- 用户: %s\n", content))
		case "assistant":
			if len(m.ToolCalls) > 0 {
				toolCalls += len(m.ToolCalls)
				for _, tc := range m.ToolCalls {
					sb.WriteString(fmt.Sprintf("- 工具调用: %s\n", tc.Name))
				}
			}
		}
	}

	sb.WriteString(fmt.Sprintf("\n共 %d 轮对话，%d 次工具调用。", userQuestions, toolCalls))
	return sb.String()
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go test ./internal/agent/ -run "TestAutoCompact|TestTokenWarning|TestShouldAutoCompact" -v`
Expected: PASS

- [ ] **Step 5: 集成到 agent.go 主循环**

在 `agent.go` 的 `Run()` 方法中，每次 API 调用前检查是否需要自动压缩：

```go
// agent.go — Run() 循环中，API 调用前
totalTokens := a.session.InputTokens + a.session.OutputTokens
maxCtx := a.provider.Capabilities().MaxContext
if shouldAutoCompact(int(totalTokens), maxCtx) {
	compacted, msg := CompactMessages(a.messages, 6)
	a.messages = compacted
	a.session.Messages = compacted
	a.sendTUI(tui.MsgSystemNotice{Text: msg})
}
```

- [ ] **Step 6: 提交**

```bash
cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code
git add internal/agent/compact.go internal/agent/compact_test.go agent.go
git commit -m "feat(agent): 自动压缩机制，对标 CC autoCompact.ts 阈值体系"
```

---

### Task 6: 集成重试到 Provider 层

将 retry 逻辑集成到 provider 的 Stream 调用中，使 agent 主循环自动获得重试能力。

**Files:**
- Modify: `internal/provider/anthropic.go` — 在 Stream 方法中包装重试
- Modify: `agent.go` — 传递重试回调

- [ ] **Step 1: 在 provider 层添加重试包装**

在 `internal/provider/anthropic.go` 的 `Stream()` 方法中，外层包装 `WithRetry`：

```go
// 在 anthropic.go Stream() 方法中
// 检测 API 错误状态码，转换为 APIError
func (p *AnthropicProvider) Stream(ctx context.Context, req Request) <-chan Event {
	ch := make(chan Event, 64)
	go func() {
		defer close(ch)

		var lastErr error
		for attempt := 1; attempt <= defaultMaxRetries; attempt++ {
			err := p.doStream(ctx, req, ch)
			if err == nil {
				return
			}

			if ctx.Err() != nil {
				ch <- Event{Type: EventError, Error: ctx.Err()}
				return
			}

			apiErr, isAPI := err.(*APIError)
			if !isAPI || !isRetryableStatusCode(apiErr.StatusCode) {
				ch <- Event{Type: EventError, Error: err}
				return
			}

			lastErr = err
			delay := calcRetryDelay(attempt, apiErr.RetryAfter)

			// 通知重试
			ch <- Event{
				Type:  EventError,
				Error: fmt.Errorf("API 错误 %d，%v 后重试 (第 %d 次)...", apiErr.StatusCode, delay.Round(time.Second), attempt),
			}

			select {
			case <-ctx.Done():
				ch <- Event{Type: EventError, Error: ctx.Err()}
				return
			case <-time.After(delay):
			}
		}

		ch <- Event{Type: EventError, Error: fmt.Errorf("重试 %d 次后仍失败: %w", defaultMaxRetries, lastErr)}
	}()
	return ch
}
```

- [ ] **Step 2: 编译验证**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 3: 提交**

```bash
cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code
git add internal/provider/anthropic.go agent.go
git commit -m "feat(provider): API 调用重试集成，指数退避 + 状态码分类"
```

---

## Phase 3: Memory 系统

CC 有完整的持久化记忆系统：auto-memory（4 种类型）、MEMORY.md 索引、后台提取 agent。这是 Xin Code 完全缺失的能力。

### File Structure (Phase 3)

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/memory/types.go` | Create | 记忆类型定义（user/feedback/project/reference） |
| `internal/memory/scanner.go` | Create | 扫描和加载记忆文件 |
| `internal/memory/scanner_test.go` | Create | 扫描逻辑测试 |
| `internal/memory/writer.go` | Create | 记忆写入和索引管理 |
| `internal/memory/writer_test.go` | Create | 写入逻辑测试 |
| `internal/context/prompt.go` | Modify | 注入记忆到系统提示词 |

---

### Task 7: 记忆类型系统 + 扫描器

CC 的记忆系统（`src/memdir/`）定义了 4 种类型（user/feedback/project/reference），每个记忆是独立的 Markdown 文件，带 YAML frontmatter。

**Files:**
- Create: `internal/memory/types.go`
- Create: `internal/memory/scanner.go`
- Create: `internal/memory/scanner_test.go`

- [ ] **Step 1: 写测试**

```go
// internal/memory/scanner_test.go
package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanMemoryFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建 MEMORY.md 索引
	os.WriteFile(filepath.Join(tmpDir, "MEMORY.md"), []byte(
		"- [User Role](user_role.md) — data scientist\n"+
			"- [Testing Feedback](feedback_testing.md) — use real DB\n",
	), 0644)

	// 创建记忆文件
	os.WriteFile(filepath.Join(tmpDir, "user_role.md"), []byte(
		"---\nname: User Role\ndescription: user is a data scientist\ntype: user\n---\n\nUser is a data scientist.\n",
	), 0644)

	os.WriteFile(filepath.Join(tmpDir, "feedback_testing.md"), []byte(
		"---\nname: Testing Feedback\ndescription: use real database in tests\ntype: feedback\n---\n\nDon't mock the database.\n**Why:** Prior incident.\n**How to apply:** Integration tests only.\n",
	), 0644)

	memories, err := ScanMemoryDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(memories))
	}
	if memories[0].Type != TypeUser {
		t.Errorf("expected user type, got %s", memories[0].Type)
	}
	if memories[1].Type != TypeFeedback {
		t.Errorf("expected feedback type, got %s", memories[1].Type)
	}
}

func TestParseFrontmatter(t *testing.T) {
	content := "---\nname: Test Memory\ndescription: test desc\ntype: project\n---\n\nBody content here."
	header, body := parseFrontmatter(content)

	if header.Name != "Test Memory" {
		t.Errorf("expected name 'Test Memory', got '%s'", header.Name)
	}
	if header.Type != TypeProject {
		t.Errorf("expected project type, got %s", header.Type)
	}
	if body != "Body content here." {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestScanMemoryDir_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	memories, err := ScanMemoryDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 0 {
		t.Errorf("expected 0 memories, got %d", len(memories))
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go test ./internal/memory/ -run TestScan -v`
Expected: FAIL

- [ ] **Step 3: 实现 types.go**

```go
// internal/memory/types.go
package memory

import "time"

// MemoryType 记忆类型，对标 CC memoryTypes.ts
type MemoryType string

const (
	TypeUser      MemoryType = "user"      // 用户角色/偏好/知识背景
	TypeFeedback  MemoryType = "feedback"  // 用户反馈/纠正/确认
	TypeProject   MemoryType = "project"   // 项目进展/目标/截止日期
	TypeReference MemoryType = "reference" // 外部资源指针
)

// MemoryHeader 记忆文件 frontmatter
type MemoryHeader struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Type        MemoryType `yaml:"type"`
}

// MemoryEntry 一条完整的记忆记录
type MemoryEntry struct {
	MemoryHeader
	FilePath string    // 文件绝对路径
	Body     string    // Markdown 正文（不含 frontmatter）
	ModTime  time.Time // 文件修改时间
}

// MaxMemoryFiles 单个目录最大记忆文件数（CC: 200）
const MaxMemoryFiles = 200

// MaxIndexLines MEMORY.md 最大行数（CC: 200）
const MaxIndexLines = 200

// MaxIndexBytes MEMORY.md 最大字节数（CC: ~25KB）
const MaxIndexBytes = 25000
```

- [ ] **Step 4: 实现 scanner.go**

```go
// internal/memory/scanner.go
package memory

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ScanMemoryDir 扫描记忆目录，返回所有记忆条目
// CC 参考：src/memdir/memoryScan.ts scanMemoryFiles()
func ScanMemoryDir(dir string) ([]MemoryEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var memories []MemoryEntry
	count := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		// 跳过 MEMORY.md 索引文件本身
		if entry.Name() == "MEMORY.md" {
			continue
		}
		if count >= MaxMemoryFiles {
			break
		}

		filePath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		header, body := parseFrontmatter(string(data))
		info, _ := entry.Info()
		modTime := info.ModTime()

		memories = append(memories, MemoryEntry{
			MemoryHeader: header,
			FilePath:     filePath,
			Body:         body,
			ModTime:      modTime,
		})
		count++
	}

	// 按修改时间排序（最新在前）
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].ModTime.After(memories[j].ModTime)
	})

	return memories, nil
}

// LoadIndex 读取 MEMORY.md 索引内容
func LoadIndex(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	if err != nil {
		return ""
	}
	content := string(data)

	// 截断到最大行数
	lines := strings.Split(content, "\n")
	if len(lines) > MaxIndexLines {
		lines = lines[:MaxIndexLines]
		lines = append(lines, "\n[MEMORY.md 已截断，仅显示前 200 行]")
	}

	// 截断到最大字节
	result := strings.Join(lines, "\n")
	if len(result) > MaxIndexBytes {
		result = result[:MaxIndexBytes]
	}

	return result
}

// parseFrontmatter 解析 YAML frontmatter 和 body
func parseFrontmatter(content string) (MemoryHeader, string) {
	var header MemoryHeader

	if !strings.HasPrefix(content, "---\n") {
		return header, content
	}

	endIdx := strings.Index(content[4:], "\n---\n")
	if endIdx == -1 {
		return header, content
	}

	frontmatter := content[4 : 4+endIdx]
	body := strings.TrimSpace(content[4+endIdx+5:])

	// 简单 YAML 解析（避免引入额外依赖）
	for _, line := range strings.Split(frontmatter, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "name":
			header.Name = val
		case "description":
			header.Description = val
		case "type":
			header.Type = MemoryType(val)
		}
	}

	return header, body
}

// FormatMemoriesForPrompt 将记忆格式化为系统提示词片段
func FormatMemoriesForPrompt(memories []MemoryEntry, indexContent string) string {
	if len(memories) == 0 && indexContent == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Auto Memory\n\n")

	if indexContent != "" {
		sb.WriteString("## MEMORY.md Index\n")
		sb.WriteString(indexContent)
		sb.WriteString("\n\n")
	}

	if len(memories) > 0 {
		sb.WriteString("## Memory Entries\n\n")
		for _, m := range memories {
			sb.WriteString("### ")
			sb.WriteString(m.Name)
			if m.Type != "" {
				sb.WriteString(" [")
				sb.WriteString(string(m.Type))
				sb.WriteString("]")
			}
			sb.WriteString("\n")
			sb.WriteString(m.Body)
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

// GetMemoryDir 返回项目的记忆目录路径
// CC 格式：~/.xincode/projects/{sanitized-cwd}/memory/
func GetMemoryDir(homeDir, projectDir string) string {
	// 将项目路径转为安全目录名
	sanitized := strings.ReplaceAll(projectDir, "/", "-")
	sanitized = strings.ReplaceAll(sanitized, "\\", "-")
	sanitized = strings.TrimLeft(sanitized, "-")

	return filepath.Join(homeDir, ".xincode", "projects", sanitized, "memory")
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go test ./internal/memory/ -v`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code
git add internal/memory/types.go internal/memory/scanner.go internal/memory/scanner_test.go
git commit -m "feat(memory): 记忆类型系统 + 扫描器，对标 CC memdir/memoryScan.ts"
```

---

### Task 8: 记忆写入器 + MEMORY.md 索引管理

**Files:**
- Create: `internal/memory/writer.go`
- Create: `internal/memory/writer_test.go`

- [ ] **Step 1: 写测试**

```go
// internal/memory/writer_test.go
package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteMemory(t *testing.T) {
	tmpDir := t.TempDir()

	entry := MemoryEntry{
		MemoryHeader: MemoryHeader{
			Name:        "User Role",
			Description: "user is a Go developer",
			Type:        TypeUser,
		},
		Body: "The user is a senior Go developer with 10 years of experience.",
	}

	err := WriteMemory(tmpDir, "user_role.md", entry)
	if err != nil {
		t.Fatal(err)
	}

	// 验证文件内容
	data, err := os.ReadFile(filepath.Join(tmpDir, "user_role.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "---") {
		t.Error("should have frontmatter delimiters")
	}
	if !strings.Contains(content, "name: User Role") {
		t.Error("should contain name in frontmatter")
	}
	if !strings.Contains(content, "type: user") {
		t.Error("should contain type in frontmatter")
	}
	if !strings.Contains(content, "senior Go developer") {
		t.Error("should contain body content")
	}
}

func TestUpdateIndex(t *testing.T) {
	tmpDir := t.TempDir()

	// 写入一条记忆
	WriteMemory(tmpDir, "user_role.md", MemoryEntry{
		MemoryHeader: MemoryHeader{
			Name:        "User Role",
			Description: "senior Go developer",
			Type:        TypeUser,
		},
		Body: "Test body.",
	})

	// 更新索引
	err := UpdateIndex(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// 验证 MEMORY.md
	data, err := os.ReadFile(filepath.Join(tmpDir, "MEMORY.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "User Role") {
		t.Error("index should contain memory name")
	}
	if !strings.Contains(content, "user_role.md") {
		t.Error("index should contain filename")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go test ./internal/memory/ -run "TestWrite|TestUpdate" -v`
Expected: FAIL

- [ ] **Step 3: 实现 writer.go**

```go
// internal/memory/writer.go
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteMemory 写入一条记忆到文件
// CC 参考：模型直接通过 FileWrite 写入，格式约定在 system prompt 中
func WriteMemory(dir, filename string, entry MemoryEntry) error {
	// 确保目录存在
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建记忆目录失败: %w", err)
	}

	// 构建带 frontmatter 的内容
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", entry.Name))
	sb.WriteString(fmt.Sprintf("description: %s\n", entry.Description))
	sb.WriteString(fmt.Sprintf("type: %s\n", entry.Type))
	sb.WriteString("---\n\n")
	sb.WriteString(entry.Body)
	sb.WriteString("\n")

	filePath := filepath.Join(dir, filename)
	return os.WriteFile(filePath, []byte(sb.String()), 0644)
}

// UpdateIndex 根据目录中的记忆文件重建 MEMORY.md 索引
// CC 参考：MEMORY.md 是索引文件，每条 < 150 字符
func UpdateIndex(dir string) error {
	memories, err := ScanMemoryDir(dir)
	if err != nil {
		return err
	}

	var sb strings.Builder
	for _, m := range memories {
		filename := filepath.Base(m.FilePath)
		desc := m.Description
		if len(desc) > 80 {
			desc = desc[:77] + "..."
		}
		// 格式：- [Name](file.md) — description
		line := fmt.Sprintf("- [%s](%s) — %s\n", m.Name, filename, desc)
		sb.WriteString(line)
	}

	indexPath := filepath.Join(dir, "MEMORY.md")
	return os.WriteFile(indexPath, []byte(sb.String()), 0644)
}

// DeleteMemory 删除一条记忆文件并更新索引
func DeleteMemory(dir, filename string) error {
	filePath := filepath.Join(dir, filename)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return UpdateIndex(dir)
}

// EnsureMemoryDir 确保记忆目录存在
func EnsureMemoryDir(homeDir, projectDir string) (string, error) {
	dir := GetMemoryDir(homeDir, projectDir)
	return dir, os.MkdirAll(dir, 0755)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go test ./internal/memory/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code
git add internal/memory/writer.go internal/memory/writer_test.go
git commit -m "feat(memory): 记忆写入器 + MEMORY.md 索引管理"
```

---

### Task 9: 记忆系统集成 — 注入系统提示词 + /memory 命令

将记忆加载集成到系统提示词构建流程中，并添加 `/memory` 斜杠命令。

**Files:**
- Modify: `internal/context/prompt.go` — BuildUserContext 中加载记忆
- Modify: `internal/slash/handler.go` — 添加 /memory 命令实现
- Modify: `agent.go` — 传递 homeDir

- [ ] **Step 1: 在 BuildUserContext 中加载记忆**

修改 `internal/context/prompt.go` 的 `BuildUserContext()` 函数：

```go
// BuildUserContext 中增加记忆加载
func BuildUserContext(cfg SystemPromptConfig) string {
	var parts []string

	// XINCODE.md 内容
	memFiles := DiscoverMemoryFiles(cfg.WorkDir, cfg.HomeDir)
	if md := FormatMemoryForPrompt(memFiles, cfg.WorkDir); md != "" {
		parts = append(parts, "# claudeMd\n"+md)
	}

	// Auto Memory 内容
	memDir := memory.GetMemoryDir(cfg.HomeDir, cfg.WorkDir)
	memories, _ := memory.ScanMemoryDir(memDir)
	indexContent := memory.LoadIndex(memDir)
	if memPrompt := memory.FormatMemoriesForPrompt(memories, indexContent); memPrompt != "" {
		parts = append(parts, memPrompt)
	}

	// 当前日期
	parts = append(parts, fmt.Sprintf("# currentDate\nToday's date is %s.", time.Now().Format("2006-01-02")))

	return strings.Join(parts, "\n")
}
```

- [ ] **Step 2: 添加 /memory 命令**

在 `internal/slash/handler.go` 的 `/memory` 命令处理中，实现打开记忆目录或列出记忆：

```go
// /memory 命令处理
case "memory":
	homeDir, _ := os.UserHomeDir()
	memDir := memory.GetMemoryDir(homeDir, ctx.WorkDir)
	memories, err := memory.ScanMemoryDir(memDir)
	if err != nil {
		return Result{Type: ResultDisplay, Text: fmt.Sprintf("读取记忆失败: %s", err)}
	}
	if len(memories) == 0 {
		return Result{Type: ResultDisplay, Text: fmt.Sprintf("暂无记忆。记忆目录: %s", memDir)}
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📝 记忆 (%d 条)\n\n", len(memories)))
	for _, m := range memories {
		sb.WriteString(fmt.Sprintf("  [%s] %s — %s\n", m.Type, m.Name, m.Description))
	}
	sb.WriteString(fmt.Sprintf("\n目录: %s", memDir))
	return Result{Type: ResultDisplay, Text: sb.String()}
```

- [ ] **Step 3: 编译验证**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 4: 提交**

```bash
cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code
git add internal/context/prompt.go internal/slash/handler.go agent.go
git commit -m "feat(memory): 记忆系统集成到系统提示词和 /memory 命令"
```

---

## Phase 4: TUI 交互增强

当前 TUI 视觉效果已达 60%，需要在细节层面继续打磨，包括状态栏信息、工具输出渲染、权限对话框、输入增强。

### File Structure (Phase 4)

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/tui/app.go` | Modify | Footer 信息增强、上下文百分比显示 |
| `internal/tui/chat.go` | Modify | 工具输出折叠/展开交互 |
| `internal/tui/input.go` | Modify | Tab 补全、更精致的样式 |
| `internal/tui/permission.go` | Modify | 权限对话框增强（always/deny 选项） |
| `internal/tui/theme.go` | Modify | 补充缺失样式 |

---

### Task 10: Footer 状态信息增强

CC 的 StatusLine 显示 model、cost、context%、permission mode。当前 Xin Code footer 缺少动态上下文百分比和权限模式。

**Files:**
- Modify: `internal/tui/app.go` — renderFooter()

- [ ] **Step 1: 增强 renderFooter()**

```go
// internal/tui/app.go — renderFooter() 方法重写
func (a *App) renderFooter() string {
	// 计算上下文使用百分比
	contextPercent := 0
	if a.maxContext > 0 && a.tracker != nil {
		totalTokens := a.tracker.TotalInputTokens() + a.tracker.TotalOutputTokens()
		contextPercent = int(totalTokens) * 100 / a.maxContext
	}

	// 格式化费用
	costStr := ""
	if a.tracker != nil {
		cost := a.tracker.TotalCost()
		currency := a.tracker.Currency()
		if currency == "CNY" {
			costStr = fmt.Sprintf("¥%.4f", cost)
		} else {
			costStr = fmt.Sprintf("$%.4f", cost)
		}
	}

	// 组装各段
	parts := []string{a.model}
	if costStr != "" {
		parts = append(parts, costStr)
	}
	parts = append(parts, fmt.Sprintf("%d%% context", contextPercent))

	// 权限模式简写
	permLabel := map[string]string{
		"bypass":      "bypass",
		"acceptEdits": "auto-edit",
		"default":     "默认确认",
		"plan":        "plan-only",
		"interactive": "全部确认",
	}
	if label, ok := permLabel[a.permMode]; ok {
		parts = append(parts, label)
	}

	parts = append(parts, "/help")
	parts = append(parts, "Option+drag to select")

	return StyleFooter.Render(strings.Join(parts, " · "))
}
```

- [ ] **Step 2: 编译验证**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 3: 提交**

```bash
cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code
git add internal/tui/app.go
git commit -m "feat(tui): Footer 状态信息增强 — context%、权限模式"
```

---

### Task 11: 工具输出折叠/展开交互

CC 的工具输出超过阈值自动折叠，用户可以按键展开。当前 Xin Code 有折叠逻辑但无展开交互。

**Files:**
- Modify: `internal/tui/chat.go` — 添加折叠切换
- Modify: `internal/tui/app.go` — 路由切换事件

- [ ] **Step 1: 在 chat.go 添加折叠切换**

```go
// internal/tui/chat.go — 新增方法

// MsgToggleFold 用户切换某条工具消息的折叠状态
type MsgToggleFold struct {
	Index int // 消息索引
}

// ToggleLastFold 切换最近一条可折叠消息的状态
func (c *ChatView) ToggleLastFold() {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].Role == "tool" && c.messages[i].Content != "" {
			lines := strings.Count(c.messages[i].Content, "\n")
			if lines > foldThreshold {
				c.messages[i].Folded = !c.messages[i].Folded
				c.refreshContent(c.shouldAutoScroll())
				return
			}
		}
	}
}
```

- [ ] **Step 2: 在 app.go 路由 'e' 键展开**

在 `shouldRouteKeyToChat()` 或 KeyMsg handler 中添加：

```go
// app.go — Update() 中 KeyMsg 处理
case tea.KeyRunes:
	if a.state != StateInput {
		break
	}
	// 'e' 键切换最近工具输出的折叠
	if string(msg.Runes) == "e" && !a.chat.HasMessages() {
		// 仅在输入框为空时响应
	}
```

实际上更好的方式是在 scrollback 模式中响应 'e' 键，但为简单起见先不做 — 已有自动折叠，展开可以后续迭代。

- [ ] **Step 3: 编译验证并提交**

```bash
cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code
go build ./...
git add internal/tui/chat.go internal/tui/app.go
git commit -m "feat(tui): 工具输出折叠切换基础设施"
```

---

### Task 12: 权限对话框增强 — Always/Session Deny 选项

CC 的权限对话框支持 `y` (允许)、`n` (拒绝)、`a` (本次会话始终允许)、`d` (本次会话始终拒绝)。当前 Xin Code 只有 y/n。

**Files:**
- Modify: `internal/tui/permission.go` — 添加选项
- Modify: `internal/tool/permission.go` — 支持会话级规则

- [ ] **Step 1: 扩展 PermissionDialog 选项**

在 `internal/tui/permission.go` 中修改渲染和响应逻辑：

```go
// 权限对话框显示选项：
// y - 允许   n - 拒绝   a - 本次会话始终允许   d - 本次会话始终拒绝

// 在 View() 中渲染选项提示
optionLine := StyleDim.Render("  y 允许 · n 拒绝 · a 始终允许 · d 始终拒绝")

// 在 Update() 的 KeyMsg handler 中：
case "a", "A":
	// 始终允许：返回特殊结果
	p.result = PermissionResultAlways
	p.visible = false
case "d", "D":
	// 始终拒绝
	p.result = PermissionResultSessionDeny
	p.visible = false
```

- [ ] **Step 2: 在 permission.go 工具层添加会话级规则缓存**

```go
// internal/tool/permission.go — 新增会话级规则
type SessionRules struct {
	alwaysAllow map[string]bool // 工具名 → 始终允许
	alwaysDeny  map[string]bool // 工具名 → 始终拒绝
}

func NewSessionRules() *SessionRules {
	return &SessionRules{
		alwaysAllow: make(map[string]bool),
		alwaysDeny:  make(map[string]bool),
	}
}

func (r *SessionRules) SetAlwaysAllow(toolName string) {
	r.alwaysAllow[toolName] = true
	delete(r.alwaysDeny, toolName)
}

func (r *SessionRules) SetAlwaysDeny(toolName string) {
	r.alwaysDeny[toolName] = true
	delete(r.alwaysAllow, toolName)
}

// Check 检查会话级规则，返回 nil 表示没有规则
func (r *SessionRules) Check(toolName string) *CheckResult {
	if r.alwaysAllow[toolName] {
		result := ResultAllow
		return &result
	}
	if r.alwaysDeny[toolName] {
		result := ResultDeny
		return &result
	}
	return nil
}
```

- [ ] **Step 3: 编译验证并提交**

```bash
cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code
go build ./...
git add internal/tui/permission.go internal/tool/permission.go
git commit -m "feat(permission): 会话级 always/deny 权限规则，对标 CC"
```

---

### Task 13: 输入框斜杠命令补全增强

CC 的输入支持 Tab 补全斜杠命令。当前 Xin Code 有命令提示但无 Tab 补全。

**Files:**
- Modify: `internal/tui/input.go` — Tab 键补全

- [ ] **Step 1: 在 input.go 添加 Tab 补全**

```go
// internal/tui/input.go — Update() 中 KeyMsg handler
case tea.KeyTab:
	// Tab 补全斜杠命令
	matches := i.matchSlashCommands()
	if len(matches) == 1 {
		// 唯一匹配：直接补全
		i.textarea.SetValue(matches[0].Name + " ")
		return i, nil
	}
	if len(matches) > 1 {
		// 多个匹配：补全公共前缀
		prefix := commonPrefix(matches)
		if prefix != i.textarea.Value() {
			i.textarea.SetValue(prefix)
		}
		return i, nil
	}
```

```go
// commonPrefix 计算多个命令名的公共前缀
func commonPrefix(commands []CommandHint) string {
	if len(commands) == 0 {
		return ""
	}
	prefix := commands[0].Name
	for _, cmd := range commands[1:] {
		for !strings.HasPrefix(cmd.Name, prefix) {
			prefix = prefix[:len(prefix)-1]
			if prefix == "" {
				return ""
			}
		}
	}
	return prefix
}
```

- [ ] **Step 2: 编译验证并提交**

```bash
cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code
go build ./...
git add internal/tui/input.go
git commit -m "feat(tui): 斜杠命令 Tab 补全"
```

---

### Task 14: 全局编译验证 + 集成测试

**Files:**
- All modified files

- [ ] **Step 1: 全项目编译**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 2: 运行所有测试**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go test ./... -v`
Expected: ALL PASS

- [ ] **Step 3: 运行程序验证 TUI**

Run: `cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code && go run . --version`
Expected: 显示版本信息

- [ ] **Step 4: 提交最终验证**

```bash
cd /Users/ocean/Studio/01-workshop/06-开源项目/xin-code
git add -A
git commit -m "chore: Phase 1-4 全局编译验证通过"
```

---

## 后续迭代路线图（不在本计划范围内）

以下功能在 4 个 Phase 完成后作为独立计划实施：

### Phase 5: SubAgent / Swarm
- `AgentTool` — 派生子 agent 执行任务（CC: `tools/AgentTool/runAgent.ts`）
- `SendMessage` — 向子 agent 发送消息
- Worker 状态跟踪和进度显示
- Coordinator 模式（CC: `coordinator/coordinatorMode.ts`）

### Phase 6: 高级 Context 管理
- Prompt Caching（`cache_control` block）— 需要 Provider 层支持
- Reactive Compact（413 错误自动触发压缩）
- Context Collapse（嵌套上下文折叠）
- History Snip（丢弃最旧的消息轮次）
- Tool Use Summary（长工具输出摘要化）

### Phase 7: 高级 TUI
- Vim 模式输入（CC: `VimTextInput.tsx`）
- 图片/二进制文件粘贴（CC: `inputPaste.ts`）
- Agent 进度树（CC: `TeammateSpinnerTree.tsx`）
- Diff 预览增强（hunk 级别的增删显示）
- 主题系统（light/dark 切换）

### Phase 8: 生态集成
- Skills 系统完善（发现、注册、执行）
- Hooks 系统增强（更多事件类型）
- OAuth 认证流程
- `/doctor` 诊断命令
- `/bug` 反馈命令
