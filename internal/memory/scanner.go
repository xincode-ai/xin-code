// internal/memory/scanner.go
// 记忆文件扫描和加载，对标 CC src/memdir/memoryScan.ts
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ScanMemoryDir 扫描记忆目录，返回所有记忆条目
// CC 参考：scanMemoryFiles() — 递归读取，按 mtime 排序，cap at 200
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

		header, body := ParseFrontmatter(string(data))
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

// LoadIndex 读取 MEMORY.md 索引内容，按 CC 规则截断
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

// ParseFrontmatter 解析 YAML frontmatter 和 body
func ParseFrontmatter(content string) (MemoryHeader, string) {
	var header MemoryHeader

	if !strings.HasPrefix(content, "---\n") {
		return header, content
	}

	endIdx := strings.Index(content[4:], "\n---\n")
	if endIdx == -1 {
		// 可能 frontmatter 后面直接到文件末尾
		endIdx = strings.Index(content[4:], "\n---")
		if endIdx == -1 {
			return header, content
		}
	}

	frontmatter := content[4 : 4+endIdx]
	bodyStart := 4 + endIdx + 4 // skip "\n---\n"
	if bodyStart > len(content) {
		bodyStart = len(content)
	}
	body := strings.TrimSpace(content[bodyStart:])

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

	if indexContent != "" {
		sb.WriteString(indexContent)
		sb.WriteString("\n\n")
	}

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

	return sb.String()
}

// GetMemoryDir 返回项目的记忆目录路径
// CC 格式：~/.xincode/projects/{sanitized-cwd}/memory/
// 路径安全化：非字母数字字符替换为 -，长路径截断 + 哈希后缀
func GetMemoryDir(homeDir, projectDir string) string {
	sanitized := sanitizePath(projectDir)
	return filepath.Join(homeDir, ".xincode", "projects", sanitized, "memory")
}

// sanitizePath 将任意路径转为安全目录名
// CC 参考：utils/sessionStoragePortable.ts — sanitizePath
func sanitizePath(path string) string {
	// 替换所有非字母数字字符为 -
	var sb strings.Builder
	prevDash := false
	for _, r := range path {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			sb.WriteByte('-')
			prevDash = true
		}
	}
	result := strings.Trim(sb.String(), "-")

	// 长路径截断（CC: MAX_SANITIZED_LENGTH = 80）
	const maxLen = 80
	if len(result) > maxLen {
		// 用前 maxLen 字符 + 简单哈希避免冲突
		hash := uint32(0)
		for _, b := range path {
			hash = hash*31 + uint32(b)
		}
		result = result[:maxLen-9] + fmt.Sprintf("-%08x", hash)
	}

	return result
}

// WriteMemory 写入一条记忆到文件
func WriteMemory(dir, filename string, entry MemoryEntry) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建记忆目录失败: %w", err)
	}

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
		line := fmt.Sprintf("- [%s](%s) — %s\n", m.Name, filename, desc)
		sb.WriteString(line)
	}

	indexPath := filepath.Join(dir, "MEMORY.md")
	return os.WriteFile(indexPath, []byte(sb.String()), 0644)
}
