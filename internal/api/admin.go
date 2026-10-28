package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/hutaomu/hutaomuchat/server/internal/config"
	"github.com/hutaomu/hutaomuchat/server/internal/httpx"
	"github.com/hutaomu/hutaomuchat/server/internal/models"
	"github.com/hutaomu/hutaomuchat/server/internal/security"
	"github.com/hutaomu/hutaomuchat/server/internal/store"
)

// handleAdminStats returns the dashboard counters and time series.
func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	users := s.Store.ListUsers()
	chats := s.Store.AllChats()

	var (
		totalGroups        int
		totalMessages      = s.Store.TotalMessages()
		todayMessages      int
		todayRegistrations int
		totalMoments       int
		todayMoments       int
		totalFriendship    int
		pendingJoins       int
		totalStickers      int
	)
	todayStart := startOfToday()
	for _, chat := range chats {
		if chat.Type == models.ChatGroup {
			totalGroups++
		}
	}
	for _, user := range users {
		if user.CreatedAt >= todayStart {
			todayRegistrations++
		}
	}
	for _, message := range s.recentMessages(20000) {
		if message.Timestamp >= todayStart {
			todayMessages++
		}
	}
	for _, moment := range s.Store.AllMoments() {
		totalMoments++
		if moment.CreatedAt >= todayStart {
			todayMoments++
		}
	}
	totalFriendship = s.countFriendships()
	for _, request := range s.Store.AllJoinRequests() {
		if request.Status == "PENDING" {
			pendingJoins++
		}
	}
	totalStickers = s.Store.CountStickers()

	messageTypes := map[string]int{}
	for _, message := range s.recentMessages(20000) {
		key := message.Type
		if key == "" {
			key = models.MsgText
		}
		messageTypes[key]++
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"onlineUsers":         s.Hub.OnlineCount(),
		"totalConnections":    s.Hub.ConnectionCount(),
		"totalUsers":          len(users),
		"todayRegistrations":  todayRegistrations,
		"totalChats":          len(chats),
		"totalGroups":         totalGroups,
		"totalMessages":       totalMessages,
		"todayMessages":       todayMessages,
		"totalMoments":        totalMoments,
		"todayMoments":        todayMoments,
		"totalFriendships":    totalFriendship,
		"pendingJoinRequests": pendingJoins,
		"offlineQueueSize":    s.Store.OfflineQueueSize(),
		"totalStickers":       totalStickers,
		"dailyMessages":       s.dailyMessageSeries(14),
		"dailyRegistrations":  s.dailyRegistrationSeries(14),
		"msgTypes":            messageTypes,
		"uptime":              int(time.Since(s.StartedAt).Seconds()),
		"serverName":          s.Config.ServerName(),
	})
}

// handleAdminUsers returns a paginated list of accounts.
func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
	page := httpx.Clamp(httpx.ParseInt(r.URL.Query().Get("page"), 1), 1, 100000)
	pageSize := httpx.Clamp(httpx.ParseInt(r.URL.Query().Get("pageSize"), 20), 1, 200)

	users := s.Store.ListUsers()
	if search != "" {
		filtered := make([]*models.User, 0, len(users))
		for _, user := range users {
			if strings.Contains(strings.ToLower(user.Nickname), search) ||
				strings.Contains(user.Phone, search) ||
				strings.Contains(strings.ToLower(user.UID), search) ||
				strings.Contains(user.ID, search) {
				filtered = append(filtered, user)
			}
		}
		users = filtered
	}

	total := len(users)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	out := make([]map[string]any, 0, end-start)
	for _, user := range users[start:end] {
		out = append(out, s.adminUserSummary(user))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"users":    out,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
		"pages":    (total + pageSize - 1) / pageSize,
	})
}

