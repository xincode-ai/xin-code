# Phase 8: 交互式 Provider 配置 + 生态集成

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 首次启动无 API Key 时自动进入 TUI 引导流程，引导用户选择 Provider、输入 API Key、选择模型，并持久化到配置文件。同时补齐 `/doctor`、`/login`、`/logout`、`/bug` 等实用命令。

**Architecture:** 新增 `internal/setup/` 包，实现独立的 Setup TUI（Bubbletea Model），在 `main.go` 中检测无 API Key 时先运行 Setup 再进入主 TUI。凭证存 `~/.xincode/auth/credentials.json`（分离于 settings.json），Provider/Model 偏好存 `~/.xincode/settings.json`。`/login` 命令复用 Setup 组件。`/doctor` 做环境检查。

**Tech Stack:** Go 1.26 + Bubbletea + Lipgloss + Bubbles (textarea/list)

---

## File Structure

| 文件 | 职责 |
|------|------|
| **Create:** `internal/setup/setup.go` | Setup TUI Model — 多步引导界面（选 Provider → 输 Key → 选 Model → 验证 → 保存） |
| **Create:** `internal/setup/providers.go` | Provider 列表定义（名称、默认 BaseURL、可选模型列表、Logo/颜色） |
| **Create:** `internal/setup/setup_test.go` | Setup 核心逻辑测试（Provider 过滤、凭证保存/读取、输入校验） |
| **Modify:** `internal/auth/auth.go` | 新增 `SaveCredentials()` 函数 |
| **Modify:** `internal/auth/apikey.go` | 新增 `WriteAPIKeyToFile()` 函数 |
| **Modify:** `config.go` | 新增 `SaveGlobalConfig()` 函数，合并 Provider/Model 写入 settings.json |
| **Modify:** `main.go` | 无 Key 时启动 Setup TUI，`/login` 回调注入 |
| **Modify:** `internal/slash/handler.go` | `/doctor`、`/bug` 命令实现，`/login` + `/logout` 接入真实逻辑 |

---

### Task 1: Provider 列表定义

**Files:**
- Create: `internal/setup/providers.go`

- [ ] **Step 1: 创建 Provider 列表数据结构和预置 Provider**

```go
// internal/setup/providers.go
package setup

// ProviderInfo 预置 Provider 信息
type ProviderInfo struct {
	ID          string   // "anthropic", "openai", "openrouter", "custom"
	Name        string   // 显示名
	Description string   // 一句话描述
	BaseURL     string   // 默认 API 端点（custom 为空）
	EnvKey      string   // 对应的环境变量名
	Models      []string // 推荐模型列表（首个为默认）
}

// BuiltinProviders 预置 Provider 列表
var BuiltinProviders = []ProviderInfo{
	{
		ID:          "anthropic",
		Name:        "Anthropic (Claude)",
		Description: "Claude 系列模型，推荐",
		BaseURL:     "", // 使用 SDK 默认
		EnvKey:      "ANTHROPIC_API_KEY",
		Models: []string{
			"claude-sonnet-4-6-20250514",
			"claude-opus-4-6-20250610",
			"claude-haiku-4-5-20251001",
		},
	},
	{
		ID:          "openai",
		Name:        "OpenAI",
		Description: "GPT / o 系列模型",
		BaseURL:     "",
		EnvKey:      "OPENAI_API_KEY",
		Models: []string{
			"gpt-4.1",
			"o4-mini",
			"o3",
		},
	},
	{
		ID:          "openrouter",
		Name:        "OpenRouter",
		Description: "多模型聚合，支持数百个模型",
		BaseURL:     "https://openrouter.ai/api/v1",
		EnvKey:      "OPENROUTER_API_KEY",
		Models: []string{
			"anthropic/claude-sonnet-4-6",
			"openai/gpt-4.1",
			"google/gemini-2.5-pro",
		},
	},
	{
		ID:          "custom",
		Name:        "自定义端点",
		Description: "兼容 OpenAI API 的自定义服务",
		BaseURL:     "",
		EnvKey:      "",
		Models:      []string{},
	},
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/setup/providers.go
git commit -m "feat(setup): 定义预置 Provider 列表"
```

