// Package models defines the persisted domain entities of the HutaomuChat
// server together with the wire formats (DTOs) exposed by the HTTP API.
//
// Domain entities use snake_case JSON tags so that a snapshot written to disk
// stays human readable and easy to diff between releases.
package models

import (
	"encoding/json"
	"time"
)

// Chat type constants.
const (
	ChatSingle = "SINGLE"
	ChatGroup  = "GROUP"
)

// Chat member roles.
const (
	RoleOwner  = "OWNER"
	RoleAdmin  = "ADMIN"
	RoleMember = "MEMBER"
)

// Online status values.
const (
	Online  = "ONLINE"
	Offline = "OFFLINE"
)

// Request / moment scopes.
const (
	ScopePublic  = "public"
	ScopeFriends = "friends"
)

// Message types.
const (
	MsgText   = "TEXT"
	MsgImage  = "IMAGE"
	MsgVideo  = "VIDEO"
	MsgFile   = "FILE"
	MsgCard   = "CARD"
	MsgSystem = "SYSTEM"
)

// GlobalChatID is the reserved identifier of the built-in public room.
const GlobalChatID = "global-chat-room"

// User is a registered account.
type User struct {
	ID                    string `json:"id"`
	UID                   string `json:"uid"`
	Phone                 string `json:"phone"`
	PasswordHash          string `json:"password_hash"`
	Nickname              string `json:"nickname"`
	Signature             string `json:"signature"`
	Gender                string `json:"gender"`
	Region                string `json:"region"`
	AvatarURL             string `json:"avatar_url"`
	BackgroundURL         string `json:"background_url"`
	BackgroundOriginalURL string `json:"background_original_url"`
	FriendVerification    bool   `json:"friend_verification"`
	PhoneSearchable       bool   `json:"phone_searchable"`
	Recommendable         bool   `json:"recommendable"`
	OnlineStatus          string `json:"online_status"`
	Banned                bool   `json:"banned"`
	Role                  string `json:"role"`
	CreatedAt             int64  `json:"created_at"`
	LastSeenAt            int64  `json:"last_seen_at"`
}

// Friendship is a directed edge between two users; the store keeps both
// directions so that "is a friend of" lookups are O(1).
type Friendship struct {
	UserID    string `json:"user_id"`
	FriendID  string `json:"friend_id"`
	CreatedAt int64  `json:"created_at"`
}

// FriendRequest is a pending or handled friend request.
type FriendRequest struct {
	ID         string `json:"id"`
	FromUserID string `json:"from_user_id"`
	ToUserID   string `json:"to_user_id"`
	Message    string `json:"message"`
	Status     string `json:"status"`
	CreatedAt  int64  `json:"created_at"`
	HandledAt  int64  `json:"handled_at"`
}

// Chat is either a direct conversation or a group.
type Chat struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	Name         string   `json:"name"`
	AvatarURL    string   `json:"avatar_url"`
	Description  string   `json:"description"`
	Announcement string   `json:"announcement"`
	GroupAvatar  string   `json:"group_avatar"`
	GroupUID     string   `json:"group_uid"`
	OwnerID      string   `json:"owner_id"`
	Admins       []string `json:"admins"`
	MutedBy      []string `json:"muted_by"`
	PinnedBy     []string `json:"pinned_by"`
	CreatedAt    int64    `json:"created_at"`
	UpdatedAt    int64    `json:"updated_at"`
}

// ChatMember links a user into a chat with a role.
type ChatMember struct {
	ChatID   string `json:"chat_id"`
	UserID   string `json:"user_id"`
	Role     string `json:"role"`
	JoinedAt int64  `json:"joined_at"`
}

// Message is a single chat message.
type Message struct {
	ID              string `json:"id"`
	ChatID          string `json:"chat_id"`
	SenderID        string `json:"sender_id"`
	SenderNick      string `json:"sender_nickname"`
	Content         string `json:"content"`
	Type            string `json:"type"`
	Status          string `json:"status"`
	Timestamp       int64  `json:"timestamp"`
	AtAll           bool   `json:"at_all"`
	FileName        string `json:"file_name,omitempty"`
	FileSize        int64  `json:"file_size,omitempty"`
	FileMIME        string `json:"file_mime,omitempty"`
	Duration        int    `json:"duration,omitempty"`
	Recalled        bool   `json:"recalled,omitempty"`
	ClientMessageID string `json:"client_message_id,omitempty"`
}