// adminUserSummary builds the administration view of an account.
func (s *Server) adminUserSummary(user *models.User) map[string]any {
	return map[string]any{
		"id":             user.ID,
		"uid":            user.UID,
		"phone":          user.Phone,
		"nickname":       user.Nickname,
		"signature":      user.Signature,
		"gender":         user.Gender,
		"region":         user.Region,
		"avatar_url":     user.AvatarURL,
		"background_url": user.BackgroundURL,
		"banned":         user.Banned,
		"role":           user.Role,
		"online":         s.Hub.IsOnline(user.ID),
		"online_status":  user.OnlineStatus,
		"has_push_token": s.Store.GetPushToken(user.ID) != nil,
		"friend_count":   s.countFriendshipsOf(user.ID),
		"chat_count":     len(s.Store.ChatsForUser(user.ID)),
		"created_at":     user.CreatedAt,
		"last_seen_at":   user.LastSeenAt,
	}
}

// handleAdminUserDetail returns a single account with its relations.
func (s *Server) handleAdminUserDetail(w http.ResponseWriter, r *http.Request) {
	user := s.Store.FindUserByID(r.PathValue("id"))
	if user == nil {
		httpx.WriteError(w, http.StatusNotFound, "用户不存在")
		return
	}
	friends := []map[string]any{}
	for _, id := range s.Store.FriendIDs(user.ID) {
		if friend := s.Store.FindUserByID(id); friend != nil {
			friends = append(friends, map[string]any{
				"id":       friend.ID,
				"uid":      friend.UID,
				"nickname": friend.Nickname,
				"avatar":   friend.AvatarURL,
			})
		}
	}
	chats := []map[string]any{}
	for _, chat := range s.Store.ChatsForUser(user.ID) {
		chats = append(chats, map[string]any{
			"id":      chat.ID,
			"name":    chat.Name,
			"type":    chat.Type,
			"role":    s.Store.ChatMemberRole(chat.ID, user.ID),
			"members": len(s.Store.ChatMembers(chat.ID)),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"user":    s.adminUserSummary(user),
		"friends": friends,
		"chats":   chats,
	})
}

// handleAdminUpdateUser edits an account.
func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	user := s.Store.FindUserByID(r.PathValue("id"))
	if user == nil {
		httpx.WriteError(w, http.StatusNotFound, "用户不存在")
		return
	}
	var req struct {
		Nickname              *string `json:"nickname"`
		Signature             *string `json:"signature"`
		AvatarURL             *string `json:"avatar_url"`
		BackgroundURL         *string `json:"background_url"`
		BackgroundOriginalURL *string `json:"background_original_url"`
		Phone                 *string `json:"phone"`
		Region                *string `json:"region"`
		Gender                *string `json:"gender"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Nickname != nil {
		user.Nickname = truncate(strings.TrimSpace(*req.Nickname), 24)
	}
	if req.Signature != nil {
		user.Signature = truncate(*req.Signature, 200)
	}
	if req.AvatarURL != nil {
		user.AvatarURL = truncate(*req.AvatarURL, 512)
	}
	if req.BackgroundURL != nil {
		user.BackgroundURL = truncate(*req.BackgroundURL, 512)
	}
	if req.BackgroundOriginalURL != nil {
		user.BackgroundOriginalURL = truncate(*req.BackgroundOriginalURL, 512)
	}
	if req.Region != nil {
		user.Region = truncate(*req.Region, 32)
	}
	if req.Gender != nil {
		user.Gender = *req.Gender
	}
	if req.Phone != nil {
		phone := strings.TrimSpace(*req.Phone)
		if phone != "" {
			if existing := s.Store.FindUserByPhone(phone); existing != nil && existing.ID != user.ID {
				httpx.WriteError(w, http.StatusConflict, "该手机号已被使用")
				return
			}
			user.Phone = phone
		}
	}
	s.Store.MarkDirty()
	s.audit(r, "UPDATE_USER", user.ID, user.Nickname)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "user": s.adminUserSummary(user)})
}

// handleAdminDeleteUser removes an account and all of its data.
func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	user := s.Store.FindUserByID(userID)
	if user == nil {
		httpx.WriteError(w, http.StatusNotFound, "用户不存在")
		return
	}
	s.Hub.DisconnectUser(userID, "account deleted")
	s.Store.DeleteUser(userID)
	s.audit(r, "DELETE_USER", userID, user.Nickname)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleAdminBanUser blocks an account.
func (s *Server) handleAdminBanUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	user := s.Store.FindUserByID(userID)
	if user == nil {
		httpx.WriteError(w, http.StatusNotFound, "用户不存在")
		return
	}
	s.Store.UpdateUser(userID, func(u *models.User) { u.Banned = true })
	s.Hub.DisconnectUser(userID, "banned")
	s.audit(r, "BAN_USER", userID, user.Nickname)
	s.notifyAdmins("user.banned", map[string]any{"userId": userID, "nickname": user.Nickname})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "banned": true})
}

// handleAdminUnbanUser unblocks an account.
func (s *Server) handleAdminUnbanUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	user := s.Store.FindUserByID(userID)
	if user == nil {
		httpx.WriteError(w, http.StatusNotFound, "用户不存在")
		return
	}
	s.Store.UpdateUser(userID, func(u *models.User) { u.Banned = false })
	s.audit(r, "UNBAN_USER", userID, user.Nickname)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "banned": false})
}

// handleAdminResetPassword sets a new password for an account.
func (s *Server) handleAdminResetPassword(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	user := s.Store.FindUserByID(userID)
	if user == nil {
		httpx.WriteError(w, http.StatusNotFound, "用户不存在")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Password) < 6 {
		httpx.WriteError(w, http.StatusBadRequest, "密码长度至少 6 个字符")
		return
	}
	hash, err := security.HashPassword(req.Password)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "密码加密失败")
		return
	}
	s.Store.UpdateUser(userID, func(u *models.User) { u.PasswordHash = hash })
	s.audit(r, "RESET_PASSWORD", userID, user.Nickname)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleAdminChats returns a paginated list of conversations.
func (s *Server) handleAdminChats(w http.ResponseWriter, r *http.Request) {
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
	chatType := strings.TrimSpace(r.URL.Query().Get("type"))
	page := httpx.Clamp(httpx.ParseInt(r.URL.Query().Get("page"), 1), 1, 100000)
	pageSize := httpx.Clamp(httpx.ParseInt(r.URL.Query().Get("pageSize"), 20), 1, 200)

	chats := s.Store.AllChats()
	filtered := make([]*models.Chat, 0, len(chats))
	for _, chat := range chats {
		if chatType != "" && chat.Type != chatType {
			continue
		}
		if search != "" &&
			!strings.Contains(strings.ToLower(chat.Name), search) &&
			!strings.Contains(chat.ID, search) &&
			!strings.Contains(strings.ToLower(chat.GroupUID), search) {
			continue
		}
		filtered = append(filtered, chat)
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].UpdatedAt > filtered[j].UpdatedAt })

	total := len(filtered)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	out := make([]map[string]any, 0, end-start)
	for _, chat := range filtered[start:end] {
		memberIDs := s.Store.ChatMembers(chat.ID)
		members := []map[string]any{}
		for _, id := range memberIDs {
			if member := s.Store.FindUserByID(id); member != nil {
				members = append(members, map[string]any{
					"id":       member.ID,
					"uid":      member.UID,
					"nickname": member.Nickname,
					"avatar":   member.AvatarURL,
					"online":   s.Hub.IsOnline(member.ID),
				})
			}
		}
		ownerName := "系统"
		if owner := s.Store.FindUserByID(chat.OwnerID); owner != nil {
			ownerName = owner.Nickname
		}
		out = append(out, map[string]any{
			"id":            chat.ID,
			"type":          chat.Type,
			"name":          chat.Name,
			"group_uid":     chat.GroupUID,
			"owner_id":      chat.OwnerID,
			"owner_name":    ownerName,
			"member_count":  len(memberIDs),
			"message_count": s.Store.CountMessages(chat.ID),
			"created_at":    chat.CreatedAt,
			"updated_at":    chat.UpdatedAt,
			"members":       members,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"chats":    out,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
		"pages":    (total + pageSize - 1) / pageSize,
	})
}

// handleAdminChatMessages returns the messages of a conversation.
func (s *Server) handleAdminChatMessages(w http.ResponseWriter, r *http.Request) {
	chatID := r.PathValue("id")
	page := httpx.Clamp(httpx.ParseInt(r.URL.Query().Get("page"), 1), 1, 100000)
	pageSize := httpx.Clamp(httpx.ParseInt(r.URL.Query().Get("pageSize"), 50), 1, 500)
	messages := s.Store.MessagesOf(chatID, 0)
	total := len(messages)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	out := make([]map[string]any, 0, end-start)
	for _, message := range messages[start:end] {
		senderName := "未知"
		if sender := s.Store.FindUserByID(message.SenderID); sender != nil {
			senderName = sender.Nickname
		}
		out = append(out, map[string]any{
			"id":          message.ID,
			"chat_id":     message.ChatID,
			"sender_id":   message.SenderID,
			"sender_name": senderName,
			"content":     message.Content,
			"type":        message.Type,
			"status":      message.Status,
			"timestamp":   message.Timestamp,
			"at_all":      message.AtAll,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"messages": out,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
		"pages":    (total + pageSize - 1) / pageSize,
	})
}

// handleAdminDeleteChat removes a conversation.
func (s *Server) handleAdminDeleteChat(w http.ResponseWriter, r *http.Request) {
	chatID := r.PathValue("id")
	if !s.Store.DeleteChat(chatID) {
		httpx.WriteError(w, http.StatusNotFound, "会话不存在")
		return
	}
	s.audit(r, "DELETE_CHAT", chatID, "")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleAdminMoments returns a paginated list of moments.
func (s *Server) handleAdminMoments(w http.ResponseWriter, r *http.Request) {
	page := httpx.Clamp(httpx.ParseInt(r.URL.Query().Get("page"), 1), 1, 100000)
	pageSize := httpx.Clamp(httpx.ParseInt(r.URL.Query().Get("pageSize"), 20), 1, 200)
	moments := s.Store.AllMoments()
	total := len(moments)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	out := make([]map[string]any, 0, end-start)
	for _, moment := range moments[start:end] {
		authorName := "未知"
		authorAvatar := ""
		if author := s.Store.FindUserByID(moment.UserID); author != nil {
			authorName = author.Nickname
			authorAvatar = author.AvatarURL
		}
		likes := []map[string]any{}
		for _, likerID := range s.Store.MomentLikes(moment.ID) {
			if liker := s.Store.FindUserByID(likerID); liker != nil {
				likes = append(likes, map[string]any{
					"id":       liker.ID,
					"nickname": liker.Nickname,
					"avatar":   liker.AvatarURL,
				})
			}
		}
		comments := []map[string]any{}
		for _, comment := range s.Store.MomentComments(moment.ID) {
			comments = append(comments, map[string]any{
				"id":          comment.ID,
				"content":     comment.Content,
				"image_url":   comment.ImageURL,
				"author_name": comment.Nickname,
				"created_at":  comment.CreatedAt,
			})
		}
		out = append(out, map[string]any{
			"id":            moment.ID,
			"user_id":       moment.UserID,
			"author_name":   authorName,
			"author_avatar": authorAvatar,
			"content":       moment.Content,
			"images":        moment.Images,
			"scope":         moment.Scope,
			"created_at":    moment.CreatedAt,
			"like_count":    len(likes),
			"comment_count": len(comments),
			"likes":         likes,
			"comments":      comments,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"moments":  out,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
		"pages":    (total + pageSize - 1) / pageSize,
	})
}

// handleAdminDeleteMoment removes a moment.
func (s *Server) handleAdminDeleteMoment(w http.ResponseWriter, r *http.Request) {
	momentID := r.PathValue("id")
	if !s.Store.RemoveMoment(momentID) {
		httpx.WriteError(w, http.StatusNotFound, "动态不存在")
		return
	}
	s.audit(r, "DELETE_MOMENT", momentID, "")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleAdminAnnouncements returns every announcement.
func (s *Server) handleAdminAnnouncements(w http.ResponseWriter, r *http.Request) {
	announcements := s.Store.AllAnnouncements()
	out := make([]map[string]any, 0, len(announcements))
	for _, announcement := range announcements {
		creatorName := "系统"
		if creator := s.Store.FindUserByID(announcement.CreatedBy); creator != nil {
			creatorName = creator.Nickname
		}
		chatName := "未知群组"
		if chat := s.Store.GetChat(announcement.ChatID); chat != nil {
			chatName = chat.Name
		}
		out = append(out, map[string]any{
			"id":           announcement.ID,
			"chat_id":      announcement.ChatID,
			"chat_name":    chatName,
			"content":      announcement.Content,
			"creator_name": creatorName,
			"created_by":   announcement.CreatedBy,
			"created_at":   announcement.CreatedAt,
			"at_all":       announcement.AtAll,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"announcements": out})
}

// handleAdminDeleteAnnouncement removes an announcement.
func (s *Server) handleAdminDeleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	announcementID := r.PathValue("id")
	if !s.Store.RemoveAnnouncement(announcementID) {
		httpx.WriteError(w, http.StatusNotFound, "公告不存在")
		return
	}
	s.audit(r, "DELETE_ANNOUNCEMENT", announcementID, "")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleAdminJoinRequests returns every group join request.
func (s *Server) handleAdminJoinRequests(w http.ResponseWriter, r *http.Request) {
	requests := s.Store.AllJoinRequests()
	out := make([]map[string]any, 0, len(requests))
	for _, request := range requests {
		userName := "未知"
		userUID := ""
		if user := s.Store.FindUserByID(request.FromUserID); user != nil {
			userName = user.Nickname
			userUID = user.UID
		}
		chatName := "未知群组"
		if chat := s.Store.GetChat(request.ChatID); chat != nil {
			chatName = chat.Name
		}
		out = append(out, map[string]any{
			"id":           request.ID,
			"from_user_id": request.FromUserID,
			"user_name":    userName,
			"user_uid":     userUID,
			"chat_id":      request.ChatID,
			"chat_name":    chatName,
			"message":      request.Message,
			"status":       request.Status,
			"created_at":   request.CreatedAt,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"joinRequests": out})
}

// handleAdminOnline returns the list of connected users.
func (s *Server) handleAdminOnline(w http.ResponseWriter, r *http.Request) {
	out := []map[string]any{}
	for _, userID := range s.Hub.OnlineUsers() {
		user := s.Store.FindUserByID(userID)
		if user == nil {
			continue
		}
		out = append(out, map[string]any{
			"id":            user.ID,
			"uid":           user.UID,
			"nickname":      user.Nickname,
			"avatar_url":    user.AvatarURL,
			"connections":   len(s.Hub.ClientsOf(user.ID)),
			"last_seen_at":  user.LastSeenAt,
			"registered_at": user.CreatedAt,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i]["nickname"].(string) < out[j]["nickname"].(string)
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"online": out, "total": len(out)})
}

// handleAdminBroadcast sends a system notification to every connected client.
func (s *Server) handleAdminBroadcast(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	title := truncate(strings.TrimSpace(req.Title), 60)
	content := truncate(strings.TrimSpace(req.Content), 500)
	if title == "" || content == "" {
		httpx.WriteError(w, http.StatusBadRequest, "标题和内容不能为空")
		return
	}
	reached := s.Hub.Broadcast(map[string]any{
		"type":    "system_broadcast",
		"title":   title,
		"content": content,
	})
	s.audit(r, "BROADCAST", title, content)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "reached": reached})
}

// handleAdminSystem returns the runtime information of the server.
func (s *Server) handleAdminSystem(w http.ResponseWriter, r *http.Request) {
	files := []map[string]any{}
	var totalSize int64
	if entries, err := os.ReadDir(s.UploadsDir); err == nil {
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			totalSize += info.Size()
			files = append(files, map[string]any{
				"name":    entry.Name(),
				"size":    info.Size(),
				"created": info.ModTime().UnixMilli(),
			})
		}
	}
	sort.SliceStable(files, func(i, j int) bool {
		return files[i]["created"].(int64) > files[j]["created"].(int64)
	})

	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"uptime":     int(time.Since(s.StartedAt).Seconds()),
		"started_at": s.StartedAt.UnixMilli(),
		"memory": map[string]any{
			"alloc":      memory.Alloc,
			"total":      memory.TotalAlloc,
			"sys":        memory.Sys,
			"heap_alloc": memory.HeapAlloc,
			"num_gc":     memory.NumGC,
		},
		"goroutines":     runtime.NumGoroutine(),
		"ws_connections": s.Hub.ConnectionCount(),
		"online_users":   s.Hub.OnlineCount(),
		"upload_files":   files,
		"upload_count":   len(files),
		"upload_size":    totalSize,
		"push_status":    s.Push.Status(),
		"go_version":     runtime.Version(),
		"platform":       runtime.GOOS + "/" + runtime.GOARCH,
		"num_cpu":        runtime.NumCPU(),
		"server_name":    s.Config.ServerName(),
		"data_dir":       s.DataDir,
	})
}

// handleAdminDeleteFile removes an uploaded file.
func (s *Server) handleAdminDeleteFile(w http.ResponseWriter, r *http.Request) {
	filename := httpx.PathToken(r.PathValue("filename"))
	if filename == "" {
		httpx.WriteError(w, http.StatusBadRequest, "文件名无效")
		return
	}
	path := filepath.Join(s.UploadsDir, filename)
	if _, err := os.Stat(path); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "文件不存在")
		return
	}
	if err := os.Remove(path); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "删除失败: "+err.Error())
		return
	}
	s.audit(r, "DELETE_FILE", filename, "")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleAdminSystemLogs returns the system log records.
func (s *Server) handleAdminSystemLogs(w http.ResponseWriter, r *http.Request) {
	limit := httpx.Clamp(httpx.ParseInt(r.URL.Query().Get("limit"), 100), 1, 1000)
	level := r.URL.Query().Get("level")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"logs": s.Logger.SystemRecords(limit, level)})
}

// handleAdminUserLogs returns the user action log records.
func (s *Server) handleAdminUserLogs(w http.ResponseWriter, r *http.Request) {
	limit := httpx.Clamp(httpx.ParseInt(r.URL.Query().Get("limit"), 100), 1, 1000)
	userID := r.URL.Query().Get("userId")
	action := r.URL.Query().Get("action")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"logs": s.Logger.UserRecords(limit, userID, action)})
}

// handleAdminLogFile returns the tail of a daily log file.
func (s *Server) handleAdminLogFile(w http.ResponseWriter, r *http.Request) {
	stream := r.PathValue("type")
	date := r.URL.Query().Get("date")
	lines := httpx.Clamp(httpx.ParseInt(r.URL.Query().Get("lines"), 200), 1, 5000)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"content": s.Logger.LogFileContent(stream, date, lines),
		"type":    stream,
		"date":    date,
	})
}

// handleAdminLogFiles returns the available log files.
func (s *Server) handleAdminLogFiles(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, s.Logger.LogFiles())
}

// handleAdminLogStats returns the aggregated log statistics.
func (s *Server) handleAdminLogStats(w http.ResponseWriter, r *http.Request) {
	systemLogs := s.Logger.SystemRecords(1000, "")
	userLogs := s.Logger.UserRecords(1000, "", "")

	levels := map[string]int{}
	for _, record := range systemLogs {
		levels[record.Level]++
	}
	actions := map[string]int{}
	activity := map[string]int{}
	for _, record := range userLogs {
		actions[record.Action]++
		activity[record.UserID]++
	}
	type topUser struct {
		UserID   string `json:"userId"`
		Count    int    `json:"count"`
		Nickname string `json:"nickname"`
	}
	top := make([]topUser, 0, len(activity))
	for userID, count := range activity {
		nickname := "未知"
		if user := s.Store.FindUserByID(userID); user != nil {
			nickname = user.Nickname
		}
		top = append(top, topUser{UserID: userID, Count: count, Nickname: nickname})
	}
	sort.SliceStable(top, func(i, j int) bool { return top[i].Count > top[j].Count })
	if len(top) > 10 {
		top = top[:10]
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"systemLogCount": len(systemLogs),
		"userLogCount":   len(userLogs),
		"systemLevels":   levels,
		"userActions":    actions,
		"topUsers":       top,
	})
}

// handleAdminAudit returns the recorded administration actions.
func (s *Server) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	limit := httpx.Clamp(httpx.ParseInt(r.URL.Query().Get("limit"), 100), 1, 500)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"audit": s.Store.AuditLogs(limit)})
}

// handleAdminSettings returns the server configuration.
func (s *Server) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	cfg := s.Config.Get()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"serverName":      cfg.ServerName,
		"serverPort":      cfg.ServerPort,
		"adminUser":       cfg.AdminUser,
		"hasPassword":     cfg.AdminPassHash != "",
		"hasJwtSecret":    cfg.JWTSecret != "",
		"jwtSecret":       "********",
		"setupTime":       cfg.SetupTime,
		"getuiConfigured": s.Push.Available(),
	})
}

// handleAdminUpdateSettings stores the server configuration.
func (s *Server) handleAdminUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServerName *string `json:"serverName"`
		ServerPort *int    `json:"serverPort"`
		AdminUser  *string `json:"adminUser"`
		AdminPass  *string `json:"adminPass"`
		JWTSecret  *string `json:"jwtSecret"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Config.Update(func(cfg *config.Config) {
		if req.ServerName != nil && *req.ServerName != "" {
			cfg.ServerName = *req.ServerName
		}
		if req.ServerPort != nil && *req.ServerPort > 0 && *req.ServerPort < 65536 {
			cfg.ServerPort = *req.ServerPort
		}
		if req.AdminUser != nil && *req.AdminUser != "" {
			cfg.AdminUser = *req.AdminUser
		}
		if req.JWTSecret != nil && *req.JWTSecret != "" && *req.JWTSecret != "********" {
			cfg.JWTSecret = *req.JWTSecret
		}
	}); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "保存设置失败: "+err.Error())
		return
	}
	if req.AdminPass != nil && *req.AdminPass != "" {
		if len(*req.AdminPass) < 6 {
			httpx.WriteError(w, http.StatusBadRequest, "管理员密码至少 6 个字符")
			return
		}
		if err := s.Config.SetAdminPassword(*req.AdminPass); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "保存密码失败: "+err.Error())
			return
		}
	}
	s.audit(r, "UPDATE_SETTINGS", "server", "服务器设置已更新")
	s.Logger.Info("服务器设置已更新", map[string]any{"admin": adminName(r)})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "设置已保存，部分设置需要重启服务端才能生效",
	})
}

