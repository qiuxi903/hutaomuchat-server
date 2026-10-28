package api

import (
	"net/http"
	"strings"

	"github.com/hutaomu/hutaomuchat/server/internal/httpx"
	"github.com/hutaomu/hutaomuchat/server/internal/models"
	"github.com/hutaomu/hutaomuchat/server/internal/store"
)

// handleListChats returns every conversation of the authenticated user.
func (s *Server) handleListChats(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chats := s.Store.ChatsForUser(user.ID)
	out := make([]models.ChatDTO, 0, len(chats))
	for _, chat := range chats {
		dto := models.NewChatDTO(chat)
		members := s.Store.ChatMembers(chat.ID)
		dto.MemberCount = len(members)
		dto.HasAnnouncement = chat.Announcement != ""
		if chat.Type == models.ChatSingle {
			dto.OnlineStatus = models.Offline
			for _, memberID := range members {
				if memberID == user.ID {
					continue
				}
				if partner := s.Store.FindUserByID(memberID); partner != nil {
					dto.OnlineStatus = partner.OnlineStatus
					if dto.Name == "" {
						dto.Name = partner.Nickname
					}
					if dto.AvatarURL == "" {
						dto.AvatarURL = partner.AvatarURL
					}
				}
			}
		}
		out = append(out, dto)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"chats": out})
}

// handleCreateSingleChat opens a direct conversation with a friend.
func (s *Server) handleCreateSingleChat(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var req struct {
		FriendID string `json:"friend_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.FriendID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "缺少 friend_id")
		return
	}
	if req.FriendID == user.ID {
		httpx.WriteError(w, http.StatusBadRequest, "不能和自己聊天")
		return
	}
	partner := s.Store.FindUserByID(req.FriendID)
	if partner == nil {
		httpx.WriteError(w, http.StatusNotFound, "用户不存在")
		return
	}
	chat := s.ensureSingleChat(user.ID, partner.ID)
	dto := models.NewChatDTO(chat)
	dto.MemberCount = 2
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"chat": dto})
}

// handleCreateGroupChat creates a group conversation.
func (s *Server) handleCreateGroupChat(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var req struct {
		Name      string   `json:"name"`
		MemberIDs []string `json:"member_ids"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "群聊名称不能为空")
		return
	}
	if len([]rune(name)) > 32 {
		httpx.WriteError(w, http.StatusBadRequest, "群聊名称过长")
		return
	}
	memberIDs := []string{}
	seen := map[string]bool{user.ID: true}
	for _, id := range req.MemberIDs {
		if id == "" || seen[id] {
			continue
		}
		if s.Store.FindUserByID(id) == nil {
			continue
		}
		seen[id] = true
		memberIDs = append(memberIDs, id)
	}
	if len(memberIDs) == 0 {
		memberIDs = append(memberIDs, user.ID)
	}

	chat := &models.Chat{
		ID:        store.NewID(),
		Type:      models.ChatGroup,
		Name:      name,
		OwnerID:   user.ID,
		Admins:    []string{user.ID},
		MutedBy:   []string{},
		PinnedBy:  []string{},
		GroupUID:  s.Store.NextGroupUID(),
		CreatedAt: models.Now(),
		UpdatedAt: models.Now(),
	}
	s.Store.CreateChat(chat)
	s.Store.AddChatMember(chat.ID, user.ID, models.RoleOwner)
	for _, id := range memberIDs {
		if id == user.ID {
			continue
		}
		s.Store.AddChatMember(chat.ID, id, models.RoleMember)
	}
	s.Logger.User(user.ID, "CREATE_CHAT", map[string]any{"chatId": chat.ID, "isGroup": true})

	dto := models.NewChatDTO(chat)
	dto.MemberCount = len(s.Store.ChatMembers(chat.ID))
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"chat": dto})
}

// handleGlobalChat returns the identifier of the public room.
func (s *Server) handleGlobalChat(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	s.Store.AutoJoinGlobalChat(user.ID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"chatId": models.GlobalChatID})
}

// handleSearchGroups finds a group by its group number.
func (s *Server) handleSearchGroups(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		httpx.WriteError(w, http.StatusBadRequest, "缺少搜索关键字")
		return
	}
	out := []models.ChatDTO{}
	for _, chat := range s.Store.AllChats() {
		if chat.Type != models.ChatGroup || chat.ID == models.GlobalChatID {
			continue
		}
		if !strings.EqualFold(chat.GroupUID, query) {
			continue
		}
		dto := models.NewChatDTO(chat)
		dto.MemberCount = len(s.Store.ChatMembers(chat.ID))
		out = append(out, dto)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"groups": out})
}

