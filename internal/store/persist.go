package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hutaomu/hutaomuchat/server/internal/models"
)

// snapshot is the on-disk representation of the store.
type snapshot struct {
	Version         int                               `json:"version"`
	SavedAt         int64                             `json:"saved_at"`
	UIDCounter      int                               `json:"uid_counter"`
	GroupUIDCounter int                               `json:"group_uid_counter"`
	Users           []*models.User                    `json:"users"`
	Friendships     []models.Friendship               `json:"friendships"`
	FriendRequests  []models.FriendRequest            `json:"friend_requests"`
	Chats           []*models.Chat                    `json:"chats"`
	ChatMembers     []models.ChatMember               `json:"chat_members"`
	Messages        map[string][]*models.Message      `json:"messages"`
	Moments         []*models.Moment                  `json:"moments"`
	MomentLikes     map[string]map[string]int64       `json:"moment_likes"`
	MomentComments  map[string][]models.MomentComment `json:"moment_comments"`
	Stickers        []*models.Sticker                 `json:"stickers"`
	Announcements   []*models.Announcement            `json:"announcements"`
	JoinRequests    []*models.GroupJoinRequest        `json:"group_join_requests"`
	PushTokens      []*models.PushToken               `json:"push_tokens"`
	AtAllUsage      map[string]*models.AtAllUsage     `json:"at_all_usage"`
	PermanentFiles  map[string]string                 `json:"permanent_files"`
	AuditLogs       []*models.AuditLog                `json:"audit_logs"`
	OfflineQueue    []models.OfflineItem              `json:"offline_queue"`
	RecallQueue     []models.RecallItem               `json:"recall_queue"`
}

// Persister writes and restores store snapshots.
type Persister struct {
	path string
	mu   sync.Mutex
}

// NewPersister creates a persister targeting the given file.
func NewPersister(path string) *Persister { return &Persister{path: path} }

// Path returns the snapshot file path.
func (p *Persister) Path() string { return p.path }

// Save writes the current state of the store to disk atomically.
func (p *Persister) Save(s *Store) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	s.mu.RLock()
	snap := snapshot{
		Version:         1,
		SavedAt:         models.Now(),
		UIDCounter:      s.uidCounter,
		GroupUIDCounter: s.groupUIDCounter,
		Users:           make([]*models.User, 0, len(s.userOrder)),
		Messages:        map[string][]*models.Message{},
		MomentLikes:     map[string]map[string]int64{},
		MomentComments:  map[string][]models.MomentComment{},
		AtAllUsage:      map[string]*models.AtAllUsage{},
		PermanentFiles:  map[string]string{},
	}
	for _, id := range s.userOrder {
		if u := s.users[id]; u != nil {
			snap.Users = append(snap.Users, u)
		}
	}
	snap.Friendships = append(snap.Friendships, s.friendships...)
	snap.FriendRequests = append(snap.FriendRequests, s.friendRequests...)
	for _, id := range s.chatOrder {
		if c := s.chats[id]; c != nil {
			snap.Chats = append(snap.Chats, c)
		}
		for _, uid := range s.members[id] {
			snap.ChatMembers = append(snap.ChatMembers, models.ChatMember{
				ChatID:   id,
				UserID:   uid,
				Role:     s.memberRole[id][uid],
				JoinedAt: models.Now(),
			})
		}
	}
	for chatID, list := range s.messages {
		snap.Messages[chatID] = list
	}
	snap.Moments = append(snap.Moments, s.moments...)
	for mid, likes := range s.momentLikes {
		snap.MomentLikes[mid] = likes
	}
	for mid, comments := range s.momentComments {
		snap.MomentComments[mid] = comments
	}
	snap.Stickers = append(snap.Stickers, s.stickers...)
	snap.Announcements = append(snap.Announcements, s.announcements...)
	snap.JoinRequests = append(snap.JoinRequests, s.groupJoinRequest...)
	for _, t := range s.pushTokens {
		snap.PushTokens = append(snap.PushTokens, t)
	}
	for uid, usage := range s.atAllUsage {
		snap.AtAllUsage[uid] = usage
	}
	for fileID, chatID := range s.permanentFiles {
		snap.PermanentFiles[fileID] = chatID
	}
	snap.AuditLogs = append(snap.AuditLogs, s.auditLogs...)
	snap.OfflineQueue = append(snap.OfflineQueue, s.offlineQueue...)
	snap.RecallQueue = append(snap.RecallQueue, s.recallQueue...)
	s.mu.RUnlock()

	data, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}
	// Keep the previous snapshot as a backup so that a damaged file can never
	// wipe out the whole dataset.
	if _, err := os.Stat(p.path); err == nil {
		_ = copyFile(p.path, p.path+".bak")
	}
	if err := os.Rename(tmp, p.path); err != nil {
		return fmt.Errorf("commit snapshot: %w", err)
	}
	// Make sure a backup exists even after the very first save.
	if _, err := os.Stat(p.path + ".bak"); os.IsNotExist(err) {
		_ = copyFile(p.path, p.path+".bak")
	}
	return nil
}