---

### Task 2: 凭证持久化

**Files:**
- Modify: `internal/auth/apikey.go`
- Modify: `internal/auth/auth.go`
- Create: `internal/setup/setup_test.go`（先写凭证相关测试）

- [ ] **Step 1: 写凭证保存/读取的失败测试**

```go
// internal/setup/setup_test.go
package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xincode-ai/xin-code/internal/auth"
)

func TestSaveAndLoadCredentials(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "auth", "credentials.json")

	// 保存
	err := auth.SaveCredentials(credPath, "sk-test-key-12345")
	if err != nil {
		t.Fatalf("SaveCredentials failed: %v", err)
	}

	// 读取
	key := auth.ReadAPIKeyFromFile(credPath)
	if key != "sk-test-key-12345" {
		t.Errorf("expected sk-test-key-12345, got %q", key)
	}
}

func TestSaveCredentialsCreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "nested", "deep", "credentials.json")

	err := auth.SaveCredentials(credPath, "test-key")
	if err != nil {
		t.Fatalf("SaveCredentials should create parent dirs: %v", err)
	}

	if _, err := os.Stat(credPath); os.IsNotExist(err) {
		t.Error("credentials file was not created")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/setup/ -run TestSave -v`
Expected: FAIL — `auth.SaveCredentials` 和 `auth.ReadAPIKeyFromFile` 不存在

- [ ] **Step 3: 实现凭证写入函数**

在 `internal/auth/apikey.go` 追加：

```go
// SaveCredentials 将 API Key 写入 JSON 凭据文件
// 自动创建父目录
func SaveCredentials(path, apiKey string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create credentials dir: %w", err)
	}
	data, err := json.MarshalIndent(credentialsFile{APIKey: apiKey}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// ReadAPIKeyFromFile 从 JSON 文件读取 API Key（公开导出版本）
func ReadAPIKeyFromFile(path string) string {
	return readAPIKeyFromFile(path)
}
```

需要在文件顶部补充 `"fmt"` 和 `"path/filepath"` 的 import。

在 `internal/auth/auth.go` 追加：

```go
// SaveAPIKey 保存 API Key 到标准凭据路径
func SaveAPIKey(configDir, apiKey string) error {
	credPath := filepath.Join(configDir, "auth", "credentials.json")
	return SaveCredentials(credPath, apiKey)
}

// ClearAPIKey 清除已保存的 API Key
func ClearAPIKey(configDir string) error {
	credPath := filepath.Join(configDir, "auth", "credentials.json")
	return os.Remove(credPath)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/setup/ -run TestSave -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/auth/apikey.go internal/auth/auth.go internal/setup/setup_test.go
git commit -m "feat(auth): 凭证持久化 — SaveCredentials/ClearAPIKey"
```

---

### Task 3: Config 持久化

**Files:**
- Modify: `config.go`

- [ ] **Step 1: 写 config 保存的测试**

在 `internal/setup/setup_test.go` 追加：

```go
func TestSaveGlobalConfig(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")

	err := SaveGlobalSettings(settingsPath, map[string]string{
		"provider": "anthropic",
		"model":    "claude-sonnet-4-6-20250514",
	})
	if err != nil {
		t.Fatalf("SaveGlobalSettings failed: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "anthropic") {
		t.Errorf("settings should contain provider, got: %s", content)
	}
	if !strings.Contains(content, "claude-sonnet") {
		t.Errorf("settings should contain model, got: %s", content)
	}
}
```

需要在 test 文件 import 中补充 `"strings"`。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/setup/ -run TestSaveGlobal -v`
Expected: FAIL — `SaveGlobalSettings` 不存在

- [ ] **Step 3: 在 setup 包中实现 SaveGlobalSettings**

在 `internal/setup/providers.go` 底部追加（因为它和 provider/config 逻辑相关）：

```go
// SaveGlobalSettings 写入/合并全局设置到 settings.json
// fields 是需要更新的 key-value 对，如 {"provider": "anthropic", "model": "xxx"}
func SaveGlobalSettings(path string, fields map[string]string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 读取现有配置
	existing := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &existing)
	}

	// 合并新字段
	for k, v := range fields {
		existing[k] = v
	}

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
```

需要在 providers.go 顶部添加 import：`"encoding/json"`, `"os"`, `"path/filepath"`。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/setup/ -run TestSaveGlobal -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/setup/providers.go internal/setup/setup_test.go
git commit -m "feat(setup): config 持久化 — SaveGlobalSettings 合并写入"
```