// Moment is a post in the user circle (public) or the friend feed.
type Moment struct {
	ID        string   `json:"id"`
	UserID    string   `json:"user_id"`
	Content   string   `json:"content"`
	Images    []string `json:"images"`
	Scope     string   `json:"scope"`
	Location  string   `json:"location,omitempty"`
	CreatedAt int64    `json:"created_at"`
}

// MomentLike records a like on a moment.
type MomentLike struct {
	MomentID string `json:"moment_id"`
	UserID   string `json:"user_id"`
	LikeAt   int64  `json:"like_at"`
}

// MomentComment is a reply on a moment.
type MomentComment struct {
	ID        string `json:"id"`
	MomentID  string `json:"moment_id"`
	UserID    string `json:"user_id"`
	Content   string `json:"content"`
	ImageURL  string `json:"image_url"`
	Nickname  string `json:"nickname"`
	CreatedAt int64  `json:"created_at"`
}

// Sticker is a user saved sticker image.
type Sticker struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	URL       string `json:"url"`
	CreatedAt int64  `json:"created_at"`
}

// Announcement is a pinned group notice.
type Announcement struct {
	ID        string `json:"id"`
	ChatID    string `json:"chat_id"`
	Content   string `json:"content"`
	CreatedBy string `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
	AtAll     bool   `json:"at_all"`
}

// GroupJoinRequest is a request to join a group.
type GroupJoinRequest struct {
	ID         string `json:"id"`
	FromUserID string `json:"from_user_id"`
	ChatID     string `json:"chat_id"`
	Message    string `json:"message"`
	Status     string `json:"status"`
	CreatedAt  int64  `json:"created_at"`
	HandledAt  int64  `json:"handled_at"`
}

// PushToken is a device push registration (GeTui CID).
type PushToken struct {
	UserID    string `json:"user_id"`
	CID       string `json:"cid"`
	Platform  string `json:"platform"`
	DeviceID  string `json:"device_id,omitempty"`
	UpdatedAt int64  `json:"updated_at"`
}

// AuditLog records an administrative action.
type AuditLog struct {
	ID        string `json:"id"`
	AdminUser string `json:"admin_user"`
	Action    string `json:"action"`
	Target    string `json:"target,omitempty"`
	Detail    string `json:"detail,omitempty"`
	IP        string `json:"ip,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// OfflineItem is a message queued for a disconnected recipient.
type OfflineItem struct {
	ID          string   `json:"id"`
	RecipientID string   `json:"recipient_id"`
	Message     *Message `json:"message"`
	Timestamp   int64    `json:"timestamp"`
}

// RecallItem is a recall event queued for a disconnected recipient.
type RecallItem struct {
	ID          string `json:"id"`
	RecipientID string `json:"recipient_id"`
	ChatID      string `json:"chat_id"`
	MessageID   string `json:"message_id"`
	Timestamp   int64  `json:"timestamp"`
}

// AtAllUsage tracks the daily @所有人 budget of a user.
type AtAllUsage struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// ImageList serialises a list of image URLs as a JSON *string*.
//
// The Android client models the moment image list as a raw string, so the wire
// format has to stay a JSON-encoded string instead of a real array.
type ImageList []string

// MarshalJSON encodes the list as a JSON string.
func (l ImageList) MarshalJSON() ([]byte, error) {
	if l == nil {
		return []byte(`"[]"`), nil
	}
	encoded, err := json.Marshal([]string(l))
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(encoded))
}

// UnmarshalJSON accepts either a JSON string or a JSON array.
func (l *ImageList) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		*l = nil
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		if s == "" {
			*l = nil
			return nil
		}
		var out []string
		if err := json.Unmarshal([]byte(s), &out); err != nil {
			// Tolerate legacy values that were not a JSON array.
			*l = ImageList{s}
			return nil
		}
		*l = out
		return nil
	}
	var out []string
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	*l = out
	return nil
}

// Now returns the current unix timestamp in milliseconds.
func Now() int64 { return time.Now().UnixMilli() }