// handleAdminUpdateGetui stores the GeTui configuration.
func (s *Server) handleAdminUpdateGetui(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AppID        string `json:"appId"`
		AppKey       string `json:"appKey"`
		MasterSecret string `json:"masterSecret"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.AppID == "" || req.AppKey == "" || req.MasterSecret == "" {
		httpx.WriteError(w, http.StatusBadRequest, "所有字段都是必填的")
		return
	}
	if err := s.saveGetuiConfig(req.AppID, req.AppKey, req.MasterSecret); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "保存配置失败: "+err.Error())
		return
	}
	s.audit(r, "UPDATE_GETUI", "getui", "个推配置已更新")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "个推配置已保存，需要重启服务端才能生效",
	})
}

// handleAdminReset clears every piece of data of the server.
func (s *Server) handleAdminReset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm string `json:"confirm"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Confirm != "RESET_ALL_DATA" {
		httpx.WriteError(w, http.StatusBadRequest, "请提供正确的确认码: RESET_ALL_DATA")
		return
	}
	s.resetEverything()
	s.audit(r, "RESET_SERVER", "all", "服务端数据已清空")
	s.Logger.Warn("服务端已被管理员重置", map[string]any{"admin": adminName(r)})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "服务端已格式化，所有数据已清除，需要重启服务端",
	})
}

