package store

import (
	"testing"

	"github.com/hutaomu/hutaomuchat/server/internal/models"
)

func newTestStore() *Store {
	return New()
}

func TestCreateAndFindUser(t *testing.T) {
	s := newTestStore()
	user := &models.User{
		ID:                 "u1",
		UID:                "htm-001",
		Phone:              "13800000001",
		Nickname:           "小明",
		FriendVerification: true,
		PhoneSearchable:    true,
		Recommendable:      true,
		CreatedAt:          models.Now(),
	}
	s.CreateUser(user)

	if got := s.FindUserByID("u1"); got == nil || got.Nickname != "小明" {
		t.Fatal("按 ID 查找用户失败")
	}
	if got := s.FindUserByPhone("13800000001"); got == nil || got.ID != "u1" {
		t.Fatal("按手机号查找用户失败")
	}
	if got := s.FindUserByUID("htm-001"); got == nil || got.ID != "u1" {
		t.Fatal("按 UID 查找用户失败")
	}
	if got := s.FindUserByPhone("19999999999"); got != nil {
		t.Fatal("不存在的手机号应返回 nil")
	}
}

func TestNextUIDIsMonotonic(t *testing.T) {
	s := newTestStore()
	if got := s.NextUID(); got != "htm-001" {
		t.Fatalf("首个 UID 应为 htm-001，实际: %s", got)
	}
	if got := s.NextUID(); got != "htm-002" {
		t.Fatalf("第二个 UID 应为 htm-002，实际: %s", got)
	}
}

func TestFriendshipIsBidirectional(t *testing.T) {
	s := newTestStore()
	s.AddFriendship("a", "b")

	if !s.AreFriends("a", "b") {
		t.Fatal("a 与 b 应为好友")
	}
	if !s.AreFriends("b", "a") {
		t.Fatal("好友关系应为双向")
	}
	if s.AreFriends("a", "a") {
		t.Fatal("不应与自己成为好友")
	}
	if len(s.FriendIDs("a")) != 1 {
		t.Fatalf("a 的好友数量应为 1，实际: %d", len(s.FriendIDs("a")))
	}

	s.RemoveFriendship("a", "b")
	if s.AreFriends("a", "b") || s.AreFriends("b", "a") {
		t.Fatal("解除好友后不应仍为好友")
	}
}

func TestAddFriendshipIsIdempotent(t *testing.T) {
	s := newTestStore()
	s.AddFriendship("a", "b")
	s.AddFriendship("a", "b")
	if len(s.FriendIDs("a")) != 1 {
		t.Fatalf("重复添加好友不应产生重复记录，实际: %d", len(s.FriendIDs("a")))
	}
}

func TestChatMembership(t *testing.T) {
	s := newTestStore()
	chat := &models.Chat{ID: "c1", Type: models.ChatGroup, Name: "测试群"}
	s.CreateChat(chat)
	s.AddChatMember("c1", "owner", models.RoleOwner)
	s.AddChatMember("c1", "member", models.RoleMember)

	if !s.IsChatMember("c1", "owner") {
		t.Fatal("群主应为群成员")
	}
	if s.ChatMemberRole("c1", "owner") != models.RoleOwner {
		t.Fatal("群主角色记录错误")
	}
	if len(s.ChatMembers("c1")) != 2 {
		t.Fatalf("群成员数量应为 2，实际: %d", len(s.ChatMembers("c1")))
	}

	// 重复添加不应产生重复成员
	s.AddChatMember("c1", "member", models.RoleMember)
	if len(s.ChatMembers("c1")) != 2 {
		t.Fatal("重复添加成员不应增加成员数量")
	}

	s.RemoveChatMember("c1", "member")
	if s.IsChatMember("c1", "member") {
		t.Fatal("移除后不应仍是群成员")
	}
}

func TestFindSingleChat(t *testing.T) {
	s := newTestStore()
	chat := &models.Chat{ID: "c1", Type: models.ChatSingle}
	s.CreateChat(chat)
	s.AddChatMember("c1", "a", models.RoleMember)
	s.AddChatMember("c1", "b", models.RoleMember)

	found := s.FindSingleChat("b", "a")
	if found == nil || found.ID != "c1" {
		t.Fatal("应能按任意顺序找到单聊会话")
	}
	if s.FindSingleChat("a", "c") != nil {
		t.Fatal("不相关用户之间不应找到单聊会话")
	}
}