---

### Task 4: Setup TUI Model

**Files:**
- Create: `internal/setup/setup.go`

这是核心组件：多步引导 TUI。使用 Bubbletea Model，步骤状态机驱动。

- [ ] **Step 1: 创建 Setup TUI — 数据结构和状态机**

```go
// internal/setup/setup.go
package setup

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/xincode-ai/xin-code/internal/auth"
)

// SetupStep 引导步骤
type SetupStep int

const (
	StepProvider  SetupStep = iota // 选择 Provider
	StepAPIKey                     // 输入 API Key
	StepBaseURL                    // 输入自定义 BaseURL（仅 custom provider）
	StepModel                     // 选择模型
	StepConfirm                   // 确认并保存
	StepDone                      // 完成
)

// Result Setup 完成后的结果
type Result struct {
	Provider string
	APIKey   string
	BaseURL  string
	Model    string
}

// Model Setup TUI Model
type Model struct {
	step         SetupStep
	configDir    string
	width        int
	height       int

	// 选择态
	providers    []ProviderInfo
	providerIdx  int
	modelIdx     int

	// 输入态
	apiKeyInput  textarea.Model
	baseURLInput textarea.Model

	// 结果
	result   Result
	err      error
	quitting bool
}

// New 创建 Setup TUI
func New(configDir string) Model {
	apiKeyTA := textarea.New()
	apiKeyTA.Placeholder = "sk-..."
	apiKeyTA.SetHeight(1)
	apiKeyTA.MaxHeight = 1
	apiKeyTA.ShowLineNumbers = false
	apiKeyTA.CharLimit = 256

	baseURLTA := textarea.New()
	baseURLTA.Placeholder = "https://api.example.com/v1"
	baseURLTA.SetHeight(1)
	baseURLTA.MaxHeight = 1
	baseURLTA.ShowLineNumbers = false
	baseURLTA.CharLimit = 256

	return Model{
		step:         StepProvider,
		configDir:    configDir,
		providers:    BuiltinProviders,
		providerIdx:  0,
		modelIdx:     0,
		apiKeyInput:  apiKeyTA,
		baseURLInput: baseURLTA,
	}
}

// GetResult 获取 Setup 结果
func (m Model) GetResult() (Result, error) {
	return m.result, m.err
}

func (m Model) Init() tea.Cmd {
	return textarea.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.apiKeyInput.SetWidth(min(60, m.width-10))
		m.baseURLInput.SetWidth(min(60, m.width-10))
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			return m, tea.Quit
		}

		switch m.step {
		case StepProvider:
			return m.updateProvider(msg)
		case StepAPIKey:
			return m.updateAPIKey(msg)
		case StepBaseURL:
			return m.updateBaseURL(msg)
		case StepModel:
			return m.updateModel(msg)
		case StepConfirm:
			return m.updateConfirm(msg)
		}
	}

	// 转发到当前活跃的 textarea
	switch m.step {
	case StepAPIKey:
		var cmd tea.Cmd
		m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
		return m, cmd
	case StepBaseURL:
		var cmd tea.Cmd
		m.baseURLInput, cmd = m.baseURLInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) selectedProvider() ProviderInfo {
	return m.providers[m.providerIdx]
}

func (m Model) updateProvider(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if m.providerIdx > 0 {
			m.providerIdx--
		}
	case tea.KeyDown:
		if m.providerIdx < len(m.providers)-1 {
			m.providerIdx++
		}
	case tea.KeyEnter:
		p := m.selectedProvider()
		m.result.Provider = p.ID
		m.result.BaseURL = p.BaseURL
		m.step = StepAPIKey
		m.apiKeyInput.Focus()
		return m, nil
	}
	return m, nil
}

func (m Model) updateAPIKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter && !msg.Alt {
		key := strings.TrimSpace(m.apiKeyInput.Value())
		if key == "" {
			return m, nil
		}
		m.result.APIKey = key
		if m.result.Provider == "custom" {
			m.step = StepBaseURL
			m.baseURLInput.Focus()
		} else {
			m.step = StepModel
			m.modelIdx = 0
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
	return m, cmd
}

func (m Model) updateBaseURL(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter && !msg.Alt {
		url := strings.TrimSpace(m.baseURLInput.Value())
		if url == "" {
			return m, nil
		}
		m.result.BaseURL = url
		m.step = StepModel
		m.modelIdx = 0
		return m, nil
	}
	var cmd tea.Cmd
	m.baseURLInput, cmd = m.baseURLInput.Update(msg)
	return m, cmd
}

func (m Model) updateModel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	models := m.selectedProvider().Models
	if len(models) == 0 {
		// custom provider 没有预置模型列表，跳到确认
		m.result.Model = "gpt-4.1" // 兜底默认
		m.step = StepConfirm
		return m, nil
	}
	switch msg.Type {
	case tea.KeyUp:
		if m.modelIdx > 0 {
			m.modelIdx--
		}
	case tea.KeyDown:
		if m.modelIdx < len(models)-1 {
			m.modelIdx++
		}
	case tea.KeyEnter:
		m.result.Model = models[m.modelIdx]
		m.step = StepConfirm
	}
	return m, nil
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		// 保存凭证
		if err := auth.SaveAPIKey(m.configDir, m.result.APIKey); err != nil {
			m.err = fmt.Errorf("保存凭证失败: %w", err)
			m.step = StepDone
			return m, tea.Quit
		}
		// 保存设置
		settingsPath := filepath.Join(m.configDir, "settings.json")
		fields := map[string]string{
			"provider": m.result.Provider,
			"model":    m.result.Model,
		}
		if m.result.BaseURL != "" {
			fields["base_url"] = m.result.BaseURL
		}
		if err := SaveGlobalSettings(settingsPath, fields); err != nil {
			m.err = fmt.Errorf("保存设置失败: %w", err)
		}
		m.step = StepDone
		return m, tea.Quit
	case "n", "N":
		m.step = StepProvider
		m.providerIdx = 0
	}
	return m, nil
}

// --- View ---

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	// 品牌色和样式（复用 tui 的色系）
	orange := lipgloss.NewStyle().Foreground(lipgloss.Color("#D77757"))
	bold := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#A0A0A0"))
	highlight := lipgloss.NewStyle().Foreground(lipgloss.Color("#D77757")).Bold(true)

	// 头部
	art := []string{
		"  ▀▄ ▄▀",
		"    █  ",
		"  ▄▀ ▀▄",
	}
	var header []string
	header = append(header, "")
	for i, line := range art {
		right := ""
		switch i {
		case 0:
			right = bold.Render("Xin Code Setup")
		case 1:
			right = dim.Render("首次配置向导")
		}
		header = append(header, orange.Render(line)+"    "+right)
	}
	header = append(header, "")

	// 步骤进度
	steps := []string{"Provider", "API Key", "Model", "确认"}
	var progress []string
	for i, name := range steps {
		stepNum := SetupStep(i)
		if i == 2 {
			stepNum = StepModel
		}
		if i == 3 {
			stepNum = StepConfirm
		}
		if stepNum < m.step {
			progress = append(progress, dim.Render("✓ "+name))
		} else if stepNum == m.step {
			progress = append(progress, highlight.Render("● "+name))
		} else {
			progress = append(progress, dim.Render("○ "+name))
		}
	}
	progressLine := "  " + strings.Join(progress, "  →  ")

	// 内容区
	var content string
	switch m.step {
	case StepProvider:
		content = m.viewProviderSelect(bold, dim, highlight)
	case StepAPIKey:
		content = m.viewAPIKeyInput(bold, dim)
	case StepBaseURL:
		content = m.viewBaseURLInput(bold, dim)
	case StepModel:
		content = m.viewModelSelect(bold, dim, highlight)
	case StepConfirm:
		content = m.viewConfirm(bold, dim, highlight)
	case StepDone:
		if m.err != nil {
			content = lipgloss.NewStyle().Foreground(lipgloss.Color("#CC3333")).Render("  ✗ " + m.err.Error())
		} else {
			content = highlight.Render("  ✓ 配置已保存，开始使用 Xin Code！")
		}
	}

	return strings.Join(header, "\n") + "\n" + progressLine + "\n\n" + content + "\n"
}

func (m Model) viewProviderSelect(bold, dim, hl lipgloss.Style) string {
	var lines []string
	lines = append(lines, bold.Render("  选择 Provider")+"  "+dim.Render("↑/↓ 选择  Enter 确认"))
	lines = append(lines, "")
	for i, p := range m.providers {
		cursor := "  "
		style := dim
		if i == m.providerIdx {
			cursor = hl.Render("❯ ")
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
		}
		lines = append(lines, fmt.Sprintf("  %s%s  %s",
			cursor, style.Render(p.Name), dim.Render(p.Description)))
	}
	return strings.Join(lines, "\n")
}

func (m Model) viewAPIKeyInput(bold, dim lipgloss.Style) string {
	p := m.selectedProvider()
	var lines []string
	lines = append(lines, bold.Render("  输入 API Key"))
	if p.EnvKey != "" {
		lines = append(lines, dim.Render(fmt.Sprintf("  （也可通过 export %s=... 设置）", p.EnvKey)))
	}
	lines = append(lines, "")
	lines = append(lines, "  "+m.apiKeyInput.View())
	lines = append(lines, "")
	lines = append(lines, dim.Render("  Enter 确认  Esc 取消"))
	return strings.Join(lines, "\n")
}

func (m Model) viewBaseURLInput(bold, dim lipgloss.Style) string {
	var lines []string
	lines = append(lines, bold.Render("  输入 API 端点"))
	lines = append(lines, dim.Render("  兼容 OpenAI API 协议的 Base URL"))
	lines = append(lines, "")
	lines = append(lines, "  "+m.baseURLInput.View())
	lines = append(lines, "")
	lines = append(lines, dim.Render("  Enter 确认  Esc 取消"))
	return strings.Join(lines, "\n")
}

func (m Model) viewModelSelect(bold, dim, hl lipgloss.Style) string {
	p := m.selectedProvider()
	models := p.Models
	if len(models) == 0 {
		return bold.Render("  无预置模型列表，将使用默认模型")
	}
	var lines []string
	lines = append(lines, bold.Render("  选择默认模型")+"  "+dim.Render("↑/↓ 选择  Enter 确认"))
	lines = append(lines, "")
	for i, model := range models {
		cursor := "  "
		style := dim
		if i == m.modelIdx {
			cursor = hl.Render("❯ ")
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
		}
		label := model
		if i == 0 {
			label += dim.Render("  (推荐)")
		}
		lines = append(lines, fmt.Sprintf("  %s%s", cursor, style.Render(label)))
	}
	return strings.Join(lines, "\n")
}

func (m Model) viewConfirm(bold, dim, hl lipgloss.Style) string {
	var lines []string
	lines = append(lines, bold.Render("  确认配置"))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  Provider:  %s", hl.Render(m.result.Provider)))
	// API Key 脱敏显示
	maskedKey := m.result.APIKey
	if len(maskedKey) > 8 {
		maskedKey = maskedKey[:4] + "****" + maskedKey[len(maskedKey)-4:]
	}
	lines = append(lines, fmt.Sprintf("  API Key:   %s", dim.Render(maskedKey)))
	if m.result.BaseURL != "" {
		lines = append(lines, fmt.Sprintf("  Base URL:  %s", dim.Render(m.result.BaseURL)))
	}
	lines = append(lines, fmt.Sprintf("  Model:     %s", hl.Render(m.result.Model)))
	lines = append(lines, "")
	lines = append(lines, bold.Render("  保存？")+"  "+dim.Render("y 确认  n 重新选择  Esc 退出"))
	return strings.Join(lines, "\n")
}
```

