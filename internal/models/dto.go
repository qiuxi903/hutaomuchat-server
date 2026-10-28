package models

// This file contains the wire formats returned by the HTTP API. Keeping them
// separate from the persisted entities means internal fields (password hashes,
// moderation flags, ...) can never leak to a client by accident.

// UserDTO is the public representation of a user.
type UserDTO struct {
	ID                    string `json:"id"`
	UID                   string `json:"uid"`
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
	Phone                 string `json:"phone"`
	IsFriend              bool   `json:"is_friend"`
}

// NewUserDTO converts a domain user into its public representation.
func NewUserDTO(u *User) UserDTO {
	if u == nil {
		return UserDTO{}
	}
	return UserDTO{
		ID:                    u.ID,
		UID:                   u.UID,
		Nickname:              u.Nickname,
		Signature:             u.Signature,
		Gender:                u.Gender,
		Region:                u.Region,
		AvatarURL:             u.AvatarURL,
		BackgroundURL:         u.BackgroundURL,
		BackgroundOriginalURL: u.BackgroundOriginalURL,
		FriendVerification:    u.FriendVerification,
		PhoneSearchable:       u.PhoneSearchable,
		Recommendable:         u.Recommendable,
		OnlineStatus:          u.OnlineStatus,
		Phone:                 u.Phone,
	}
}

// ChatDTO is the wire format of a conversation.
type ChatDTO struct {
	ID              string   `json:"id"`
	Type            string   `json:"type"`
	Name            string   `json:"name"`
	AvatarURL       string   `json:"avatar_url"`
	Description     string   `json:"description"`
	Announcement    string   `json:"announcement"`
	GroupAvatar     string   `json:"group_avatar"`
	GroupUID        string   `json:"group_uid"`
	OwnerID         string   `json:"owner_id"`
	Admins          []string `json:"admins"`
	MutedBy         []string `json:"muted_by"`
	PinnedBy        []string `json:"pinned_by"`
	CreatedAt       int64    `json:"created_at"`
	UpdatedAt       int64    `json:"updated_at"`
	MemberCount     int      `json:"member_count"`
	OnlineStatus    string   `json:"online_status"`
	HasAnnouncement bool     `json:"has_announcement"`
}

// NewChatDTO converts a domain chat into its wire format.
func NewChatDTO(c *Chat) ChatDTO {
	if c == nil {
		return ChatDTO{}
	}
	return ChatDTO{
		ID:           c.ID,
		Type:         c.Type,
		Name:         c.Name,
		AvatarURL:    c.AvatarURL,
		Description:  c.Description,
		Announcement: c.Announcement,
		GroupAvatar:  c.GroupAvatar,
		GroupUID:     c.GroupUID,
		OwnerID:      c.OwnerID,
		Admins:       nonNil(c.Admins),
		MutedBy:      nonNil(c.MutedBy),
		PinnedBy:     nonNil(c.PinnedBy),
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
	}
}

// MessageDTO is the wire format of a message.
type MessageDTO struct {
	ID              string `json:"id"`
	ChatID          string `json:"chat_id"`
	SenderID        string `json:"sender_id"`
	SenderNick      string `json:"sender_nickname"`
	Content         string `json:"content"`
	Type            string `json:"type"`
	Status          string `json:"status"`
	Timestamp       int64  `json:"timestamp"`
	AtAll           bool   `json:"at_all"`
	Permanent       bool   `json:"permanent"`
	FileName        string `json:"file_name,omitempty"`
	FileSize        int64  `json:"file_size,omitempty"`
	FileMIME        string `json:"file_mime,omitempty"`
	Duration        int    `json:"duration,omitempty"`
	ClientMessageID string `json:"client_message_id,omitempty"`
}

// NewMessageDTO converts a domain message into its wire format.
func NewMessageDTO(m *Message) MessageDTO {
	if m == nil {
		return MessageDTO{}
	}
	return MessageDTO{
		ID:              m.ID,
		ChatID:          m.ChatID,
		SenderID:        m.SenderID,
		SenderNick:      m.SenderNick,
		Content:         m.Content,
		Type:            m.Type,
		Status:          m.Status,
		Timestamp:       m.Timestamp,
		AtAll:           m.AtAll,
		FileName:        m.FileName,
		FileSize:        m.FileSize,
		FileMIME:        m.FileMIME,
		Duration:        m.Duration,
		ClientMessageID: m.ClientMessageID,
	}
}

// MomentDTO is the wire format of a moment.
type MomentDTO struct {
	ID        string             `json:"id"`
	UserID    string             `json:"user_id"`
	Content   string             `json:"content"`
	Images    ImageList          `json:"images"`
	Scope     string             `json:"scope"`
	Location  string             `json:"location,omitempty"`
	CreatedAt int64              `json:"created_at"`
	User      *UserDTO           `json:"user,omitempty"`
	Likes     []UserDTO          `json:"likes"`
	Comments  []MomentCommentDTO `json:"comments"`
	Liked     bool               `json:"liked"`
}

// MomentCommentDTO is the wire format of a moment comment.
type MomentCommentDTO struct {
	ID        string `json:"id"`
	MomentID  string `json:"moment_id"`
	UserID    string `json:"user_id"`
	Content   string `json:"content"`
	ImageURL  string `json:"image_url"`
	Nickname  string `json:"nickname"`
	CreatedAt int64  `json:"created_at"`
}

// AnnouncementDTO is the wire format of a group announcement.
type AnnouncementDTO struct {
	ID           string `json:"id"`
	ChatID       string `json:"chat_id"`
	Content      string `json:"content"`
	CreatedBy    string `json:"created_by"`
	CreatedAt    int64  `json:"created_at"`
	AtAll        bool   `json:"at_all"`
	AuthorName   string `json:"author_name"`
	AuthorAvatar string `json:"author_avatar"`
}

// StickerDTO is the wire format of a sticker.
type StickerDTO struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	CreatedAt int64  `json:"created_at"`
}

// FriendRequestDTO is the wire format of a friend request.
type FriendRequestDTO struct {
	ID         string   `json:"id"`
	FromUserID string   `json:"from_user_id"`
	ToUserID   string   `json:"to_user_id"`
	Message    string   `json:"message"`
	Status     string   `json:"status"`
	CreatedAt  int64    `json:"created_at"`
	FromUser   *UserDTO `json:"from_user,omitempty"`
}

// JoinRequestDTO is the wire format of a group join request.
type JoinRequestDTO struct {
	ID         string   `json:"id"`
	FromUserID string   `json:"from_user_id"`
	ChatID     string   `json:"chat_id"`
	Message    string   `json:"message"`
	Status     string   `json:"status"`
	CreatedAt  int64    `json:"created_at"`
	FromUser   *UserDTO `json:"from_user,omitempty"`
	ChatName   string   `json:"chat_name"`
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
