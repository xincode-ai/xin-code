package tui

import (
	"fmt"
	"math/rand"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

// SpinnerState 保存 Spinner 的运行状态
type SpinnerState struct {
	frame     int
	verb      string
	startTime time.Time
	active    bool
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
func (s *SpinnerState) View() string {
	if !s.active {
		return ""
	}
	glyph := StyleSpinner.Render(spinnerFrames[s.frame])
	elapsed := StyleDim.Render(formatElapsed(time.Since(s.startTime)))
	return fmt.Sprintf("%s %s… %s", glyph, s.verb, elapsed)
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