// handleGetChat returns a conversation and its members.
func (s *Server) handleGetChat(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chatID := r.PathValue("id")
	if !s.Store.IsChatMember(chatID, user.ID) {
		httpx.WriteError(w, http.StatusForbidden, "你不是该会话成员")
		return
	}
	chat := s.Store.GetChat(chatID)
	if chat == nil {
		httpx.WriteError(w, http.StatusNotFound, "会话不存在")
		return
	}
	dto := models.NewChatDTO(chat)
	dto.MemberCount = len(s.Store.ChatMembers(chatID))
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"chat":    dto,
		"members": s.Store.ChatMembers(chatID),
	})
}

// handleLeaveGroup removes the authenticated user from a group.
func (s *Server) handleLeaveGroup(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chatID := r.PathValue("id")
	if chatID == models.GlobalChatID {
		httpx.WriteError(w, http.StatusBadRequest, "用户圈不能退出")
		return
	}
	chat := s.Store.GetChat(chatID)
	if chat == nil {
		httpx.WriteError(w, http.StatusNotFound, "会话不存在")
		return
	}
	if chat.Type != models.ChatGroup {
		httpx.WriteError(w, http.StatusBadRequest, "只能退出群聊")
		return
	}
	if !s.Store.IsChatMember(chatID, user.ID) {
		httpx.WriteError(w, http.StatusBadRequest, "你不是该群成员")
		return
	}

	s.Store.RemoveChatMember(chatID, user.ID)
	if chat.OwnerID == user.ID {
		remaining := s.Store.ChatMembers(chatID)
		if len(remaining) > 0 {
			admins := []string{}
			for _, id := range chat.Admins {
				if id != user.ID {
					admins = append(admins, id)
				}
			}
			newOwner := remaining[0]
			if len(admins) > 0 {
				newOwner = admins[0]
			}
			chat.OwnerID = newOwner
			chat.Admins = append(admins, newOwner)
		}
	}
	chat.Admins = removeFrom(chat.Admins, user.ID)
	s.Store.MarkDirty()

	s.Logger.User(user.ID, "LEAVE_CHAT", map[string]any{"chatId": chatID})
	s.Hub.BroadcastUsers(s.Store.ChatMembers(chatID), map[string]any{
		"type":     "member_left",
		"chatId":   chatID,
		"userId":   user.ID,
		"userName": user.Nickname,
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handlePostAnnouncement publishes a group announcement.
func (s *Server) handlePostAnnouncement(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chatID := r.PathValue("id")
	var req struct {
		Content string `json:"content"`
		AtAll   bool   `json:"at_all"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	chat := s.Store.GetChat(chatID)
	if chat == nil {
		httpx.WriteError(w, http.StatusNotFound, "会话不存在")
		return
	}
	if !s.isChatAdmin(user.ID, chat) {
		httpx.WriteError(w, http.StatusForbidden, "只有群主或管理员可以发布公告")
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		httpx.WriteError(w, http.StatusBadRequest, "公告内容不能为空")
		return
	}

	atAll := req.AtAll
	if atAll {
		allowed, _ := s.Store.CheckAtAll(user.ID)
		if !allowed {
			atAll = false
		}
	}

	announcement := s.Store.AddAnnouncement(models.Announcement{
		ID:        store.NewID(),
		ChatID:    chatID,
		Content:   truncate(content, 1000),
		CreatedBy: user.ID,
		CreatedAt: models.Now(),
		AtAll:     atAll,
	})
	chat.Announcement = truncate(content, 200)
	chat.UpdatedAt = models.Now()
	s.Store.MarkDirty()

	message := s.Store.AddMessage(&models.Message{
		ID:         store.NewID(),
		ChatID:     chatID,
		SenderID:   user.ID,
		SenderNick: user.Nickname,
		Content:    content,
		Type:       models.MsgCard,
		Status:     "SENT",
		Timestamp:  models.Now(),
		AtAll:      atAll,
	})
	s.broadcastChatMessage(chatID, user.ID, message, atAll)

	s.Logger.User(user.ID, "POST_ANNOUNCEMENT", map[string]any{"chatId": chatID, "atAll": atAll})

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"announcement": announcementDTO(s, announcement),
		"message":      models.NewMessageDTO(message),
	})
}

// handleListAnnouncements returns the announcements of a conversation.
func (s *Server) handleListAnnouncements(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chatID := r.PathValue("id")
	if !s.Store.IsChatMember(chatID, user.ID) {
		httpx.WriteError(w, http.StatusForbidden, "你不是该会话成员")
		return
	}
	announcements := s.Store.AnnouncementsOf(chatID)
	out := make([]models.AnnouncementDTO, 0, len(announcements))
	for _, announcement := range announcements {
		out = append(out, announcementDTO(s, &announcement))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"announcements": out})
}

// handleDeleteAnnouncement removes an announcement.
func (s *Server) handleDeleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chatID := r.PathValue("id")
	announcementID := r.PathValue("announcementId")
	chat := s.Store.GetChat(chatID)
	if chat == nil {
		httpx.WriteError(w, http.StatusNotFound, "会话不存在")
		return
	}
	if !s.isChatAdmin(user.ID, chat) {
		httpx.WriteError(w, http.StatusForbidden, "只有群主或管理员可以删除公告")
		return
	}
	if !s.Store.RemoveAnnouncement(announcementID) {
		httpx.WriteError(w, http.StatusNotFound, "公告不存在")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleUpdateGroupAvatar changes the avatar of a group.
func (s *Server) handleUpdateGroupAvatar(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chatID := r.PathValue("id")
	var req struct {
		AvatarURL string `json:"avatar_url"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	chat := s.Store.GetChat(chatID)
	if chat == nil {
		httpx.WriteError(w, http.StatusNotFound, "会话不存在")
		return
	}
	if !s.isChatAdmin(user.ID, chat) {
		httpx.WriteError(w, http.StatusForbidden, "只有群主或管理员可以修改群头像")
		return
	}
	url := truncate(req.AvatarURL, 512)
	chat.GroupAvatar = url
	chat.AvatarURL = url
	s.Store.MarkDirty()
	s.Logger.User(user.ID, "UPDATE_GROUP_AVATAR", map[string]any{"chatId": chatID})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"chat": models.NewChatDTO(chat)})
}

// handleToggleMute mutes or unmutes a conversation for the current user.
func (s *Server) handleToggleMute(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chatID := r.PathValue("id")
	chat := s.Store.GetChat(chatID)
	if chat == nil {
		httpx.WriteError(w, http.StatusNotFound, "会话不存在")
		return
	}
	muted := toggleInList(&chat.MutedBy, user.ID)
	s.Store.MarkDirty()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"muted": muted})
}

