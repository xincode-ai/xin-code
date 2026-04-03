package tui

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// spinnerFrames 帧序列：前进 + 反转拼接，形成来回动画
var spinnerFrames = []string{"·", "✢", "✳", "✶", "✻", "✽", "✻", "✶", "✳", "✢"}

// spinnerVerbs 精选 40 个趣味英文动词，每次 Start 时随机选一个
var spinnerVerbs = []string{
	"Thinking",
	"Brewing",
	"Clauding",
	"Cogitating",
	"Pondering",
	"Musing",
	"Ruminating",
	"Deliberating",
	"Contemplating",
	"Scheming",
	"Synthesizing",
	"Calculating",
	"Reasoning",
	"Analyzing",
	"Extrapolating",
	"Interpolating",
	"Hypothesizing",
	"Conjecturing",
	"Theorizing",
	"Philosophizing",
	"Daydreaming",
	"Wandering",
	"Exploring",
	"Investigating",
	"Excavating",
	"Assembling",
	"Orchestrating",
	"Conjuring",
	"Manifesting",
	"Channeling",
	"Summoning",
	"Weaving",
	"Crafting",
	"Forging",
	"Distilling",
	"Refining",
	"Percolating",
	"Simmering",
	"Marinating",
	"Vibing",
}

// MsgSpinnerTick 是驱动 Spinner 帧推进的 Tick 消息
type MsgSpinnerTick struct{}

// SpinnerTickCmd 返回一个 80ms 间隔的 Tick 命令
func SpinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return MsgSpinnerTick{}
	})
}

// SubAgentStatus 子 agent 状态
type SubAgentStatus struct {
	ID          string
	Description string
	Done        bool
}

// SpinnerState 保存 Spinner 的运行状态
type SpinnerState struct {
	frame     int
	verb      string
	startTime time.Time
	active    bool
	subAgents []SubAgentStatus
}

// NewSpinnerState 初始化一个空闲状态的 Spinner
func NewSpinnerState() SpinnerState {
	return SpinnerState{}
}

// Start 开始计时并随机选取动词
func (s *SpinnerState) Start() {
	s.active = true
	s.frame = 0
	s.startTime = time.Now()
	s.verb = spinnerVerbs[rand.Intn(len(spinnerVerbs))]
}

// Stop 停止 Spinner
func (s *SpinnerState) Stop() {
	s.active = false
}

// Tick 推进一帧
func (s *SpinnerState) Tick() {
	if s.active {
		s.frame = (s.frame + 1) % len(spinnerFrames)
	}
}

// View 渲染 Spinner 行，格式：`  [glyph] [verb]… [elapsed]`
// glyph 使用品牌橙色，elapsed 使用 dim 色
// 如果有子 agent，在下方渲染树形列表
func (s *SpinnerState) View() string {
	if !s.active {
		return ""
	}
	glyph := StyleSpinner.Render(spinnerFrames[s.frame])
	elapsed := StyleDim.Render(formatElapsed(time.Since(s.startTime)))
	mainLine := fmt.Sprintf("%s %s… %s", glyph, s.verb, elapsed)

	if len(s.subAgents) == 0 {
		return mainLine
	}

	// 渲染子 agent 树形列表
	var lines []string
	lines = append(lines, mainLine)

	for i, sa := range s.subAgents {
		isLast := i == len(s.subAgents)-1
		branch := "├─"
		if isLast {
			branch = "└─"
		}

		if sa.Done {
			// 已完成：绿色 ✓ + dim 描述
			marker := lipgloss.NewStyle().Foreground(ColorSuccess).Render("✓")
			desc := StyleDim.Render(sa.Description)
			lines = append(lines, fmt.Sprintf("  %s %s %s", branch, marker, desc))
		} else {
			// 运行中：品牌色 ⏺ + 描述（交替闪烁效果通过 frame 奇偶实现）
			var marker string
			if s.frame%2 == 0 {
				marker = StyleSpinner.Render(BlackCircle())
			} else {
				marker = lipgloss.NewStyle().Foreground(ColorBrandDim).Render(BlackCircle())
			}
			desc := lipgloss.NewStyle().Foreground(ColorBrand).Render(sa.Description)
			lines = append(lines, fmt.Sprintf("  %s %s %s", branch, marker, desc))
		}
	}

	return strings.Join(lines, "\n")
}

// AddSubAgent 注册一个正在运行的子 agent
func (s *SpinnerState) AddSubAgent(id, description string) {
	s.subAgents = append(s.subAgents, SubAgentStatus{
		ID:          id,
		Description: description,
		Done:        false,
	})
}

// CompleteSubAgent 标记指定子 agent 为已完成
func (s *SpinnerState) CompleteSubAgent(id string) {
	for i := range s.subAgents {
		if s.subAgents[i].ID == id {
			s.subAgents[i].Done = true
			return
		}
	}
}

// ClearSubAgents 清空所有子 agent 状态
func (s *SpinnerState) ClearSubAgents() {
	s.subAgents = nil
}

// formatElapsed 将 duration 格式化为人类可读字符串：
// 不足 60s 显示 "Ns"，否则显示 "Nm Ns"
func formatElapsed(d time.Duration) string {
	total := int(d.Seconds())
	if total < 60 {
		return fmt.Sprintf("%ds", total)
	}
	m := total / 60
	sec := total % 60
	return fmt.Sprintf("%dm %ds", m, sec)
}
