package context

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MemoryType 配置文件来源层级
// CC 参考：src/utils/claudemd.ts — MemoryType
type MemoryType string

const (
	MemoryTypeManaged MemoryType = "managed"  // /etc/xincode/XINCODE.md（系统级）
	MemoryTypeUser    MemoryType = "user"     // ~/.xincode/XINCODE.md（用户级）
	MemoryTypeProject MemoryType = "project"  // ./XINCODE.md, .xincode/XINCODE.md, .xincode/rules/*.md
	MemoryTypeLocal   MemoryType = "local"    // ./XINCODE.local.md（不提交 git 的个人配置）
	MemoryTypeAutoMem MemoryType = "automem"  // ~/.xincode/projects/{hash}/memory/
)

// MemoryFileInfo 表示一个已发现的配置/记忆文件
type MemoryFileInfo struct {
	Path    string     // 绝对路径
	Type    MemoryType // 来源层级
	Content string     // 文件内容（含 @include 展开后）
}

// 最大 @include 深度，防止循环引用（CC: 5）
const maxIncludeDepth = 5

// 单个文件最大字符数（CC: 40000）
const maxFileChars = 40000

// includeRegex 匹配 @path、@./path、@~/path 格式的引用指令
var includeRegex = regexp.MustCompile(`(?m)^@(.+)$`)

// DiscoverMemoryFiles 按优先级发现所有配置文件
// 加载顺序（低 → 高优先级）：managed → user → project → rules → local
// CC 参考：src/utils/claudemd.ts — getClaudeMds()
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

	// 展开 @include 指令（带循环引用保护）
	absPath, _ := filepath.Abs(path)
	visited := map[string]bool{absPath: true}
	content = processIncludes(content, filepath.Dir(path), 0, visited)

	return &MemoryFileInfo{
		Path:    path,
		Type:    memType,
		Content: content,
	}
}

// processIncludes 递归展开 @include 指令（带循环引用保护）
// 支持格式：@/absolute/path、@./relative/path、@~/home/path
// CC 参考：processMemoryFile() 使用 processedPaths: Set<string> 做路径去重
func processIncludes(content, baseDir string, depth int, visited map[string]bool) string {
	if depth >= maxIncludeDepth {
		return content
	}

	return includeRegex.ReplaceAllStringFunc(content, func(match string) string {
		refPath := strings.TrimSpace(match[1:]) // 去掉 @ 前缀

		// 路径校验：必须包含路径特征字符，排除 @username 等非文件引用
		if !strings.ContainsAny(refPath, "/\\.~") {
			return match // 不像路径，保留原文
		}

		// 解析路径
		var resolvedPath string
		switch {
		case strings.HasPrefix(refPath, "~/"):
			home, _ := os.UserHomeDir()
			resolvedPath = filepath.Join(home, refPath[2:])
		case strings.HasPrefix(refPath, "./"):
			resolvedPath = filepath.Join(baseDir, refPath)
		case filepath.IsAbs(refPath):
			resolvedPath = refPath
		default:
			resolvedPath = filepath.Join(baseDir, refPath)
		}

		// 循环引用保护：已访问过的路径不再展开
		absResolved, _ := filepath.Abs(resolvedPath)
		if visited[absResolved] {
			return "" // 跳过循环引用
		}

		data, err := os.ReadFile(absResolved)
		if err != nil {
			return match // 文件不存在，保留原文
		}

		// 标记为已访问
		visited[absResolved] = true

		included := string(data)
		if len(included) > maxFileChars {
			included = included[:maxFileChars]
		}

		// 递归展开
		return processIncludes(included, filepath.Dir(absResolved), depth+1, visited)
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
		label := typeLabels[f.Type]
		sb.WriteString("Contents of ")
		sb.WriteString(f.Path)
		sb.WriteString(" (")
		sb.WriteString(label)
		sb.WriteString("):\n\n")
		sb.WriteString(f.Content)
		sb.WriteString("\n\n")
	}

	return sb.String()
}