func TestMessageHistoryPerChat(t *testing.T) {
	s := newTestStore()
	for i := 0; i < 5; i++ {
		s.AddMessage(&models.Message{
			ID:        string(rune('a' + i)),
			ChatID:    "c1",
			SenderID:  "u1",
			Timestamp: models.Now(),
		})
	}
	s.AddMessage(&models.Message{ID: "z", ChatID: "c2", SenderID: "u1"})

	if got := len(s.MessagesOf("c1", 0)); got != 5 {
		t.Fatalf("c1 的消息数量应为 5，实际: %d", got)
	}
	if got := len(s.MessagesOf("c1", 2)); got != 2 {
		t.Fatalf("限制数量后应返回 2 条，实际: %d", got)
	}
	if s.CountMessages("c2") != 1 {
		t.Fatal("消息应按会话隔离")
	}
}

func TestMessageHistoryIsTrimmed(t *testing.T) {
	s := newTestStore()
	for i := 0; i < MaxMessagesPerChat+50; i++ {
		s.AddMessage(&models.Message{ID: string(rune(i)), ChatID: "c1"})
	}
	if got := len(s.MessagesOf("c1", 0)); got != MaxMessagesPerChat {
		t.Fatalf("历史消息应被裁剪到 %d 条，实际: %d", MaxMessagesPerChat, got)
	}
}

func TestRemoveMessage(t *testing.T) {
	s := newTestStore()
	s.AddMessage(&models.Message{ID: "m1", ChatID: "c1"})
	if !s.RemoveMessage("m1") {
		t.Fatal("删除已存在的消息应返回 true")
	}
	if s.RemoveMessage("m1") {
		t.Fatal("删除不存在的消息应返回 false")
	}
	if s.FindMessage("m1") != nil {
		t.Fatal("消息应已从存储中移除")
	}
}

func TestMomentLikesToggle(t *testing.T) {
	s := newTestStore()
	moment := &models.Moment{ID: "mo1", UserID: "u1", Scope: models.ScopePublic}
	s.AddMoment(moment)

	if !s.LikeMoment("mo1", "u2") {
		t.Fatal("首次点赞应返回 true")
	}
	if s.LikeMoment("mo1", "u2") {
		t.Fatal("重复点赞应取消点赞并返回 false")
	}
	if s.IsLiked("mo1", "u2") {
		t.Fatal("取消点赞后不应仍处于已点赞状态")
	}
}

func TestPublicAndFriendMomentsVisibility(t *testing.T) {
	s := newTestStore()
	s.AddMoment(&models.Moment{ID: "pub", UserID: "u1", Scope: models.ScopePublic})
	s.AddMoment(&models.Moment{ID: "fr", UserID: "u1", Scope: models.ScopeFriends})
	s.AddMoment(&models.Moment{ID: "other", UserID: "u3", Scope: models.ScopeFriends})
	s.AddFriendship("u1", "u2")

	public := s.PublicMoments()
	if len(public) != 1 || public[0].ID != "pub" {
		t.Fatal("用户圈应只返回公开动态")
	}

	friends := s.FriendMoments("u2")
	if len(friends) != 1 || friends[0].ID != "fr" {
		t.Fatal("好友动态应包含好友的 friends 可见动态")
	}
}

func TestDeleteUserRemovesRelatedData(t *testing.T) {
	s := newTestStore()
	s.CreateUser(&models.User{ID: "u1", UID: "htm-001", Phone: "13800000001"})
	s.CreateUser(&models.User{ID: "u2", UID: "htm-002", Phone: "13800000002"})
	s.AddFriendship("u1", "u2")
	s.AddMessage(&models.Message{ID: "m1", ChatID: "c1", SenderID: "u1"})
	s.AddMoment(&models.Moment{ID: "mo1", UserID: "u1"})
	s.SetPushToken("u1", "cid-1", "android", "")

	if !s.DeleteUser("u1") {
		t.Fatal("删除用户应返回 true")
	}
	if s.FindUserByID("u1") != nil {
		t.Fatal("用户应已删除")
	}
	if s.FindUserByPhone("13800000001") != nil {
		t.Fatal("手机号索引应同步清理")
	}
	if s.AreFriends("u1", "u2") || s.AreFriends("u2", "u1") {
		t.Fatal("好友关系应同步清理")
	}
	if s.FindMoment("mo1") != nil {
		t.Fatal("动态应同步清理")
	}
	if s.GetPushToken("u1") != nil {
		t.Fatal("推送令牌应同步清理")
	}
}

