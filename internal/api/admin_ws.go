package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/hutaomu/hutaomuchat/server/internal/models"
	"github.com/hutaomu/hutaomuchat/server/internal/security"
	"github.com/hutaomu/hutaomuchat/server/internal/store"
	"github.com/hutaomu/hutaomuchat/server/internal/ws"
)

// adminSocketMessage is the envelope of the administration socket protocol.
type adminSocketMessage struct {
	Type  string `json:"type"`
	Token string `json:"token,omitempty"`
}

// handleAdminWebSocket upgrades a request for the live administration feed.
func (s *Server) handleAdminWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := ws.Upgrade(w, r)
	if err != nil {
		http.Error(w, "WebSocket 升级失败: "+err.Error(), http.StatusBadRequest)
		return
	}

	client := ws.NewClient(conn, s.Hub)
	client.Admin = true
	defer func() {
		if client.UserID != "" {
			s.Hub.Unregister(client)
		}
		_ = client.CloseConn()
	}()

	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	for {
		opcode, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if opcode != ws.OpText {
			continue
		}
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		var msg adminSocketMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			continue
		}
		if msg.Type != "authenticate" {
			continue
		}
		claims, err := security.VerifyToken(s.jwtSecret(), msg.Token)
		if err != nil || claims.Role != "admin" {
			_ = client.Send(map[string]any{"type": "auth_error", "message": "Invalid token"})
			return
		}
		client.UserID = "admin:" + claims.Nickname + ":" + store.NewID()[:8]
		client.UID = claims.Nickname
		s.Hub.Register(client)

		_ = client.Send(map[string]any{
			"type":   "authenticated",
			"online": s.Hub.OnlineCount(),
			"server": s.Config.ServerName(),
		})
		s.pushAdminStats(client)
	}
}

// pushAdminStats sends the current dashboard counters to a console.
func (s *Server) pushAdminStats(client *ws.Client) {
	_ = client.Send(map[string]any{
		"type": "stats",
		"stats": map[string]any{
			"onlineUsers":      s.Hub.OnlineCount(),
			"totalUsers":       s.Store.CountUsers(),
			"totalChats":       len(s.Store.AllChats()),
			"totalMessages":    s.Store.TotalMessages(),
			"totalMoments":     len(s.Store.AllMoments()),
			"offlineQueueSize": s.Store.OfflineQueueSize(),
			"wsConnections":    s.Hub.ConnectionCount(),
			"uptime":           int(time.Since(s.StartedAt).Seconds()),
			"timestamp":        models.Now(),
		},
	})
}

// notifyAdmins pushes an event to every connected administration console.
func (s *Server) notifyAdmins(event string, payload map[string]any) {
	message := map[string]any{"type": "event", "event": event, "timestamp": models.Now()}
	for key, value := range payload {
		message[key] = value
	}
	s.Hub.BroadcastAdmins(message)
}
