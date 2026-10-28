package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hutaomu/hutaomuchat/server/internal/models"
)

// TestSnapshotSurvivesCorruption makes sure a damaged snapshot file falls back
// to the backup copy instead of emptying the database.
//
// A truncated data file was the reason why accounts (and therefore the login
// state of the clients) disappeared in the previous implementation.
func TestSnapshotSurvivesCorruption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")

	s := New()
	s.CreateUser(&models.User{ID: "u1", UID: "htm-001", Phone: "13800000001", Nickname: "小明"})
	persister := NewPersister(path)
	if err := persister.Save(s); err != nil {
		t.Fatalf("保存快照失败: %v", err)
	}
	// 第二次保存会产生备份副本
	s.CreateUser(&models.User{ID: "u2", UID: "htm-002", Phone: "13800000002", Nickname: "小红"})
	if err := persister.Save(s); err != nil {
		t.Fatalf("保存快照失败: %v", err)
	}
	if _, err := os.Stat(persister.BackupPath()); err != nil {
		t.Fatalf("应当存在备份快照: %v", err)
	}

	// 把主文件损坏成非法 JSON
	if err := os.WriteFile(path, []byte(`{"users": [ truncated`), 0o600); err != nil {
		t.Fatalf("写入损坏文件失败: %v", err)
	}

	restored := New()
	loaded, err := persister.Load(restored)
	if err != nil {
		t.Fatalf("损坏时应回退到备份快照，而不是报错: %v", err)
	}
	if !loaded {
		t.Fatal("应当从备份快照恢复数据")
	}
	if restored.FindUserByID("u1") == nil {
		t.Fatal("用户数据应当从备份中恢复")
	}
}

// TestSnapshotBackupIsAlwaysPresent verifies that a backup exists even after
// the very first save.
func TestSnapshotBackupIsAlwaysPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")

	s := New()
	s.CreateUser(&models.User{ID: "u1", UID: "htm-001", Phone: "13800000001"})
	persister := NewPersister(path)
	if err := persister.Save(s); err != nil {
		t.Fatalf("保存快照失败: %v", err)
	}
	if _, err := os.Stat(persister.BackupPath()); err != nil {
		t.Fatalf("首次保存后也应存在备份: %v", err)
	}
}

// TestSnapshotIsWrittenAtomically verifies that no partially written file is
// left behind.
func TestSnapshotIsWrittenAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")

	s := New()
	s.CreateUser(&models.User{ID: "u1", UID: "htm-001", Phone: "13800000001"})
	persister := NewPersister(path)
	if err := persister.Save(s); err != nil {
		t.Fatalf("保存快照失败: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Fatal("临时文件不应残留在磁盘上")
	}
}