// handleAdminRestart asks the process manager to restart the server.
func (s *Server) handleAdminRestart(w http.ResponseWriter, r *http.Request) {
	s.audit(r, "RESTART_SERVER", "server", "请求重启")
	s.Logger.Info("管理员请求重启服务端", map[string]any{"admin": adminName(r)})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "重启信号已发送，服务端将在 2 秒后重启",
	})
	go func() {
		time.Sleep(2 * time.Second)
		restartServer()
	}()
}

// ---------------------------------------------------------------- helpers --

// audit records an administration action.
func (s *Server) audit(r *http.Request, action, target, detail string) {
	s.Store.AddAuditLog(models.AuditLog{
		ID:        store.NewID(),
		AdminUser: adminName(r),
		Action:    action,
		Target:    target,
		Detail:    detail,
		IP:        httpx.ClientIP(r),
		CreatedAt: models.Now(),
	})
}

// countFriendshipsOf returns the number of friends of a user.
func (s *Server) countFriendshipsOf(userID string) int {
	return len(s.Store.FriendIDs(userID))
}

// countFriendships returns the total number of friendships.
func (s *Server) countFriendships() int {
	total := 0
	for _, user := range s.Store.ListUsers() {
		total += len(s.Store.FriendIDs(user.ID))
	}
	return total / 2
}

