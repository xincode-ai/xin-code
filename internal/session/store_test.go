package session

import (
	"os"
	"testing"
)

func TestStoreSaveLoad(t *testing.T) {
	// 使用临时目录
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	sess := NewSession("test-model", "/tmp/project")
	sess.Name = "测试会话"

	// 保存
	if err := store.Save(sess); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 加载
	loaded, err := store.Load(sess.ID)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	if loaded.ID != sess.ID {
		t.Errorf("ID 不匹配: got %s, want %s", loaded.ID, sess.ID)
	}
	if loaded.Model != sess.Model {
		t.Errorf("模型不匹配: got %s, want %s", loaded.Model, sess.Model)
	}
	if loaded.WorkDir != sess.WorkDir {
		t.Errorf("工作目录不匹配: got %s, want %s", loaded.WorkDir, sess.WorkDir)
	}
}

func TestStoreList(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	// 创建多个会话
	sess1 := NewSession("model1", "/project/a")
	sess2 := NewSession("model2", "/project/b")
	sess2.ID = sess2.ID + "-2" // 确保 ID 不同

	_ = store.Save(sess1)
	_ = store.Save(sess2)

	// 列出所有
	entries, err := store.List("")
	if err != nil {
		t.Fatalf("列出失败: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("应有 2 个会话: got %d", len(entries))
	}

	// 按工作目录过滤
	filtered, err := store.List("/project/a")
	if err != nil {
		t.Fatalf("过滤列出失败: %v", err)
	}
	if len(filtered) != 1 {
		t.Errorf("过滤后应有 1 个会话: got %d", len(filtered))
	}
}

func TestStoreDelete(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	sess := NewSession("model", "/tmp")
	_ = store.Save(sess)

	// 删除
	if err := store.Delete(sess.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	// 验证文件不存在
	_, err := store.Load(sess.ID)
	if err == nil {
		t.Error("删除后应无法加载")
	}

	// 验证索引中已移除
	entries, _ := store.List("")
	if len(entries) != 0 {
		t.Errorf("删除后索引应为空: got %d", len(entries))
	}
}

func TestStoreListEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	// sessions 目录不存在时
	entries, err := store.List("")
	if err != nil {
		t.Fatalf("空列表不应报错: %v", err)
	}
	if entries != nil && len(entries) != 0 {
		t.Errorf("空列表应为 nil 或空: got %d", len(entries))
	}

	// 创建 sessions 目录但无文件
	os.MkdirAll(tmpDir+"/sessions", 0755)
	entries, err = store.List("")
	if err != nil {
		t.Fatalf("空目录列表不应报错: %v", err)
	}
}
