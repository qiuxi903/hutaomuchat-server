package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hutaomu/hutaomuchat/server/internal/config"
	"github.com/hutaomu/hutaomuchat/server/internal/logging"
	"github.com/hutaomu/hutaomuchat/server/internal/push"
	"github.com/hutaomu/hutaomuchat/server/internal/store"
	"github.com/hutaomu/hutaomuchat/server/internal/ws"
)

// newTestServer builds a server backed by a temporary directory.
func newTestServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	dir := t.TempDir()

	logger, err := logging.New(filepath.Join(dir, "logs"), logging.LevelError)
	if err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	cfg := config.NewManager(filepath.Join(dir, "server.config.json"))
	st := store.New()
	hub := ws.NewHub()
	pushClient := push.New(push.Config{})

	server := New(st, cfg, logger, hub, pushClient,
		filepath.Join(dir, "uploads"),
		filepath.Join(dir, "web"),
		filepath.Join(dir, "data"),
		filepath.Join(dir, "logs"))
	os.MkdirAll(filepath.Join(dir, "uploads"), 0o755)
	os.MkdirAll(filepath.Join(dir, "web"), 0o755)

	return server, server.Routes()
}

// do performs a JSON request against the test server.
func do(t *testing.T, handler http.Handler, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("编码请求失败: %v", err)
		}
		reader = strings.NewReader(string(encoded))
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	var payload map[string]any
	if recorder.Body.Len() > 0 {
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("解析响应失败 (%d): %s", recorder.Code, recorder.Body.String())
		}
	}
	return recorder.Code, payload
}

// registerUser registers an account and returns its token and id.
func registerUser(t *testing.T, handler http.Handler, phone, nickname string) (string, string) {
	t.Helper()
	status, payload := do(t, handler, http.MethodPost, "/api/auth/register", "", map[string]any{
		"phone":    phone,
		"password": "test1234",
		"nickname": nickname,
	})
	if status != http.StatusOK {
		t.Fatalf("注册失败 (%d): %v", status, payload)
	}
	token, _ := payload["token"].(string)
	user, _ := payload["user"].(map[string]any)
	id, _ := user["id"].(string)
	return token, id
}

func TestHealthEndpoint(t *testing.T) {
	_, handler := newTestServer(t)
	status, payload := do(t, handler, http.MethodGet, "/api/health", "", nil)
	if status != http.StatusOK {
		t.Fatalf("健康检查应返回 200，实际: %d", status)
	}
	if payload["status"] != "ok" {
		t.Fatalf("健康检查状态不正确: %v", payload["status"])
	}
}

func TestSetupGuardsTheAPI(t *testing.T) {
	_, handler := newTestServer(t)

	// 未初始化时除 setup 与 health 外的接口都应被拒绝
	status, _ := do(t, handler, http.MethodPost, "/api/auth/register", "", map[string]any{
		"phone": "13800000001", "password": "test1234", "nickname": "小明",
	})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("未初始化时注册应返回 503，实际: %d", status)
	}
	status, _ = do(t, handler, http.MethodGet, "/api/health", "", nil)
	if status != http.StatusOK {
		t.Fatalf("健康检查在未初始化时应可用，实际: %d", status)
	}
}

func TestSetupStoresConfiguration(t *testing.T) {
	server, handler := newTestServer(t)

	status, payload := do(t, handler, http.MethodPost, "/api/setup", "", map[string]any{
		"adminUser":  "root",
		"adminPass":  "secret123",
		"serverName": "测试服务器",
		"serverPort": 3999,
	})
	if status != http.StatusOK {
		t.Fatalf("初始化应返回 200，实际: %d (%v)", status, payload)
	}
	if !server.Config.Initialized() {
		t.Fatal("初始化后应标记为已初始化")
	}
	if server.Config.ServerName() != "测试服务器" {
		t.Fatalf("服务器名称未生效: %s", server.Config.ServerName())
	}

	// 重复初始化应被拒绝
	status, _ = do(t, handler, http.MethodPost, "/api/setup", "", map[string]any{
		"adminUser": "root", "adminPass": "secret123",
	})
	if status != http.StatusForbidden {
		t.Fatalf("重复初始化应返回 403，实际: %d", status)
	}
}

