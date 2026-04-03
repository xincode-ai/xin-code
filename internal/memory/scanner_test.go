package memory

import (
	"os"
	"path/filepath"
	"strings"
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
		"---\nname: Testing Feedback\ndescription: use real database in tests\ntype: feedback\n---\n\nDon't mock the database.\n**Why:** Prior incident.\n",
	), 0644)

	memories, err := ScanMemoryDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(memories))
	}

	// 验证类型解析正确
	types := map[MemoryType]bool{}
	for _, m := range memories {
		types[m.Type] = true
	}
	if !types[TypeUser] {
		t.Error("should contain user type memory")
	}
	if !types[TypeFeedback] {
		t.Error("should contain feedback type memory")
	}
}

func TestParseFrontmatter(t *testing.T) {
	content := "---\nname: Test Memory\ndescription: test desc\ntype: project\n---\n\nBody content here."
	header, body := ParseFrontmatter(content)

	if header.Name != "Test Memory" {
		t.Errorf("expected name 'Test Memory', got '%s'", header.Name)
	}
	if header.Type != TypeProject {
		t.Errorf("expected project type, got %s", header.Type)
	}
	if body != "Body content here." {
		t.Errorf("unexpected body: '%s'", body)
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := "Just plain text"
	header, body := ParseFrontmatter(content)
	if header.Name != "" {
		t.Error("should have empty header")
	}
	if body != content {
		t.Error("body should be the full content")
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

func TestScanMemoryDir_NonExistent(t *testing.T) {
	memories, err := ScanMemoryDir("/nonexistent/path")
	if err != nil {
		t.Fatal("should not error for nonexistent dir")
	}
	if len(memories) != 0 {
		t.Error("should return empty for nonexistent dir")
	}
}

func TestWriteMemory(t *testing.T) {
	tmpDir := t.TempDir()

	entry := MemoryEntry{
		MemoryHeader: MemoryHeader{
			Name:        "User Role",
			Description: "user is a Go developer",
			Type:        TypeUser,
		},
		Body: "The user is a senior Go developer.",
	}

	err := WriteMemory(tmpDir, "user_role.md", entry)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "user_role.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
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

	WriteMemory(tmpDir, "user_role.md", MemoryEntry{
		MemoryHeader: MemoryHeader{
			Name:        "User Role",
			Description: "senior Go developer",
			Type:        TypeUser,
		},
		Body: "Test body.",
	})

	err := UpdateIndex(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

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

func TestLoadIndex(t *testing.T) {
	tmpDir := t.TempDir()
	indexContent := "- [Test](test.md) — test memory\n"
	os.WriteFile(filepath.Join(tmpDir, "MEMORY.md"), []byte(indexContent), 0644)

	result := LoadIndex(tmpDir)
	if result != indexContent {
		t.Errorf("unexpected index content: %s", result)
	}
}

func TestLoadIndex_NonExistent(t *testing.T) {
	result := LoadIndex("/nonexistent/path")
	if result != "" {
		t.Error("should return empty for nonexistent dir")
	}
}