// handleTogglePin pins or unpins a conversation for the current user.
func (s *Server) handleTogglePin(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chatID := r.PathValue("id")
	chat := s.Store.GetChat(chatID)
	if chat == nil {
		httpx.WriteError(w, http.StatusNotFound, "会话不存在")
		return
	}
	pinned := toggleInList(&chat.PinnedBy, user.ID)
	s.Store.MarkDirty()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"pinned": pinned})
}

// handleAddAdmin promotes a member to administrator.
func (s *Server) handleAddAdmin(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chatID := r.PathValue("id")
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	chat := s.Store.GetChat(chatID)
	if chat == nil {
		httpx.WriteError(w, http.StatusNotFound, "会话不存在")
		return
	}
	if chat.OwnerID != user.ID {
		httpx.WriteError(w, http.StatusForbidden, "只有群主可以任命管理员")
		return
	}
	if !s.Store.IsChatMember(chatID, req.UserID) {
		httpx.WriteError(w, http.StatusBadRequest, "该用户不是群成员")
		return
	}
	if !containsString(chat.Admins, req.UserID) {
		chat.Admins = append(chat.Admins, req.UserID)
		s.Store.SetChatMemberRole(chatID, req.UserID, models.RoleAdmin)
	}
	s.Store.MarkDirty()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleRemoveAdmin demotes an administrator.
func (s *Server) handleRemoveAdmin(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chatID := r.PathValue("id")
	targetID := r.PathValue("userId")
	chat := s.Store.GetChat(chatID)
	if chat == nil {
		httpx.WriteError(w, http.StatusNotFound, "会话不存在")
		return
	}
	if chat.OwnerID != user.ID {
		httpx.WriteError(w, http.StatusForbidden, "只有群主可以撤销管理员")
		return
	}
	if targetID == chat.OwnerID {
		httpx.WriteError(w, http.StatusBadRequest, "不能撤销群主身份")
		return
	}
	chat.Admins = removeFrom(chat.Admins, targetID)
	s.Store.SetChatMemberRole(chatID, targetID, models.RoleMember)
	s.Store.MarkDirty()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleGroupFiles returns the shared files of a conversation.
func (s *Server) handleGroupFiles(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chatID := r.PathValue("id")
	if !s.Store.IsChatMember(chatID, user.ID) {
		httpx.WriteError(w, http.StatusForbidden, "你不是该会话成员")
		return
	}
	files := s.Store.GroupFiles(chatID, models.Now())
	out := make([]map[string]any, 0, len(files))
	for _, file := range files {
		out = append(out, map[string]any{
			"id":              file.ID,
			"chat_id":         file.ChatID,
			"sender_id":       file.SenderID,
			"sender_nickname": file.SenderNick,
			"content":         file.Content,
			"type":            file.Type,
			"status":          file.Status,
			"timestamp":       file.Timestamp,
			"file_name":       file.FileName,
			"file_size":       file.FileSize,
			"file_mime":       file.FileMIME,
			"permanent":       file.Permanent,
			"expired":         file.Expired,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"files": out})
}

// handleMarkFilePermanent protects a shared file from the automatic clean up.
func (s *Server) handleMarkFilePermanent(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chatID := r.PathValue("id")
	chat := s.Store.GetChat(chatID)
	if chat == nil {
		httpx.WriteError(w, http.StatusNotFound, "会话不存在")
		return
	}
	if !s.isChatAdmin(user.ID, chat) {
		httpx.WriteError(w, http.StatusForbidden, "只有群主或管理员可以管理群文件")
		return
	}
	s.Store.MarkFilePermanent(r.PathValue("fileId"), chatID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleDeleteGroupFile removes a shared file.
func (s *Server) handleDeleteGroupFile(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chatID := r.PathValue("id")
	fileID := r.PathValue("fileId")
	chat := s.Store.GetChat(chatID)
	if chat == nil {
		httpx.WriteError(w, http.StatusNotFound, "会话不存在")
		return
	}
	if !s.isChatAdmin(user.ID, chat) {
		httpx.WriteError(w, http.StatusForbidden, "只有群主或管理员可以管理群文件")
		return
	}
	message := s.Store.FindMessage(fileID)
	if message == nil || message.ChatID != chatID || message.Type != models.MsgFile {
		httpx.WriteError(w, http.StatusNotFound, "文件不存在")
		return
	}
	s.Store.RemoveMessage(fileID)
	s.Store.UnmarkFilePermanent(fileID)
	s.Hub.BroadcastUsers(s.Store.ChatMembers(chatID), map[string]any{
		"type":      "RECALL",
		"chatId":    chatID,
		"messageId": fileID,
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleSendJoinRequest asks to join a group.
func (s *Server) handleSendJoinRequest(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chatID := r.PathValue("id")
	var req struct {
		Message string `json:"message"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	chat := s.Store.GetChat(chatID)
	if chat == nil || chat.Type != models.ChatGroup {
		httpx.WriteError(w, http.StatusNotFound, "群聊不存在")
		return
	}
	if chatID == models.GlobalChatID {
		httpx.WriteError(w, http.StatusBadRequest, "无法申请加入用户圈")
		return
	}
	if s.Store.IsChatMember(chatID, user.ID) {
		httpx.WriteError(w, http.StatusBadRequest, "你已经是群成员")
		return
	}
	for _, request := range s.Store.JoinRequestsOf(chatID) {
		if request.FromUserID == user.ID {
			httpx.WriteError(w, http.StatusBadRequest, "已发送过申请，请等待管理员处理")
			return
		}
	}
	request := s.Store.AddJoinRequest(models.GroupJoinRequest{
		ID:         store.NewID(),
		FromUserID: user.ID,
		ChatID:     chatID,
		Message:    truncate(strings.TrimSpace(req.Message), 200),
		Status:     "PENDING",
		CreatedAt:  models.Now(),
	})
	s.Logger.User(user.ID, "JOIN_CHAT", map[string]any{"chatId": chatID})

	recipients := append([]string{chat.OwnerID}, chat.Admins...)
	s.Hub.BroadcastUsers(recipients, map[string]any{
		"type":         "JOIN_REQUEST",
		"chatId":       chatID,
		"chatName":     chat.Name,
		"from_user_id": user.ID,
		"requestId":    request.ID,
		"message":      request.Message,
	})

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "request": request})
}

// handleListJoinRequests returns the pending requests of a group.
func (s *Server) handleListJoinRequests(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chatID := r.PathValue("id")
	chat := s.Store.GetChat(chatID)
	if chat == nil {
		httpx.WriteError(w, http.StatusNotFound, "群聊不存在")
		return
	}
	if !s.isChatAdmin(user.ID, chat) {
		httpx.WriteError(w, http.StatusForbidden, "权限不足")
		return
	}
	requests := s.Store.JoinRequestsOf(chatID)
	out := make([]models.JoinRequestDTO, 0, len(requests))
	for _, request := range requests {
		dto := models.JoinRequestDTO{
			ID:         request.ID,
			FromUserID: request.FromUserID,
			ChatID:     request.ChatID,
			Message:    request.Message,
			Status:     request.Status,
			CreatedAt:  request.CreatedAt,
			ChatName:   chat.Name,
		}
		if from := s.Store.FindUserByID(request.FromUserID); from != nil {
			profile := models.NewUserDTO(from)
			dto.FromUser = &profile
		}
		out = append(out, dto)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"requests": out})
}

// handleManagedJoinRequests returns the pending requests of every group the
// authenticated user administrates.
func (s *Server) handleManagedJoinRequests(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	requests := s.Store.ManagedJoinRequests(user.ID)
	out := make([]models.JoinRequestDTO, 0, len(requests))
	for _, request := range requests {
		dto := models.JoinRequestDTO{
			ID:         request.ID,
			FromUserID: request.FromUserID,
			ChatID:     request.ChatID,
			Message:    request.Message,
			Status:     request.Status,
			CreatedAt:  request.CreatedAt,
		}
		if chat := s.Store.GetChat(request.ChatID); chat != nil {
			dto.ChatName = chat.Name
		}
		if from := s.Store.FindUserByID(request.FromUserID); from != nil {
			profile := models.NewUserDTO(from)
			dto.FromUser = &profile
		}
		out = append(out, dto)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"requests": out})
}

// handleRespondJoinRequest accepts or rejects a group join request.
func (s *Server) handleRespondJoinRequest(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var req struct {
		Status string `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	status := strings.ToUpper(strings.TrimSpace(req.Status))
	if status != "ACCEPTED" && status != "REJECTED" {
		httpx.WriteError(w, http.StatusBadRequest, "无效状态")
		return
	}
	request := s.Store.FindJoinRequest(r.PathValue("requestId"))
	if request == nil || request.Status != "PENDING" {
		httpx.WriteError(w, http.StatusNotFound, "申请不存在或已处理")
		return
	}
	chat := s.Store.GetChat(request.ChatID)
	if chat == nil {
		httpx.WriteError(w, http.StatusNotFound, "群聊不存在")
		return
	}
	if !s.isChatAdmin(user.ID, chat) {
		httpx.WriteError(w, http.StatusForbidden, "权限不足")
		return
	}
	if !s.Store.SetJoinRequestStatus(request.ID, status) {
		httpx.WriteError(w, http.StatusInternalServerError, "处理失败")
		return
	}
	if status == "ACCEPTED" {
		s.Store.AddChatMember(request.ChatID, request.FromUserID, models.RoleMember)
		s.Hub.SendToUser(request.FromUserID, map[string]any{
			"type":      "JOIN_REQUEST_RESPONSE",
			"chatId":    request.ChatID,
			"chatName":  chat.Name,
			"status":    status,
			"requestId": request.ID,
		})
		s.Hub.BroadcastUsers(s.Store.ChatMembers(request.ChatID), map[string]any{
			"type":   "member_joined",
			"chatId": request.ChatID,
			"userId": request.FromUserID,
		})
	} else {
		s.Hub.SendToUser(request.FromUserID, map[string]any{
			"type":      "JOIN_REQUEST_RESPONSE",
			"chatId":    request.ChatID,
			"chatName":  chat.Name,
			"status":    status,
			"requestId": request.ID,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// isChatAdmin reports whether the user owns or administrates the conversation.
func (s *Server) isChatAdmin(userID string, chat *models.Chat) bool {
	if chat == nil {
		return false
	}
	if chat.OwnerID == userID {
		return true
	}
	return containsString(chat.Admins, userID)
}

// announcementDTO enriches an announcement with the author information.
func announcementDTO(s *Server, announcement *models.Announcement) models.AnnouncementDTO {
	dto := models.AnnouncementDTO{
		ID:        announcement.ID,
		ChatID:    announcement.ChatID,
		Content:   announcement.Content,
		CreatedBy: announcement.CreatedBy,
		CreatedAt: announcement.CreatedAt,
		AtAll:     announcement.AtAll,
	}
	if author := s.Store.FindUserByID(announcement.CreatedBy); author != nil {
		dto.AuthorName = author.Nickname
		dto.AuthorAvatar = author.AvatarURL
	}
	return dto
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func removeFrom(list []string, value string) []string {
	out := make([]string, 0, len(list))
	for _, item := range list {
		if item != value {
			out = append(out, item)
		}
	}
	return out
}

// toggleInList adds or removes a value and reports whether it is present now.
func toggleInList(list *[]string, value string) bool {
	for i, item := range *list {
		if item == value {
			*list = append((*list)[:i], (*list)[i+1:]...)
			return false
		}
	}
	*list = append(*list, value)
	return true
}