func TestCheckAtAllDailyBudget(t *testing.T) {
	s := newTestStore()
	for i := 0; i < MaxAtAllPerDay; i++ {
		allowed, remaining := s.CheckAtAll("u1")
		if !allowed {
			t.Fatalf("第 %d 次 @所有人 应被允许", i+1)
		}
		if remaining != MaxAtAllPerDay-i-1 {
			t.Fatalf("剩余次数计算错误: %d", remaining)
		}
	}
	allowed, remaining := s.CheckAtAll("u1")
	if allowed || remaining != 0 {
		t.Fatal("超出每日上限后应拒绝")
	}
	// 其他用户拥有独立的额度
	if allowed, _ := s.CheckAtAll("u2"); !allowed {
		t.Fatal("不同用户应有独立的 @所有人 额度")
	}
}

func TestOfflineQueueDrain(t *testing.T) {
	s := newTestStore()
	s.AddToOfflineQueue("u1", &models.Message{ID: "m1", Content: "第一条"})
	s.AddToOfflineQueue("u2", &models.Message{ID: "m2", Content: "第二条"})
	s.AddToOfflineQueue("u1", &models.Message{ID: "m3", Content: "第三条"})

	pending := s.TakePendingMessages("u1")
	if len(pending) != 2 {
		t.Fatalf("u1 应收到 2 条离线消息，实际: %d", len(pending))
	}
	if pending[0].Content != "第一条" || pending[1].Content != "第三条" {
		t.Fatal("离线消息顺序错误")
	}
	if len(s.TakePendingMessages("u1")) != 0 {
		t.Fatal("离线消息取走后不应重复投递")
	}
	if len(s.TakePendingMessages("u2")) != 1 {
		t.Fatal("其他用户的离线消息不应受影响")
	}
}

func TestRecallQueue(t *testing.T) {
	s := newTestStore()
	s.AddToRecallQueue("u1", "c1", "m1")
	s.AddToRecallQueue("u2", "c1", "m2")

	recalls := s.TakePendingRecalls("u1")
	if len(recalls) != 1 || recalls[0].MessageID != "m1" {
		t.Fatal("撤回队列投递错误")
	}
	if len(s.TakePendingRecalls("u1")) != 0 {
		t.Fatal("撤回事件不应重复投递")
	}
}

func TestGlobalChatAlwaysExists(t *testing.T) {
	s := newTestStore()
	chat := s.GetOrCreateGlobalChat()
	if chat == nil || chat.ID != models.GlobalChatID {
		t.Fatal("用户圈应始终存在")
	}
	s.AutoJoinGlobalChat("u1")
	if !s.IsChatMember(models.GlobalChatID, "u1") {
		t.Fatal("新用户应自动加入用户圈")
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/store.json"

	s := New()
	s.CreateUser(&models.User{ID: "u1", UID: "htm-001", Phone: "13800000001", Nickname: "小明"})
	s.CreateUser(&models.User{ID: "u2", UID: "htm-002", Phone: "13800000002", Nickname: "小红"})
	s.AddFriendship("u1", "u2")
	s.AddMessage(&models.Message{ID: "m1", ChatID: "c1", SenderID: "u1", Content: "你好"})
	s.AddMoment(&models.Moment{ID: "mo1", UserID: "u1", Content: "今天不错"})

	persister := NewPersister(path)
	if err := persister.Save(s); err != nil {
		t.Fatalf("保存快照失败: %v", err)
	}

	restored := New()
	loaded, err := persister.Load(restored)
	if err != nil {
		t.Fatalf("读取快照失败: %v", err)
	}
	if !loaded {
		t.Fatal("应成功读取快照")
	}
	if restored.FindUserByID("u1") == nil {
		t.Fatal("用户数据未恢复")
	}
	if !restored.AreFriends("u1", "u2") {
		t.Fatal("好友关系未恢复")
	}
	if len(restored.MessagesOf("c1", 0)) != 1 {
		t.Fatal("消息未恢复")
	}
	if restored.FindMoment("mo1") == nil {
		t.Fatal("动态未恢复")
	}
	if restored.GetOrCreateGlobalChat() == nil {
		t.Fatal("用户圈应始终存在")
	}
}

func TestResetClearsEverything(t *testing.T) {
	s := New()
	s.CreateUser(&models.User{ID: "u1", UID: "htm-001", Phone: "13800000001"})
	s.AddMessage(&models.Message{ID: "m1", ChatID: "c1"})

	s.Reset()
	if s.CountUsers() != 0 {
		t.Fatal("重置后不应残留用户")
	}
	if s.TotalMessages() != 0 {
		t.Fatal("重置后不应残留消息")
	}
	if s.GetOrCreateGlobalChat() == nil {
		t.Fatal("重置后用户圈应重新创建")
	}
}