// BackupPath returns the location of the backup snapshot.
func (p *Persister) BackupPath() string { return p.path + ".bak" }

// copyFile copies a file, creating the destination directory if needed.
func copyFile(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destination, data, 0o600)
}

// Load restores a previously saved snapshot into the store.
func (p *Persister) Load(s *Store) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := os.ReadFile(p.path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read snapshot: %w", err)
	}
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		// The primary snapshot is damaged; fall back to the backup copy so
		// that the database is not silently emptied.
		backup, backupErr := os.ReadFile(p.path + ".bak")
		if backupErr != nil {
			return false, fmt.Errorf("decode snapshot: %w", err)
		}
		if backupErr := json.Unmarshal(backup, &snap); backupErr != nil {
			return false, fmt.Errorf("decode snapshot: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.users = map[string]*models.User{}
	s.usersByPhone = map[string]*models.User{}
	s.usersByUID = map[string]*models.User{}
	s.userOrder = nil
	s.friendSet = map[string]map[string]struct{}{}
	s.chats = map[string]*models.Chat{}
	s.chatOrder = nil
	s.members = map[string][]string{}
	s.memberRole = map[string]map[string]string{}
	s.messages = map[string][]*models.Message{}
	s.momentIndex = map[string]*models.Moment{}
	s.momentLikes = map[string]map[string]int64{}
	s.momentComments = map[string][]models.MomentComment{}
	s.pushTokens = map[string]*models.PushToken{}
	s.atAllUsage = map[string]*models.AtAllUsage{}
	s.permanentFiles = map[string]string{}

	for _, u := range snap.Users {
		if u == nil || u.ID == "" {
			continue
		}
		s.users[u.ID] = u
		s.usersByPhone[u.Phone] = u
		s.usersByUID[u.UID] = u
		s.userOrder = append(s.userOrder, u.ID)
	}
	s.uidCounter = snap.UIDCounter
	s.groupUIDCounter = snap.GroupUIDCounter

	s.friendships = snap.Friendships
	for _, f := range snap.Friendships {
		if s.friendSet[f.UserID] == nil {
			s.friendSet[f.UserID] = map[string]struct{}{}
		}
		if s.friendSet[f.FriendID] == nil {
			s.friendSet[f.FriendID] = map[string]struct{}{}
		}
		s.friendSet[f.UserID][f.FriendID] = struct{}{}
		s.friendSet[f.FriendID][f.UserID] = struct{}{}
	}
	s.friendRequests = snap.FriendRequests

	for _, c := range snap.Chats {
		if c == nil || c.ID == "" {
			continue
		}
		s.chats[c.ID] = c
		s.chatOrder = append(s.chatOrder, c.ID)
	}
	for _, m := range snap.ChatMembers {
		s.members[m.ChatID] = append(s.members[m.ChatID], m.UserID)
		if s.memberRole[m.ChatID] == nil {
			s.memberRole[m.ChatID] = map[string]string{}
		}
		s.memberRole[m.ChatID][m.UserID] = m.Role
	}
	for chatID, list := range snap.Messages {
		s.messages[chatID] = list
	}
	s.moments = snap.Moments
	for _, m := range snap.Moments {
		if m != nil {
			s.momentIndex[m.ID] = m
		}
	}
	s.momentLikes = snap.MomentLikes
	s.momentComments = snap.MomentComments
	s.stickers = snap.Stickers
	s.announcements = snap.Announcements
	s.groupJoinRequest = snap.JoinRequests
	for _, t := range snap.PushTokens {
		if t != nil {
			s.pushTokens[t.UserID] = t
		}
	}
	s.atAllUsage = snap.AtAllUsage
	s.permanentFiles = snap.PermanentFiles
	if s.permanentFiles == nil {
		s.permanentFiles = map[string]string{}
	}
	s.auditLogs = snap.AuditLogs
	s.offlineQueue = snap.OfflineQueue
	s.recallQueue = snap.RecallQueue

	// Guarantee the public room always exists.
	s.getOrCreateGlobalChat()
	s.dirty = false
	return true, nil
}

// StartAutoSave writes a snapshot whenever the store is marked dirty and also
// on a fixed interval so that no change is ever lost for more than a minute.
func (p *Persister) StartAutoSave(s *Store, interval time.Duration, onError func(error)) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.DirtyChan():
				// Coalesce bursts of writes.
				time.Sleep(300 * time.Millisecond)
				if err := p.Save(s); err != nil && onError != nil {
					onError(err)
				}
			case <-ticker.C:
				if err := p.Save(s); err != nil && onError != nil {
					onError(err)
				}
			}
		}
	}()
}
