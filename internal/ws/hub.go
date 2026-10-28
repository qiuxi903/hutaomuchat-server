package ws

import (
	"encoding/json"
	"sync"
	"time"
)

// Client is a single authenticated WebSocket connection.
//
// A user may hold several connections at the same time (phone, tablet, web
// console); every connection is tracked separately.
type Client struct {
	conn   *Conn
	hub    *Hub
	UserID string
	UID    string
	Admin  bool

	sendMu sync.Mutex
	closed bool
}

// Hub keeps track of the connected clients of the server.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*Client]struct{}

	onStatusChange func(userID string, online bool)
}

// NewHub creates an empty hub.
func NewHub() *Hub {
	return &Hub{clients: map[string]map[*Client]struct{}{}}
}

// NewClient creates a client bound to a connection and a hub.
func NewClient(conn *Conn, hub *Hub) *Client {
	return &Client{conn: conn, hub: hub}
}

// OnStatusChange registers a callback invoked whenever a user connects or
// disconnects.
func (h *Hub) OnStatusChange(fn func(userID string, online bool)) {
	h.onStatusChange = fn
}

// Register adds a client to the hub.
func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	if h.clients[c.UserID] == nil {
		h.clients[c.UserID] = map[*Client]struct{}{}
	}
	h.clients[c.UserID][c] = struct{}{}
	h.mu.Unlock()

	if h.onStatusChange != nil {
		h.onStatusChange(c.UserID, true)
	}
}

// Unregister removes a client from the hub.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	conns, ok := h.clients[c.UserID]
	if ok {
		delete(conns, c)
		if len(conns) == 0 {
			delete(h.clients, c.UserID)
		}
	}
	h.mu.Unlock()

	c.sendMu.Lock()
	c.closed = true
	c.sendMu.Unlock()

	if h.onStatusChange != nil && !c.Admin && !h.IsOnline(c.UserID) {
		h.onStatusChange(c.UserID, false)
	}
}

// Conn returns the underlying network connection.
func (c *Client) Conn() *Conn { return c.conn }

// CloseConn closes the underlying network connection.
func (c *Client) CloseConn() error { return c.conn.Close(CloseNormal, "") }

// Send marshals v and writes it to the client.
func (c *Client) Send(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.closed {
		return nil
	}
	return c.conn.WriteText(data)
}

// SendRaw writes an already encoded payload.
func (c *Client) SendRaw(data []byte) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.closed {
		return nil
	}
	return c.conn.WriteText(data)
}

// IsOnline reports whether the user has at least one live connection.
func (h *Hub) IsOnline(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID]) > 0
}

// OnlineCount returns the number of users currently connected.
func (h *Hub) OnlineCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ConnectionCount returns the total number of open connections.
func (h *Hub) ConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	total := 0
	for _, set := range h.clients {
		total += len(set)
	}
	return total
}

// OnlineUsers returns the ids of the connected users.
func (h *Hub) OnlineUsers() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]string, 0, len(h.clients))
	for id := range h.clients {
		out = append(out, id)
	}
	return out
}

// ClientsOf returns the connections of a user.
func (h *Hub) ClientsOf(userID string) []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	set := h.clients[userID]
	out := make([]*Client, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	return out
}

// SendToUser delivers a payload to every connection of a user and returns the
// number of connections reached.
func (h *Hub) SendToUser(userID string, v any) int {
	clients := h.ClientsOf(userID)
	for _, c := range clients {
		_ = c.Send(v)
	}
	return len(clients)
}

// Broadcast delivers a payload to every connected client.
func (h *Hub) Broadcast(v any) int {
	h.mu.RLock()
	all := []*Client{}
	for _, set := range h.clients {
		for c := range set {
			all = append(all, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range all {
		_ = c.Send(v)
	}
	return len(all)
}

// BroadcastUsers delivers a payload to the given users.
func (h *Hub) BroadcastUsers(userIDs []string, v any) int {
	count := 0
	for _, id := range userIDs {
		count += h.SendToUser(id, v)
	}
	return count
}

// BroadcastAdmins delivers a payload to every connected administration
// console.
func (h *Hub) BroadcastAdmins(v any) int {
	h.mu.RLock()
	admins := []*Client{}
	for _, set := range h.clients {
		for c := range set {
			if c.Admin {
				admins = append(admins, c)
			}
		}
	}
	h.mu.RUnlock()
	for _, c := range admins {
		_ = c.Send(v)
	}
	return len(admins)
}

// DisconnectUser closes every connection of a user, e.g. after a ban.
func (h *Hub) DisconnectUser(userID string, reason string) int {
	clients := h.ClientsOf(userID)
	for _, c := range clients {
		_ = c.Send(map[string]any{"type": "force_disconnect", "reason": reason})
		time.Sleep(10 * time.Millisecond)
		_ = c.conn.Close(CloseBanned, reason)
	}
	return len(clients)
}