func TestAdminLoginAndProtectedRoutes(t *testing.T) {
	_, handler := newTestServer(t)
	do(t, handler, http.MethodPost, "/api/setup", "", map[string]any{
		"adminUser": "admin", "adminPass": "admin123",
	})

	status, payload := do(t, handler, http.MethodPost, "/api/admin/login", "", map[string]any{
		"username": "admin", "password": "admin123",
	})
	if status != http.StatusOK {
		t.Fatalf("管理员登录应返回 200，实际: %d", status)
	}
	token, _ := payload["token"].(string)
	if token == "" {
		t.Fatal("管理员登录应返回令牌")
	}

	status, _ = do(t, handler, http.MethodGet, "/api/admin/stats", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("缺少令牌访问管理接口应返回 401，实际: %d", status)
	}
	status, _ = do(t, handler, http.MethodGet, "/api/admin/stats", token, nil)
	if status != http.StatusOK {
		t.Fatalf("携带令牌访问管理接口应返回 200，实际: %d", status)
	}

	// 用户令牌不得访问管理接口
	userToken, _ := registerUser(t, handler, "13800000001", "小明")
	status, _ = do(t, handler, http.MethodGet, "/api/admin/stats", userToken, nil)
	if status != http.StatusForbidden {
		t.Fatalf("普通用户访问管理接口应返回 403，实际: %d", status)
	}
}

