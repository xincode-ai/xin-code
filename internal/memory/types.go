// internal/memory/types.go
// 记忆类型定义，对标 CC src/memdir/memoryTypes.ts
package memory

import "time"

// MemoryType 记忆类型
type MemoryType string

const (
	TypeUser      MemoryType = "user"      // 用户角色/偏好/知识背景
	TypeFeedback  MemoryType = "feedback"  // 用户反馈/纠正/确认
	TypeProject   MemoryType = "project"   // 项目进展/目标/截止日期
	TypeReference MemoryType = "reference" // 外部资源指针
)

// MemoryHeader 记忆文件 YAML frontmatter
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

// CC 对标常量
const (
	MaxMemoryFiles = 200   // 单个目录最大记忆文件数（CC: 200）
	MaxIndexLines  = 200   // MEMORY.md 最大行数（CC: 200）
	MaxIndexBytes  = 25000 // MEMORY.md 最大字节数（CC: ~25KB）
)
