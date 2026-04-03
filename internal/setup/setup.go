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
	StepProvider SetupStep = iota // 选择 Provider
	StepAPIKey                    // 输入 API Key
	StepBaseURL                   // 输入自定义 BaseURL（仅 custom provider）
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
	step      SetupStep
	configDir string
	width     int
	height    int

	// 选择态
	providers   []ProviderInfo
	providerIdx int
	modelIdx    int

	// 输入态
	apiKeyInput  textarea.Model
	baseURLInput textarea.Model
	modelInput   textarea.Model // custom provider 手动输入模型名

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

	modelTA := textarea.New()
	modelTA.Placeholder = "模型名称，如 gpt-4.1"
	modelTA.SetHeight(1)
	modelTA.MaxHeight = 1
	modelTA.ShowLineNumbers = false
	modelTA.CharLimit = 128

	return Model{
		step:         StepProvider,
		configDir:    configDir,
		providers:    BuiltinProviders,
		providerIdx:  0,
		modelIdx:     0,
		apiKeyInput:  apiKeyTA,
		baseURLInput: baseURLTA,
		modelInput:   modelTA,
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
		m.modelInput.SetWidth(min(60, m.width-10))
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
	case StepModel:
		if len(m.selectedProvider().Models) == 0 {
			var cmd tea.Cmd
			m.modelInput, cmd = m.modelInput.Update(msg)
			return m, cmd
		}
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
		// custom provider 需要手动输入模型名
		if len(m.selectedProvider().Models) == 0 {
			m.modelInput.Focus()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.baseURLInput, cmd = m.baseURLInput.Update(msg)
	return m, cmd
}

func (m Model) updateModel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	models := m.selectedProvider().Models
	if len(models) == 0 {
		// custom provider：用 textarea 输入模型名
		if msg.Type == tea.KeyEnter && !msg.Alt {
			name := strings.TrimSpace(m.modelInput.Value())
			if name == "" {
				return m, nil
			}
			m.result.Model = name
			m.step = StepConfirm
			return m, nil
		}
		var cmd tea.Cmd
		m.modelInput, cmd = m.modelInput.Update(msg)
		return m, cmd
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
	case "y", "Y":
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

	// 步骤进度（映射：0=Provider, 1=APIKey, 2=Model, 3=Confirm）
	type stepDisplay struct {
		name  string
		match SetupStep
	}
	displaySteps := []stepDisplay{
		{"Provider", StepProvider},
		{"API Key", StepAPIKey},
		{"Model", StepModel},
		{"确认", StepConfirm},
	}
	var progress []string
	for _, ds := range displaySteps {
		if ds.match < m.step {
			progress = append(progress, dim.Render("✓ "+ds.name))
		} else if ds.match == m.step || (ds.match == StepModel && m.step == StepBaseURL) {
			progress = append(progress, highlight.Render("● "+ds.name))
		} else {
			progress = append(progress, dim.Render("○ "+ds.name))
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
		var lines []string
		lines = append(lines, bold.Render("  输入模型名称"))
		lines = append(lines, dim.Render("  自定义端点使用的模型 ID"))
		lines = append(lines, "")
		lines = append(lines, "  "+m.modelInput.View())
		lines = append(lines, "")
		lines = append(lines, dim.Render("  Enter 确认  Esc 取消"))
		return strings.Join(lines, "\n")
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
