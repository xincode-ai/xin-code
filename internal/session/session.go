package session

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/xincode-ai/xin-code/internal/provider"
)

// Session 会话
type Session struct {
	ID        string             `json:"id"`
	Name      string             `json:"name,omitempty"`
	Model     string             `json:"model"`
	WorkDir   string             `json:"work_dir"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
	Messages  []provider.Message `json:"messages"`
	Turns     int                `json:"turns"`

	// 费用信息
	TotalInputTokens  int     `json:"total_input_tokens"`
	TotalOutputTokens int     `json:"total_output_tokens"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
}

// NewSession 创建新会话
func NewSession(model, workDir string) *Session {
	now := time.Now()
	return &Session{
		ID:        generateID(now),
		Model:     model,
		WorkDir:   workDir,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  make([]provider.Message, 0),
	}
}

// AddMessage 追加消息
func (s *Session) AddMessage(msg provider.Message) {
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()
	// 用户消息算一轮
	if msg.Role == provider.RoleUser {
		s.Turns++
	}
}

// UpdateCost 更新费用信息
func (s *Session) UpdateCost(inputTokens, outputTokens int, costUSD float64) {
	s.TotalInputTokens += inputTokens
	s.TotalOutputTokens += outputTokens
	s.TotalCostUSD += costUSD
}

// ExportMarkdown 导出为 Markdown
func (s *Session) ExportMarkdown() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Session: %s\n\n", s.ID))
	sb.WriteString(fmt.Sprintf("- **模型**: %s\n", s.Model))
	sb.WriteString(fmt.Sprintf("- **工作目录**: %s\n", s.WorkDir))
	sb.WriteString(fmt.Sprintf("- **创建时间**: %s\n", s.CreatedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("- **轮次**: %d\n", s.Turns))
	sb.WriteString(fmt.Sprintf("- **费用**: $%.4f\n\n", s.TotalCostUSD))
	sb.WriteString("---\n\n")

	for _, msg := range s.Messages {
		switch msg.Role {
		case provider.RoleUser:
			sb.WriteString("## 👤 User\n\n")
			sb.WriteString(msg.TextContent())
			sb.WriteString("\n\n")
		case provider.RoleAssistant:
			sb.WriteString("## 🤖 Assistant\n\n")
			text := msg.TextContent()
			if text != "" {
				sb.WriteString(text)
				sb.WriteString("\n\n")
			}
			// 工具调用
			for _, call := range msg.ToolCalls() {
				sb.WriteString(fmt.Sprintf("**工具调用**: `%s`\n", call.Name))
				sb.WriteString(fmt.Sprintf("```json\n%s\n```\n\n", call.Input))
			}
		}
	}

	return sb.String()
}

// generateID 生成会话 ID（时间戳 + 随机后缀，避免同秒碰撞）
func generateID(t time.Time) string {
	return fmt.Sprintf("%s-%04x", t.Format("20060102-150405"), rand.Intn(0xFFFF))
}