func TestRegisterAndLogin(t *testing.T) {
	_, handler := newTestServer(t)
	do(t, handler, http.MethodPost, "/api/setup", "", map[string]any{
		"adminUser": "admin", "adminPass": "admin123",
	})

	token, id := registerUser(t, handler, "13800000001", "小明")
	if token == "" || id == "" {
		t.Fatal("注册应返回令牌与用户 ID")
	}

	status, payload := do(t, handler, http.MethodGet, "/api/auth/me", token, nil)
	if status != http.StatusOK {
		t.Fatalf("获取资料应返回 200，实际: %d", status)
	}
	user, _ := payload["user"].(map[string]any)
	if user["nickname"] != "小明" {
		t.Fatalf("昵称不正确: %v", user["nickname"])
	}
	if _, leaked := user["password_hash"]; leaked {
		t.Fatal("接口不得返回 password_hash 字段")
	}

	// 重复手机号
	status, _ = do(t, handler, http.MethodPost, "/api/auth/register", "", map[string]any{
		"phone": "13800000001", "password": "test1234", "nickname": "重复",
	})
	if status != http.StatusConflict {
		t.Fatalf("重复注册应返回 409，实际: %d", status)
	}

	// 错误密码
	status, _ = do(t, handler, http.MethodPost, "/api/auth/login", "", map[string]any{
		"phone": "13800000001", "password": "wrong-password",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("错误密码应返回 401，实际: %d", status)
	}
}

func TestSearchUsersDoesNotLeakPasswordHash(t *testing.T) {
	_, handler := newTestServer(t)
	do(t, handler, http.MethodPost, "/api/setup", "", map[string]any{
		"adminUser": "admin", "adminPass": "admin123",
	})
	token, _ := registerUser(t, handler, "13800000001", "小明")
	registerUser(t, handler, "13800000002", "小红")

	status, payload := do(t, handler, http.MethodGet, "/api/users/search?q=%E5%B0%8F%E7%BA%A2", token, nil)
	if status != http.StatusOK {
		t.Fatalf("搜索应返回 200，实际: %d", status)
	}
	users, _ := payload["users"].([]any)
	if len(users) != 1 {
		t.Fatalf("应搜索到 1 位用户，实际: %d", len(users))
	}
	entry, _ := users[0].(map[string]any)
	if _, leaked := entry["password_hash"]; leaked {
		t.Fatal("搜索结果不得包含 password_hash")
	}
}

func TestFriendRequestFlow(t *testing.T) {
	_, handler := newTestServer(t)
	do(t, handler, http.MethodPost, "/api/setup", "", map[string]any{
		"adminUser": "admin", "adminPass": "admin123",
	})
	tokenA, _ := registerUser(t, handler, "13800000001", "小明")
	tokenB, idB := registerUser(t, handler, "13800000002", "小红")

	status, payload := do(t, handler, http.MethodPost, "/api/friends/request", tokenA, map[string]any{
		"to_user_id": idB, "message": "你好",
	})
	if status != http.StatusOK {
		t.Fatalf("发送好友申请应返回 200，实际: %d", status)
	}
	requestID, _ := payload["request_id"].(string)

	status, payload = do(t, handler, http.MethodGet, "/api/friends/requests", tokenB, nil)
	if status != http.StatusOK {
		t.Fatalf("获取好友申请应返回 200，实际: %d", status)
	}
	requests, _ := payload["requests"].([]any)
	if len(requests) != 1 {
		t.Fatalf("应收到 1 条申请，实际: %d", len(requests))
	}

	status, _ = do(t, handler, http.MethodPost, "/api/friends/requests/"+requestID+"/respond", tokenB,
		map[string]any{"status": "ACCEPTED"})
	if status != http.StatusOK {
		t.Fatalf("同意好友申请应返回 200，实际: %d", status)
	}

	status, payload = do(t, handler, http.MethodGet, "/api/friends", tokenA, nil)
	if status != http.StatusOK {
		t.Fatalf("获取好友列表应返回 200，实际: %d", status)
	}
	friends, _ := payload["friends"].([]any)
	if len(friends) != 1 {
		t.Fatalf("好友列表应有 1 人，实际: %d", len(friends))
	}

	// 删除好友
	status, _ = do(t, handler, http.MethodDelete, "/api/friends/"+idB, tokenA, nil)
	if status != http.StatusOK {
		t.Fatalf("删除好友应返回 200，实际: %d", status)
	}
}

func TestGroupChatAndAnnouncement(t *testing.T) {
	_, handler := newTestServer(t)
	do(t, handler, http.MethodPost, "/api/setup", "", map[string]any{
		"adminUser": "admin", "adminPass": "admin123",
	})
	tokenA, _ := registerUser(t, handler, "13800000001", "小明")
	_, idB := registerUser(t, handler, "13800000002", "小红")

	status, payload := do(t, handler, http.MethodPost, "/api/chats/group", tokenA, map[string]any{
		"name": "测试群", "member_ids": []string{idB},
	})
	if status != http.StatusOK {
		t.Fatalf("创建群聊应返回 200，实际: %d", status)
	}
	chat, _ := payload["chat"].(map[string]any)
	chatID, _ := chat["id"].(string)
	if chat["group_uid"] != "htmq-001" {
		t.Fatalf("群号不正确: %v", chat["group_uid"])
	}

	// 非成员不能发布公告
	status, _ = do(t, handler, http.MethodPost, "/api/chats/"+chatID+"/announcement", tokenA,
		map[string]any{"content": "欢迎"})
	if status != http.StatusOK {
		t.Fatalf("群主发布公告应返回 200，实际: %d", status)
	}

	status, payload = do(t, handler, http.MethodGet, "/api/chats/"+chatID+"/announcements", tokenA, nil)
	if status != http.StatusOK {
		t.Fatalf("获取公告应返回 200，实际: %d", status)
	}
	announcements, _ := payload["announcements"].([]any)
	if len(announcements) != 1 {
		t.Fatalf("应有 1 条公告，实际: %d", len(announcements))
	}
}

func TestMomentsImagesAreJSONString(t *testing.T) {
	_, handler := newTestServer(t)
	do(t, handler, http.MethodPost, "/api/setup", "", map[string]any{
		"adminUser": "admin", "adminPass": "admin123",
	})
	token, _ := registerUser(t, handler, "13800000001", "小明")

	status, payload := do(t, handler, http.MethodPost, "/api/moments", token, map[string]any{
		"content": "测试动态", "images": []string{"/uploads/a.png"},
	})
	if status != http.StatusOK {
		t.Fatalf("发布动态应返回 200，实际: %d", status)
	}
	moment, _ := payload["moment"].(map[string]any)
	images, ok := moment["images"].(string)
	if !ok {
		t.Fatalf("images 字段应为 JSON 字符串，实际类型: %T", moment["images"])
	}
	if !strings.Contains(images, "/uploads/a.png") {
		t.Fatalf("images 内容不正确: %s", images)
	}
}

func TestNonMemberCannotReadMessages(t *testing.T) {
	_, handler := newTestServer(t)
	do(t, handler, http.MethodPost, "/api/setup", "", map[string]any{
		"adminUser": "admin", "adminPass": "admin123",
	})
	tokenA, _ := registerUser(t, handler, "13800000001", "小明")
	tokenC, _ := registerUser(t, handler, "13800000003", "小刚")

	status, payload := do(t, handler, http.MethodPost, "/api/chats/group", tokenA, map[string]any{
		"name": "私密群", "member_ids": []string{},
	})
	if status != http.StatusOK {
		t.Fatalf("创建群聊应返回 200，实际: %d", status)
	}
	chat, _ := payload["chat"].(map[string]any)
	chatID, _ := chat["id"].(string)

	status, _ = do(t, handler, http.MethodGet, "/api/messages/"+chatID, tokenC, nil)
	if status != http.StatusForbidden {
		t.Fatalf("非成员读取消息应返回 403，实际: %d", status)
	}
}

func TestBannedUserIsRejected(t *testing.T) {
	server, handler := newTestServer(t)
	do(t, handler, http.MethodPost, "/api/setup", "", map[string]any{
		"adminUser": "admin", "adminPass": "admin123",
	})
	token, id := registerUser(t, handler, "13800000001", "小明")

	status, payload := do(t, handler, http.MethodPost, "/api/admin/login", "", map[string]any{
		"username": "admin", "password": "admin123",
	})
	if status != http.StatusOK {
		t.Fatalf("管理员登录失败: %d", status)
	}
	adminToken, _ := payload["token"].(string)

	status, _ = do(t, handler, http.MethodPost, "/api/admin/users/"+id+"/ban", adminToken, nil)
	if status != http.StatusOK {
		t.Fatalf("封禁用户应返回 200，实际: %d", status)
	}

	status, _ = do(t, handler, http.MethodGet, "/api/auth/me", token, nil)
	if status != http.StatusForbidden {
		t.Fatalf("被封禁用户访问接口应返回 403，实际: %d", status)
	}

	status, _ = do(t, handler, http.MethodPost, "/api/admin/users/"+id+"/unban", adminToken, nil)
	if status != http.StatusOK {
		t.Fatalf("解封用户应返回 200，实际: %d", status)
	}
	status, _ = do(t, handler, http.MethodGet, "/api/auth/me", token, nil)
	if status != http.StatusOK {
		t.Fatalf("解封后应可正常访问，实际: %d", status)
	}
	_ = server
}

func TestUnknownAPIRouteReturnsJSON(t *testing.T) {
	_, handler := newTestServer(t)
	do(t, handler, http.MethodPost, "/api/setup", "", map[string]any{
		"adminUser": "admin", "adminPass": "admin123",
	})
	status, payload := do(t, handler, http.MethodGet, "/api/does-not-exist", "", nil)
	if status != http.StatusNotFound {
		t.Fatalf("未知接口应返回 404，实际: %d", status)
	}
	if payload["error"] == nil {
		t.Fatal("未知接口应返回 JSON 错误信息")
	}
}

func TestSetupPageIsServedWhenNotInitialized(t *testing.T) {
	server, handler := newTestServer(t)
	// 模拟部署目录中存在初始化页面
	os.WriteFile(filepath.Join(server.WebDir, "setup.html"), []byte("<html>setup</html>"), 0o644)

	request := httptest.NewRequest(http.MethodGet, "/admin/setup", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("初始化页面应返回 200，实际: %d", recorder.Code)
	}
}

func TestAdminStatsAndUserList(t *testing.T) {
	_, handler := newTestServer(t)
	do(t, handler, http.MethodPost, "/api/setup", "", map[string]any{
		"adminUser": "admin", "adminPass": "admin123",
	})
	registerUser(t, handler, "13800000001", "小明")
	registerUser(t, handler, "13800000002", "小红")

	status, payload := do(t, handler, http.MethodPost, "/api/admin/login", "", map[string]any{
		"username": "admin", "password": "admin123",
	})
	adminToken, _ := payload["token"].(string)

	status, payload = do(t, handler, http.MethodGet, "/api/admin/stats", adminToken, nil)
	if status != http.StatusOK {
		t.Fatalf("统计接口应返回 200，实际: %d", status)
	}
	if payload["totalUsers"].(float64) != 2 {
		t.Fatalf("用户数量不正确: %v", payload["totalUsers"])
	}
	if len(payload["dailyMessages"].([]any)) != 14 {
		t.Fatal("消息趋势应包含 14 天数据")
	}

	status, payload = do(t, handler, http.MethodGet, "/api/admin/users?page=1&pageSize=10", adminToken, nil)
	if status != http.StatusOK {
		t.Fatalf("用户列表应返回 200，实际: %d", status)
	}
	if payload["total"].(float64) != 2 {
		t.Fatalf("用户总数不正确: %v", payload["total"])
	}
	users, _ := payload["users"].([]any)
	entry, _ := users[0].(map[string]any)
	if _, leaked := entry["password_hash"]; leaked {
		t.Fatal("管理后台用户列表不得包含 password_hash")
	}
}

func TestUploadRejectsDisallowedType(t *testing.T) {
	_, handler := newTestServer(t)
	do(t, handler, http.MethodPost, "/api/setup", "", map[string]any{
		"adminUser": "admin", "adminPass": "admin123",
	})
	token, _ := registerUser(t, handler, "13800000001", "小明")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "payload.sh")
	if err != nil {
		t.Fatalf("构造上传请求失败: %v", err)
	}
	part.Write([]byte("#!/bin/sh\necho hacked\n"))
	writer.Close()

	request := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code == http.StatusOK {
		t.Fatal("可执行文件应被上传接口拒绝")
	}
}