- [ ] **Step 2: 编译验证**

Run: `go build ./internal/setup/`
Expected: 编译通过

- [ ] **Step 3: Commit**

```bash
git add internal/setup/setup.go
git commit -m "feat(setup): Setup TUI 引导界面 — 多步选择 Provider/Key/Model"
```

---

### Task 5: main.go 接入 Setup TUI

**Files:**
- Modify: `main.go`

- [ ] **Step 1: 在 main.go 中检测无 API Key 时启动 Setup**

在 `main.go` 的 `cfg.APIKey == ""` 错误处理块替换为 Setup TUI 启动逻辑：

```go
// 替换原来的 fmt.Fprintln 错误退出逻辑
if cfg.APIKey == "" {
	// 无 API Key：启动 Setup 引导
	setupModel := setup.New(XinCodeDir())
	setupProg := tea.NewProgram(setupModel, tea.WithAltScreen())
	finalModel, err := setupProg.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Setup 错误: %s\n", err)
		os.Exit(1)
	}
	result, setupErr := finalModel.(setup.Model).GetResult()
	if setupErr != nil {
		fmt.Fprintf(os.Stderr, "配置失败: %s\n", setupErr)
		os.Exit(1)
	}
	if result.APIKey == "" {
		fmt.Fprintln(os.Stderr, "未完成配置，退出。")
		os.Exit(0)
	}
	// 重新加载配置（刚写入的 settings.json + credentials.json）
	cfg = LoadConfig()
	if cfg.APIKey == "" {
		fmt.Fprintln(os.Stderr, "配置已保存但加载失败，请检查 ~/.xincode/ 目录")
		os.Exit(1)
	}
	authSource = "config"
}
```

