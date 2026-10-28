package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/hutaomu/hutaomuchat/server/internal/httpx"
	"github.com/hutaomu/hutaomuchat/server/internal/models"
	"github.com/hutaomu/hutaomuchat/server/internal/store"
)

// handleGetMessages returns the message history of a conversation.
func (s *Server) handleGetMessages(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chatID := r.PathValue("chatId")
	if !s.Store.IsChatMember(chatID, user.ID) {
		httpx.WriteError(w, http.StatusForbidden, "你不是该会话成员")
		return
	}
	limit := httpx.Clamp(httpx.ParseInt(r.URL.Query().Get("limit"), 200), 1, 1000)
	messages := s.Store.MessagesOf(chatID, limit)
	out := make([]models.MessageDTO, 0, len(messages))
	for _, message := range messages {
		out = append(out, models.NewMessageDTO(message))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"messages": out})
}

// handleSendMessage stores and delivers a message sent over HTTP.
//
// The Android client delivers messages over the WebSocket; this endpoint
// exists for clients that prefer a plain request/response flow.
func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	chatID := r.PathValue("chatId")
	if !s.Store.IsChatMember(chatID, user.ID) {
		httpx.WriteError(w, http.StatusForbidden, "你不是该会话成员")
		return
	}
	var req struct {
		Content string `json:"content"`
		Type    string `json:"type"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		httpx.WriteError(w, http.StatusBadRequest, "消息内容不能为空")
		return
	}
	messageType := strings.ToUpper(strings.TrimSpace(req.Type))
	if messageType == "" {
		messageType = models.MsgText
	}
	if !validMessageType(messageType) {
		httpx.WriteError(w, http.StatusBadRequest, "不支持的消息类型")
		return
	}

	message := &models.Message{
		ID:         store.NewID(),
		ChatID:     chatID,
		SenderID:   user.ID,
		SenderNick: user.Nickname,
		Content:    truncate(content, 5000),
		Type:       messageType,
		Status:     "SENT",
		Timestamp:  models.Now(),
	}
	s.Store.AddMessage(message)
	s.touchChat(chatID)
	s.broadcastChatMessage(chatID, user.ID, message, false)
	s.Logger.User(user.ID, "SEND_MESSAGE", map[string]any{"chatId": chatID, "messageType": messageType})

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"message": models.NewMessageDTO(message)})
}

// broadcastChatMessage delivers a message to every member of a conversation.
//
// Members that are currently offline receive the message through the offline
// queue and, when available, through the system level push channel.
func (s *Server) broadcastChatMessage(chatID, senderID string, message *models.Message, atAll bool) {
	chat := s.Store.GetChat(chatID)
	members := s.Store.ChatMembers(chatID)
	payload := map[string]any{
		"type":      "message",
		"message":   models.NewMessageDTO(message),
		"chat_name": chatName(chat),
		"is_group":  chat != nil && chat.Type == models.ChatGroup,
	}
	for _, memberID := range members {
		if memberID == senderID {
			continue
		}
		if s.Hub.SendToUser(memberID, payload) > 0 {
			if atAll {
				s.Hub.SendToUser(memberID, map[string]any{
					"type":       "at_all_notification",
					"chatId":     chatID,
					"senderName": message.SenderNick,
					"content":    message.Content,
					"messageId":  message.ID,
				})
			}
			continue
		}
		s.Store.AddToOfflineQueue(memberID, message)
		s.sendPushNotification(memberID, chat, message)
	}
}

// sendPushNotification delivers a system level push notification.
func (s *Server) sendPushNotification(userID string, chat *models.Chat, message *models.Message) {
	if !s.Push.Available() {
		return
	}
	token := s.Store.GetPushToken(userID)
	if token == nil {
		return
	}
	title := message.SenderNick
	body := message.Content
	if chat != nil && chat.Type == models.ChatGroup {
		title = chat.Name
		body = message.SenderNick + ": " + message.Content
	}
	if message.Type != models.MsgText {
		labels := map[string]string{
			models.MsgImage: "[图片]",
			models.MsgVideo: "[视频]",
			models.MsgFile:  "[文件]",
			models.MsgCard:  "[群公告]",
		}
		if label, ok := labels[message.Type]; ok {
			body = label
		}
	}
	if len([]rune(body)) > 100 {
		body = string([]rune(body)[:100])
	}
	go func() {
		if err := s.Push.SendToUsers([]string{token.CID}, title, body, map[string]any{
			"chatId":   message.ChatID,
			"senderId": message.SenderID,
		}); err != nil {
			s.Logger.Warn("推送发送失败", map[string]any{"error": err.Error()})
		}
	}()
}

// touchChat updates the activity timestamp of a conversation.
func (s *Server) touchChat(chatID string) {
	if chat := s.Store.GetChat(chatID); chat != nil {
		chat.UpdatedAt = models.Now()
		s.Store.MarkDirty()
	}
}

// chatName returns the display name of a conversation.
func chatName(chat *models.Chat) string {
	if chat == nil {
		return ""
	}
	return chat.Name
}

// validMessageType reports whether the message type is supported.
func validMessageType(messageType string) bool {
	switch messageType {
	case models.MsgText, models.MsgImage, models.MsgVideo, models.MsgFile, models.MsgCard:
		return true
	default:
		return false
	}
}

// nowMillis returns the current unix timestamp in milliseconds.
func nowMillis() int64 { return time.Now().UnixMilli() }