// countStickers returns the total number of stored stickers.
func (s *Server) countStickers() int {
	return s.Store.CountStickers()
}

// recentMessages returns the most recent messages of every conversation.
func (s *Server) recentMessages(limit int) []*models.Message {
	out := []*models.Message{}
	for _, chat := range s.Store.AllChats() {
		out = append(out, s.Store.MessagesOf(chat.ID, limit)...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Timestamp > out[j].Timestamp })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// dailyMessageSeries returns the message counters of the last n days.
func (s *Server) dailyMessageSeries(days int) []map[string]any {
	series := []map[string]any{}
	now := time.Now()
	for i := days - 1; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location()).UnixMilli()
		end := start + 24*60*60*1000
		count := 0
		for _, message := range s.recentMessages(50000) {
			if message.Timestamp >= start && message.Timestamp < end {
				count++
			}
		}
		series = append(series, map[string]any{
			"date":  day.Format("2006-01-02"),
			"label": day.Format("01-02"),
			"count": count,
		})
	}
	return series
}

// dailyRegistrationSeries returns the registration counters of the last n days.
func (s *Server) dailyRegistrationSeries(days int) []map[string]any {
	series := []map[string]any{}
	users := s.Store.ListUsers()
	now := time.Now()
	for i := days - 1; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location()).UnixMilli()
		end := start + 24*60*60*1000
		count := 0
		for _, user := range users {
			if user.CreatedAt >= start && user.CreatedAt < end {
				count++
			}
		}
		series = append(series, map[string]any{
			"date":  day.Format("2006-01-02"),
			"label": day.Format("01-02"),
			"count": count,
		})
	}
	return series
}

// startOfToday returns the timestamp of midnight in the local timezone.
func startOfToday() int64 {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UnixMilli()
}