需要在 main.go 的 import 中添加 `"github.com/xincode-ai/xin-code/internal/setup"`。

- [ ] **Step 2: 编译并手动验证**

Run: `go build -o xin-code . && echo "build ok"`
Expected: 编译通过

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "feat: 首次启动无 Key 时自动进入 Setup 引导"
```

---

### Task 6: /login 和 /logout 真实实现

**Files:**
- Modify: `main.go`（注入回调）
- Modify: `internal/slash/handler.go`（接入真实逻辑）

- [ ] **Step 1: 在 main.go 中注入 /login 回调**

在 `main.go` 的 `app.OnSkillsList = ...` 附近添加：

```go
app.OnLogin = func() string {
	// 启动 Setup TUI（在当前终端内以子程序运行）
	// 注意：这里无法直接启动 tea.Program（已有一个在运行）
	// 简化方案：提示用户退出后重新启动
	return "请退出后运行 xin-code 重新配置，或设置环境变量：\n" +
		"  export ANTHROPIC_API_KEY=your-key\n" +
		"  export OPENAI_API_KEY=your-key\n\n" +
		"当前凭据路径: " + filepath.Join(XinCodeDir(), "auth", "credentials.json")
}
app.OnLogout = func() string {
	if err := auth.ClearAPIKey(XinCodeDir()); err != nil {
		return fmt.Sprintf("清除凭据失败: %s", err)
	}
	return "已清除保存的 API Key。\n下次启动将重新进入配置向导。"
}
```

App 结构体需要添加 `OnLogout` 字段。在 `internal/tui/app.go` 的 App struct 中添加：

```go
OnLogout func() string
```

同时在 `internal/slash/handler.go` 的 `Context` struct 中添加：

```go
OnLogout func() string // /logout
```

- [ ] **Step 2: 更新 slash handler 的 /login 和 /logout**

`/login` handler 改为：

```go
func cmdLogin() *Command {
	return &Command{
		Name:        "/login",
		Description: "配置 API Key",
		Handler: func(args []string, ctx *Context) Result {
			if ctx.OnLogin != nil {
				msg := ctx.OnLogin()
				return Result{Type: ResultDisplay, Content: msg}
			}
			return Result{Type: ResultDisplay, Content: "请设置环境变量: export XINCODE_API_KEY=your-key"}
		},
	}
}
```

`/logout` handler 改为：

```go
func cmdLogout() *Command {
	return &Command{
		Name:        "/logout",
		Description: "清除保存的认证信息",
		Handler: func(args []string, ctx *Context) Result {
			if ctx.OnLogout != nil {
				msg := ctx.OnLogout()
				return Result{Type: ResultDisplay, Content: msg}
			}
			return Result{Type: ResultDisplay, Content: "认证信息管理未就绪"}
		},
	}
}
```

- [ ] **Step 3: 更新 handleSlashCommand 传递 OnLogout**

在 `internal/tui/app.go` 的 `handleSlashCommand` 方法中，Context 初始化添加：

```go
OnLogout: a.OnLogout,
```

- [ ] **Step 4: 编译验证**

Run: `go build -o xin-code . && echo "build ok"`
Expected: 编译通过

- [ ] **Step 5: Commit**

```bash
git add main.go internal/tui/app.go internal/slash/handler.go
git commit -m "feat: /login 和 /logout 接入真实凭证管理"
```

---

### Task 7: /doctor 诊断命令

**Files:**
- Modify: `internal/slash/handler.go`

- [ ] **Step 1: 实现 /doctor 命令**

在 `handler.go` 的 `registerAll` 中注册 `cmdDoctor()`，并添加实现：

```go
// 在 registerAll 的系统类区域添加
h.register(cmdDoctor())
```

```go
func cmdDoctor() *Command {
	return &Command{
		Name:        "/doctor",
		Description: "环境诊断",
		Handler: func(args []string, ctx *Context) Result {
			var sb strings.Builder
			sb.WriteString("🩺 环境诊断\n\n")

			// 1. API Key
			apiKeyStatus := "✓"
			apiKeyDetail := "已配置"
			if ctx.CostString == "" || ctx.Model == "" {
				apiKeyStatus = "✗"
				apiKeyDetail = "未检测到 API Key"
			}
			sb.WriteString(fmt.Sprintf("  %s API Key:      %s\n", apiKeyStatus, apiKeyDetail))

			// 2. Provider
			sb.WriteString(fmt.Sprintf("  ✓ Provider:     %s\n", ctx.Provider))

			// 3. Model
			sb.WriteString(fmt.Sprintf("  ✓ Model:        %s\n", ctx.Model))

			// 4. 配置路径
			homeDir, _ := os.UserHomeDir()
			configDir := filepath.Join(homeDir, ".xincode")
			settingsPath := filepath.Join(configDir, "settings.json")
			credPath := filepath.Join(configDir, "auth", "credentials.json")
			checkFile := func(path string) string {
				if _, err := os.Stat(path); err == nil {
					return "✓ 存在"
				}
				return "✗ 不存在"
			}
			sb.WriteString(fmt.Sprintf("  %s settings.json:     %s\n", checkFile(settingsPath)[:1], settingsPath))
			sb.WriteString(fmt.Sprintf("  %s credentials.json:  %s\n", checkFile(credPath)[:1], credPath))

			// 5. 工作目录
			sb.WriteString(fmt.Sprintf("  ✓ 工作目录:     %s\n", ctx.WorkDir))

			// 6. Git
			gitStatus := "✗ 非 git 仓库"
			if _, err := os.Stat(filepath.Join(ctx.WorkDir, ".git")); err == nil {
				gitStatus = "✓ git 仓库"
			}
			sb.WriteString(fmt.Sprintf("  %s\n", gitStatus))

			// 7. XINCODE.md
			xincodeStatus := "✗ 未找到"
			if _, err := os.Stat(filepath.Join(ctx.WorkDir, "XINCODE.md")); err == nil {
				xincodeStatus = "✓ 已找到"
			}
			sb.WriteString(fmt.Sprintf("  %s XINCODE.md\n", xincodeStatus[:1]))

			// 8. 上下文用量
			if ctx.MaxContext > 0 {
				pct := float64(ctx.TotalTokens) / float64(ctx.MaxContext) * 100
				ctxStatus := "✓"
				if pct > 80 {
					ctxStatus = "⚠"
				}
				sb.WriteString(fmt.Sprintf("  %s 上下文:       %.1f%% (%d/%d)\n", ctxStatus, pct, ctx.TotalTokens, ctx.MaxContext))
			}

			return Result{Type: ResultDisplay, Content: sb.String()}
		},
	}
}
```

需要在 handler.go import 中添加 `"path/filepath"`。

- [ ] **Step 2: 编译验证**

Run: `go build ./... && echo "ok"`
Expected: 编译通过

- [ ] **Step 3: Commit**

```bash
git add internal/slash/handler.go
git commit -m "feat: /doctor 环境诊断命令"
```

---

### Task 8: /bug 反馈命令

**Files:**
- Modify: `internal/slash/handler.go`

- [ ] **Step 1: 实现 /bug 命令**

在 `registerAll` 中注册 `cmdBug()`：

```go
h.register(cmdBug())
```

```go
func cmdBug() *Command {
	return &Command{
		Name:        "/bug",
		Description: "报告问题",
		Handler: func(args []string, ctx *Context) Result {
			var sb strings.Builder
			sb.WriteString("🐛 报告问题\n\n")
			sb.WriteString("  请在 GitHub 上提交 Issue：\n")
			sb.WriteString("  https://github.com/xincode-ai/xin-code/issues/new\n\n")
			sb.WriteString("  请附上以下信息：\n")
			sb.WriteString(fmt.Sprintf("  版本:     %s\n", ctx.Version))
			sb.WriteString(fmt.Sprintf("  模型:     %s\n", ctx.Model))
			sb.WriteString(fmt.Sprintf("  Provider: %s\n", ctx.Provider))
			sb.WriteString(fmt.Sprintf("  权限模式: %s\n", ctx.PermMode))
			return Result{Type: ResultDisplay, Content: sb.String()}
		},
	}
}
```

- [ ] **Step 2: 编译验证**

Run: `go build ./... && echo "ok"`
Expected: 编译通过

- [ ] **Step 3: Commit**

```bash
git add internal/slash/handler.go
git commit -m "feat: /bug 反馈命令"
```

---

### Task 9: 集成测试

**Files:**
- Modify: `internal/setup/setup_test.go`

- [ ] **Step 1: 添加 Setup TUI 状态流转测试**

```go
func TestSetupModelStepFlow(t *testing.T) {
	m := New(t.TempDir())

	// 初始状态应该是 StepProvider
	if m.step != StepProvider {
		t.Errorf("expected StepProvider, got %d", m.step)
	}

	// 模拟选择第一个 Provider (Anthropic)
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)
	if m.step != StepAPIKey {
		t.Errorf("after provider select, expected StepAPIKey, got %d", m.step)
	}
	if m.result.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %q", m.result.Provider)
	}
}

