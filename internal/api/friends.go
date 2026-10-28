package api

import (
	"net/http"
	"strings"

	"github.com/hutaomu/hutaomuchat/server/internal/httpx"
	"github.com/hutaomu/hutaomuchat/server/internal/models"
	"github.com/hutaomu/hutaomuchat/server/internal/store"
)

// handleListFriends returns the friend list of the authenticated user.
func (s *Server) handleListFriends(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	friends := s.Store.FriendsOf(user.ID)
	out := make([]models.UserDTO, 0, len(friends))
	for _, friend := range friends {
		dto := models.NewUserDTO(friend)
		dto.IsFriend = true
		out = append(out, dto)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"friends": out})
}

// handleSendFriendRequest creates a friend request.
//
// Accounts that disabled the friend verification accept the request right
// away.
func (s *Server) handleSendFriendRequest(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var req struct {
		ToUserID string `json:"to_user_id"`
		Message  string `json:"message"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ToUserID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "缺少 to_user_id")
		return
	}
	if req.ToUserID == user.ID {
		httpx.WriteError(w, http.StatusBadRequest, "不能添加自己为好友")
		return
	}
	target := s.Store.FindUserByID(req.ToUserID)
	if target == nil {
		httpx.WriteError(w, http.StatusNotFound, "用户不存在")
		return
	}
	if s.Store.AreFriends(user.ID, target.ID) {
		httpx.WriteError(w, http.StatusConflict, "你们已经是好友了")
		return
	}
	for _, pending := range s.Store.SentFriendRequests(user.ID) {
		if pending.ToUserID == target.ID {
			httpx.WriteError(w, http.StatusConflict, "好友申请已发送，请等待对方处理")
			return
		}
	}

	if !target.FriendVerification {
		s.Store.AddFriendship(user.ID, target.ID)
		s.Store.AddFriendship(target.ID, user.ID)
		s.ensureSingleChat(user.ID, target.ID)
		s.Logger.User(user.ID, "ADD_FRIEND", map[string]any{"friendId": target.ID, "auto": true})
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "auto_accepted": true})
		return
	}

	request := s.Store.AddFriendRequest(models.FriendRequest{
		ID:         store.NewID(),
		FromUserID: user.ID,
		ToUserID:   target.ID,
		Message:    truncate(strings.TrimSpace(req.Message), 200),
		Status:     "PENDING",
		CreatedAt:  models.Now(),
	})
	s.Logger.User(user.ID, "SEND_FRIEND_REQUEST", map[string]any{"toUserId": target.ID})

	s.Hub.SendToUser(target.ID, map[string]any{
		"type":       "friend_request",
		"fromUserId": user.ID,
		"requestId":  request.ID,
	})

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "request_id": request.ID})
}

// handleFriendRequests returns the pending requests of the authenticated user.
func (s *Server) handleFriendRequests(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	requests := s.Store.PendingFriendRequests(user.ID)
	out := make([]models.FriendRequestDTO, 0, len(requests))
	for _, request := range requests {
		dto := models.FriendRequestDTO{
			ID:         request.ID,
			FromUserID: request.FromUserID,
			ToUserID:   request.ToUserID,
			Message:    request.Message,
			Status:     request.Status,
			CreatedAt:  request.CreatedAt,
		}
		if from := s.Store.FindUserByID(request.FromUserID); from != nil {
			friend := models.NewUserDTO(from)
			dto.FromUser = &friend
		}
		out = append(out, dto)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"requests": out})
}

// handleRespondFriendRequest accepts or rejects a friend request.
func (s *Server) handleRespondFriendRequest(w http.ResponseWriter, r *http.Request) {
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
		httpx.WriteError(w, http.StatusBadRequest, "无效的请求状态")
		return
	}
	request := s.Store.FindFriendRequest(r.PathValue("id"), user.ID)
	if request == nil {
		httpx.WriteError(w, http.StatusNotFound, "申请不存在或已处理")
		return
	}
	if request.Status != "PENDING" {
		httpx.WriteError(w, http.StatusConflict, "申请已处理")
		return
	}
	if !s.Store.SetFriendRequestStatus(request.ID, user.ID, status) {
		httpx.WriteError(w, http.StatusInternalServerError, "处理失败")
		return
	}
	if status == "ACCEPTED" {
		s.Store.AddFriendship(user.ID, request.FromUserID)
		s.Store.AddFriendship(request.FromUserID, user.ID)
		s.ensureSingleChat(user.ID, request.FromUserID)
		s.Logger.User(user.ID, "ADD_FRIEND", map[string]any{"friendId": request.FromUserID})
		s.Hub.SendToUser(request.FromUserID, map[string]any{
			"type":       "friend_request_accepted",
			"fromUserId": user.ID,
		})
	} else {
		s.Hub.SendToUser(request.FromUserID, map[string]any{
			"type":       "friend_request_rejected",
			"fromUserId": user.ID,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleDeleteFriend removes a friend and the direct conversation with them.
func (s *Server) handleDeleteFriend(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	friendID := r.PathValue("friendId")
	if !s.Store.AreFriends(user.ID, friendID) {
		httpx.WriteError(w, http.StatusNotFound, "你们还不是好友")
		return
	}
	s.Store.RemoveFriendship(user.ID, friendID)
	if chat := s.Store.FindSingleChat(user.ID, friendID); chat != nil {
		s.Store.DeleteChat(chat.ID)
	}
	s.Logger.User(user.ID, "DELETE_FRIEND", map[string]any{"friendId": friendID})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// ensureSingleChat returns the direct conversation of two users, creating it
// when it does not exist yet.
func (s *Server) ensureSingleChat(a, b string) *models.Chat {
	if chat := s.Store.FindSingleChat(a, b); chat != nil {
		return chat
	}
	chat := &models.Chat{
		ID:        store.NewID(),
		Type:      models.ChatSingle,
		OwnerID:   a,
		Admins:    []string{},
		MutedBy:   []string{},
		PinnedBy:  []string{},
		CreatedAt: models.Now(),
		UpdatedAt: models.Now(),
	}
	if partner := s.Store.FindUserByID(b); partner != nil {
		chat.Name = partner.Nickname
	}
	s.Store.CreateChat(chat)
	s.Store.AddChatMember(chat.ID, a, models.RoleMember)
	s.Store.AddChatMember(chat.ID, b, models.RoleMember)
	return chat
}
