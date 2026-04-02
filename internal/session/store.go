package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// IndexEntry 会话索引条目
type IndexEntry struct {
	ID        string    `json:"id"`
	Name      string    `json:"name,omitempty"`
	Model     string    `json:"model"`
	WorkDir   string    `json:"work_dir"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Turns     int       `json:"turns"`
	CostUSD   float64   `json:"cost_usd"`
}

// Store 会话持久化存储
type Store struct {
	dir string // ~/.xincode/sessions/
}

// NewStore 创建存储
func NewStore(xincodeDir string) *Store {
	dir := filepath.Join(xincodeDir, "sessions")
	return &Store{dir: dir}
}

// Save 保存会话到文件（原子写入：先写 .tmp 再 rename）
func (s *Store) Save(sess *Session) error {
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return fmt.Errorf("创建 sessions 目录失败: %w", err)
	}

	// 保存会话数据（原子写入）
	path := filepath.Join(s.dir, sess.ID+".json")
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化会话失败: %w", err)
	}
	if err := atomicWriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入会话文件失败: %w", err)
	}

	// 更新索引
	return s.updateIndex(sess)
}

// Load 加载会话
func (s *Store) Load(id string) (*Session, error) {
	path := filepath.Join(s.dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取会话文件失败: %w", err)
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("解析会话文件失败: %w", err)
	}
	return &sess, nil
}

// List 列出所有会话（按工作目录过滤，时间倒序）
func (s *Store) List(workDir string) ([]IndexEntry, error) {
	indexPath := filepath.Join(s.dir, "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []IndexEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}

	// 按工作目录过滤
	if workDir != "" {
		var filtered []IndexEntry
		for _, e := range entries {
			if e.WorkDir == workDir {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	// 时间倒序
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].UpdatedAt.After(entries[j].UpdatedAt)
	})

	return entries, nil
}

// Delete 删除会话
func (s *Store) Delete(id string) error {
	path := filepath.Join(s.dir, id+".json")
	_ = os.Remove(path)

	// 从索引中移除
	return s.removeFromIndex(id)
}

// updateIndex 更新索引文件
func (s *Store) updateIndex(sess *Session) error {
	indexPath := filepath.Join(s.dir, "index.json")

	var entries []IndexEntry
	if data, err := os.ReadFile(indexPath); err == nil {
		_ = json.Unmarshal(data, &entries)
	}

	// 更新或添加
	found := false
	entry := IndexEntry{
		ID:        sess.ID,
		Name:      sess.Name,
		Model:     sess.Model,
		WorkDir:   sess.WorkDir,
		CreatedAt: sess.CreatedAt,
		UpdatedAt: sess.UpdatedAt,
		Turns:     sess.Turns,
		CostUSD:   sess.TotalCostUSD,
	}

	for i, e := range entries {
		if e.ID == sess.ID {
			entries[i] = entry
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, entry)
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(indexPath, data, 0644)
}

// removeFromIndex 从索引中移除
func (s *Store) removeFromIndex(id string) error {
	indexPath := filepath.Join(s.dir, "index.json")

	var entries []IndexEntry
	if data, err := os.ReadFile(indexPath); err == nil {
		_ = json.Unmarshal(data, &entries)
	}

	var filtered []IndexEntry
	for _, e := range entries {
		if e.ID != id {
			filtered = append(filtered, e)
		}
	}

	data, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(indexPath, data, 0644)
}

// atomicWriteFile 原子写入文件：先写 .tmp 再 rename，防止崩溃时损坏
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, perm); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