func TestSetupCustomProviderRequiresBaseURL(t *testing.T) {
	m := New(t.TempDir())

	// 选择 custom provider（第 4 个，index=3）
	m.providerIdx = 3
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)

	// 输入 API Key
	m.apiKeyInput.SetValue("test-key")
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)

	// custom provider 应该进入 StepBaseURL
	if m.step != StepBaseURL {
		t.Errorf("custom provider should go to StepBaseURL, got %d", m.step)
	}
}
```

- [ ] **Step 2: 运行全部测试**

Run: `go test ./internal/setup/ -v`
Expected: 所有测试通过

- [ ] **Step 3: 运行全项目编译 + 已有测试**

Run: `go build ./... && go test ./... 2>&1 | tail -20`
Expected: 编译通过，所有测试通过

- [ ] **Step 4: Commit**

```bash
git add internal/setup/setup_test.go
git commit -m "test(setup): Setup TUI 状态流转测试"
```

---

### Task 10: 最终验证与清理

- [ ] **Step 1: 全量编译 + 测试**

Run: `go build -o xin-code . && go test ./... -v 2>&1 | tail -30`
Expected: 编译通过，所有测试通过

- [ ] **Step 2: 手动启动验证**

临时移除 API Key 环境变量，运行 `./xin-code`，应进入 Setup 引导界面。

- [ ] **Step 3: 确认 git 状态干净**

Run: `git status`
Expected: 无未提交变更
