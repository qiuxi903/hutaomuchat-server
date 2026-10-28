package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/hutaomu/hutaomuchat/server/internal/models"
	"github.com/hutaomu/hutaomuchat/server/internal/security"
	"github.com/hutaomu/hutaomuchat/server/internal/store"
	"github.com/hutaomu/hutaomuchat/server/internal/ws"
)

// socketMessage is the envelope used by the WebSocket protocol.
type socketMessage struct {
	Type      string          `json:"type"`
	Token     string          `json:"token,omitempty"`
	ChatID    string          `json:"chatId,omitempty"`
	Content   string          `json:"content,omitempty"`
	MsgType   string          `json:"msgType,omitempty"`
	MessageID string          `json:"messageId,omitempty"`
	Status    string          `json:"status,omitempty"`
	ToUserID  string          `json:"toUserId,omitempty"`
	AtAll     bool            `json:"atAll,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// handleWebSocket upgrades a request and runs the client protocol.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !s.Config.Initialized() {
		http.Error(w, "服务器尚未初始化", http.StatusServiceUnavailable)
		return
	}
	conn, err := ws.Upgrade(w, r)
	if err != nil {
		http.Error(w, "WebSocket 升级失败: "+err.Error(), http.StatusBadRequest)
		return
	}

	client := ws.NewClient(conn, s.Hub)
	go s.runClient(client)
}

// runClient reads messages of a client until the connection is closed.
func (s *Server) runClient(client *ws.Client) {
	defer func() {
		if client.UserID != "" {
			s.Hub.Unregister(client)
		}
		_ = client.CloseConn()
	}()

	authenticated := false
	_ = client.Conn().SetReadDeadline(time.Now().Add(90 * time.Second))

	for {
		opcode, payload, err := client.Conn().ReadMessage()
		if err != nil {
			return
		}
		if opcode != ws.OpText {
			continue
		}
		_ = client.Conn().SetReadDeadline(time.Now().Add(90 * time.Second))

		var msg socketMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			_ = client.Send(map[string]any{"type": "error", "message": "Invalid JSON"})
			continue
		}

		if !authenticated {
			if msg.Type != "authenticate" {
				_ = client.Send(map[string]any{"type": "auth_error", "message": "请先进行身份验证"})
				continue
			}
			if !s.authenticateClient(client, msg.Token) {
				return
			}
			authenticated = true
			continue
		}

		s.dispatchSocketMessage(client, msg)
	}
}

// authenticateClient validates the token of a connecting client.
func (s *Server) authenticateClient(client *ws.Client, token string) bool {
	claims, err := security.VerifyToken(s.jwtSecret(), token)
	if err != nil {
		_ = client.Send(map[string]any{"type": "auth_error", "message": "Invalid token"})
		return false
	}
	user := s.Store.FindUserByID(claims.UserID)
	if user == nil {
		_ = client.Send(map[string]any{"type": "auth_error", "message": "用户不存在"})
		return false
	}
	if user.Banned {
		_ = client.Send(map[string]any{"type": "error", "message": "账号已被封禁"})
		_ = client.Conn().Close(ws.CloseBanned, "banned")
		return false
	}

	client.UserID = user.ID
	client.UID = user.UID
	s.Hub.Register(client)

	_ = client.Send(map[string]any{"type": "authenticated", "userId": user.ID})
	s.broadcastOnlineStatus(user.ID, models.Online)

	pending := s.Store.TakePendingMessages(user.ID)
	if len(pending) > 0 {
		items := make([]models.MessageDTO, 0, len(pending))
		for _, message := range pending {
			items = append(items, models.NewMessageDTO(message))
		}
		_ = client.Send(map[string]any{"type": "pending_messages", "messages": items})
	}
	for _, recall := range s.Store.TakePendingRecalls(user.ID) {
		_ = client.Send(map[string]any{
			"type":      "RECALL",
			"chatId":    recall.ChatID,
			"messageId": recall.MessageID,
		})
	}
	return true
}

// dispatchSocketMessage routes an incoming message to its handler.
func (s *Server) dispatchSocketMessage(client *ws.Client, msg socketMessage) {
	switch msg.Type {
	case "authenticate":
		// Already authenticated; ignore a repeated handshake.
	case "message":
		s.handleSocketMessage(client, msg)
	case "RECALL":
		s.handleSocketRecall(client, msg)
	case "typing":
		s.handleSocketTyping(client, msg)
	case "message_ack":
		s.Store.UpdateMessageStatus(msg.MessageID, msg.Status)
	case "friend_request":
		s.Hub.SendToUser(msg.ToUserID, map[string]any{
			"type":       "friend_request",
			"fromUserId": client.UserID,
		})
	case "ping":
		_ = client.Send(map[string]any{"type": "pong", "timestamp": models.Now()})
	default:
		_ = client.Send(map[string]any{"type": "error", "message": "Unknown message type"})
	}
}

// handleSocketMessage stores and delivers a message sent over the socket.
func (s *Server) handleSocketMessage(client *ws.Client, msg socketMessage) {
	if client.UserID == "" {
		return
	}
	chatID := msg.ChatID
	if chatID == "" {
		_ = client.Send(map[string]any{"type": "error", "message": "缺少 chatId"})
		return
	}
	if !s.Store.IsChatMember(chatID, client.UserID) {
		_ = client.Send(map[string]any{"type": "error", "message": "你不是该会话成员"})
		return
	}
	messageType := strings.ToUpper(strings.TrimSpace(msg.MsgType))
	if messageType == "" {
		messageType = models.MsgText
	}
	if !validMessageType(messageType) {
		_ = client.Send(map[string]any{"type": "error", "message": "不支持的消息类型"})
		return
	}
	content := msg.Content
	if messageType == models.MsgText && strings.TrimSpace(content) == "" {
		_ = client.Send(map[string]any{"type": "error", "message": "消息内容不能为空"})
		return
	}

	sender := s.Store.FindUserByID(client.UserID)
	atAll := false
	atAllRemaining := 0
	if messageType == models.MsgText && containsAtAll(content) {
		chat := s.Store.GetChat(chatID)
		if !s.isChatAdmin(client.UserID, chat) {
			content = stripAtAll(content)
			_ = client.Send(map[string]any{"type": "at_all_error", "message": "只有管理员和群主才能@所有人"})
		} else {
			allowed, remaining := s.Store.CheckAtAll(client.UserID)
			if !allowed {
				content = stripAtAll(content)
				_ = client.Send(map[string]any{
					"type":    "at_all_error",
					"message": "今日@所有人次数已用完（每天最多 20 次）",
				})
			} else {
				atAll = true
				atAllRemaining = remaining
			}
		}
	}

	messageID := msg.MessageID
	if messageID == "" {
		messageID = store.NewID()
	}
	message := &models.Message{
		ID:              messageID,
		ChatID:          chatID,
		SenderID:        client.UserID,
		SenderNick:      senderNickname(sender),
		Content:         truncate(content, 5000),
		Type:            messageType,
		Status:          "SENT",
		Timestamp:       models.Now(),
		AtAll:           atAll,
		ClientMessageID: msg.MessageID,
	}
	s.Store.AddMessage(message)
	s.touchChat(chatID)
	s.broadcastChatMessage(chatID, client.UserID, message, atAll)
	s.Logger.User(client.UserID, "SEND_MESSAGE", map[string]any{
		"chatId":      chatID,
		"messageType": messageType,
	})
	s.notifyAdmins("message.sent", map[string]any{
		"chatId":   chatID,
		"senderId": client.UserID,
		"type":     messageType,
	})

	if atAll {
		_ = client.Send(map[string]any{"type": "at_all_info", "remaining": atAllRemaining})
	}
}

// handleSocketRecall removes a message that the sender owns.
func (s *Server) handleSocketRecall(client *ws.Client, msg socketMessage) {
	if client.UserID == "" || msg.ChatID == "" || msg.MessageID == "" {
		return
	}
	original := s.Store.FindMessage(msg.MessageID)
	if original == nil || original.SenderID != client.UserID {
		return
	}
	s.Store.RemoveMessage(msg.MessageID)
	s.Logger.User(client.UserID, "RECALL_MESSAGE", map[string]any{"messageId": msg.MessageID})

	for _, memberID := range s.Store.ChatMembers(msg.ChatID) {
		if memberID == client.UserID {
			continue
		}
		if s.Hub.SendToUser(memberID, map[string]any{
			"type":      "RECALL",
			"chatId":    msg.ChatID,
			"messageId": msg.MessageID,
		}) == 0 {
			s.Store.AddToRecallQueue(memberID, msg.ChatID, msg.MessageID)
		}
	}
}

// handleSocketTyping forwards a typing indicator to the other members.
func (s *Server) handleSocketTyping(client *ws.Client, msg socketMessage) {
	if client.UserID == "" || msg.ChatID == "" {
		return
	}
	for _, memberID := range s.Store.ChatMembers(msg.ChatID) {
		if memberID == client.UserID {
			continue
		}
		s.Hub.SendToUser(memberID, map[string]any{
			"type":   "typing",
			"chatId": msg.ChatID,
			"userId": client.UserID,
		})
	}
}

// broadcastOnlineStatus notifies the friends of a user about their presence.
func (s *Server) broadcastOnlineStatus(userID, status string) {
	s.Store.UpdateUser(userID, func(u *models.User) {
		u.OnlineStatus = status
		if status == models.Offline {
			u.LastSeenAt = models.Now()
		}
	})
	for _, friendID := range s.Store.FriendIDs(userID) {
		s.Hub.SendToUser(friendID, map[string]any{
			"type":   "online_status",
			"userId": userID,
			"status": status,
		})
	}
}

// containsAtAll reports whether the text mentions everybody.
func containsAtAll(content string) bool {
	return strings.Contains(content, "@所有人") || strings.Contains(content, "@全体成员")
}

// stripAtAll removes the everybody mention from a text.
func stripAtAll(content string) string {
	content = strings.ReplaceAll(content, "@所有人", "")
	content = strings.ReplaceAll(content, "@全体成员", "")
	return strings.TrimSpace(content)
}

func senderNickname(user *models.User) string {
	if user == nil {
		return "Unknown"
	}
	return user.Nickname
}
