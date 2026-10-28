package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hutaomu/hutaomuchat/server/internal/models"
)

// TestRefreshTokenKeepsSessionAlive verifies that an expired access token can
// be exchanged for a new one without asking the user for the password again.
func TestRefreshTokenKeepsSessionAlive(t *testing.T) {
	_, handler := newTestServer(t)
	do(t, handler, http.MethodPost, "/api/setup", "", map[string]any{
		"adminUser": "admin", "adminPass": "admin123",
	})

	status, payload := do(t, handler, http.MethodPost, "/api/auth/register", "", map[string]any{
		"phone": "13800000001", "password": "test1234", "nickname": "小明",
	})
	if status != http.StatusOK {
		t.Fatalf("注册失败: %d", status)
	}
	accessToken, _ := payload["token"].(string)
	refreshToken, _ := payload["refresh_token"].(string)
	if accessToken == "" || refreshToken == "" {
		t.Fatal("注册应同时返回访问令牌与刷新令牌")
	}
	if payload["expires_in"].(float64) <= 0 {
		t.Fatal("注册应返回令牌有效期")
	}

	// 刷新令牌可以换取新的访问令牌
	status, payload = do(t, handler, http.MethodPost, "/api/auth/refresh", "", map[string]any{
		"refresh_token": refreshToken,
	})
	if status != http.StatusOK {
		t.Fatalf("刷新令牌应返回 200，实际: %d (%v)", status, payload)
	}
	newAccess, _ := payload["token"].(string)
	newRefresh, _ := payload["refresh_token"].(string)
	if newAccess == "" {
		t.Fatal("刷新应返回新的访问令牌")
	}
	if newRefresh == "" || newRefresh == refreshToken {
		t.Fatal("刷新令牌应当轮换")
	}

	// 新的访问令牌可以正常访问接口
	status, _ = do(t, handler, http.MethodGet, "/api/auth/me", newAccess, nil)
	if status != http.StatusOK {
		t.Fatalf("刷新后的令牌应可用，实际: %d", status)
	}

	// 访问令牌不能当作刷新令牌使用
	status, _ = do(t, handler, http.MethodPost, "/api/auth/refresh", "", map[string]any{
		"refresh_token": accessToken,
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("访问令牌不应被当作刷新令牌，实际: %d", status)
	}

	// 无效的刷新令牌
	status, _ = do(t, handler, http.MethodPost, "/api/auth/refresh", "", map[string]any{
		"refresh_token": "not-a-token",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("无效刷新令牌应返回 401，实际: %d", status)
	}
}

// TestCleanupKeepsReferencedUploads makes sure that avatars, backgrounds and
// chat pictures survive the cleanup job.
func TestCleanupKeepsReferencedUploads(t *testing.T) {
	server, handler := newTestServer(t)
	do(t, handler, http.MethodPost, "/api/setup", "", map[string]any{
		"adminUser": "admin", "adminPass": "admin123",
	})
	token, userID := registerUser(t, handler, "13800000001", "小明")

	// 造三个文件：头像、聊天图片、以及无人引用的孤儿文件
	avatar := "avatar.png"
	picture := "picture.png"
	orphan := "orphan.png"
	for _, name := range []string{avatar, picture, orphan} {
		path := filepath.Join(server.UploadsDir, name)
		if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
			t.Fatalf("创建测试文件失败: %v", err)
		}
	}

	// 把文件时间改成很久以前，模拟超过保留期
	old := time.Now().Add(-30 * 24 * time.Hour)
	for _, name := range []string{avatar, picture, orphan} {
		os.Chtimes(filepath.Join(server.UploadsDir, name), old, old)
	}

	server.Store.UpdateUser(userID, func(u *models.User) {
		u.AvatarURL = "/uploads/" + avatar
		u.BackgroundURL = "/uploads/" + picture
	})
	server.Store.AddSticker(models.Sticker{ID: "s1", UserID: userID, URL: "/uploads/" + picture})

	removed := server.CleanupUploads()
	if removed != 1 {
		t.Fatalf("应只清理 1 个无人引用的文件，实际清理了 %d 个", removed)
	}
	if _, err := os.Stat(filepath.Join(server.UploadsDir, avatar)); err != nil {
		t.Fatal("头像文件不应被清理")
	}
	if _, err := os.Stat(filepath.Join(server.UploadsDir, picture)); err != nil {
		t.Fatal("仍在使用的图片不应被清理")
	}
	if _, err := os.Stat(filepath.Join(server.UploadsDir, orphan)); err == nil {
		t.Fatal("无人引用的文件应当被清理")
	}

	// 未超过保留期的文件即使无人引用也要保留
	fresh := "fresh.png"
	os.WriteFile(filepath.Join(server.UploadsDir, fresh), []byte("data"), 0o644)
	server.CleanupUploads()
	if _, err := os.Stat(filepath.Join(server.UploadsDir, fresh)); err != nil {
		t.Fatal("未超过保留期的文件不应被清理")
	}
	_ = token
}

// TestExtractUploadNames checks the parsing of upload references.
func TestExtractUploadNames(t *testing.T) {
	cases := []struct {
		input    string
		expected []string
	}{
		{"/uploads/abc.png", []string{"abc.png"}},
		{"", nil},
		{"https://example.com/x.png", nil},
		{`{"url":"/uploads/abc.png"}`, []string{"abc.png"}},
		{"/uploads/a.png /uploads/b.jpg", []string{"a.png", "b.jpg"}},
	}
	for _, c := range cases {
		got := extractUploadNames(c.input)
		if len(got) != len(c.expected) {
			t.Fatalf("extractUploadNames(%q) = %v，期望 %v", c.input, got, c.expected)
		}
		for i := range got {
			if got[i] != c.expected[i] {
				t.Fatalf("extractUploadNames(%q) = %v，期望 %v", c.input, got, c.expected)
			}
		}
	}
}
