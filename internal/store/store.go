// Package store implements the in-memory data store of the server.
//
// All entities live in memory behind a single RWMutex and are indexed by the
// identifiers the API actually queries by (user id, phone, uid, chat id). A
// background goroutine periodically writes an atomic JSON snapshot to disk and
// the same snapshot is written on shutdown.
package store

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/hutaomu/hutaomuchat/server/internal/models"
)

// MaxMessagesPerChat bounds the amount of history kept in memory per chat.
const MaxMessagesPerChat = 5000

// MaxOfflineQueue bounds the per-server offline delivery queue.
const MaxOfflineQueue = 20000

// Store is the central data store.
type Store struct {
	mu sync.RWMutex

	users        map[string]*models.User
	usersByPhone map[string]*models.User
	usersByUID   map[string]*models.User
	userOrder    []string

	friendships    []models.Friendship
	friendSet      map[string]map[string]struct{}
	friendRequests []models.FriendRequest

	chats      map[string]*models.Chat
	chatOrder  []string
	members    map[string][]string
	memberRole map[string]map[string]string

	messages map[string][]*models.Message

	moments        []*models.Moment
	momentIndex    map[string]*models.Moment
	momentLikes    map[string]map[string]int64
	momentComments map[string][]models.MomentComment

	stickers         []*models.Sticker
	announcements    []*models.Announcement
	groupJoinRequest []*models.GroupJoinRequest
	pushTokens       map[string]*models.PushToken
	atAllUsage       map[string]*models.AtAllUsage
	auditLogs        []*models.AuditLog
	permanentFiles   map[string]string

	groupUIDCounter int
	uidCounter      int

	offlineQueue []models.OfflineItem
	recallQueue  []models.RecallItem

	dirty   bool
	dirtyCh chan struct{}
}

// New creates an empty store.
func New() *Store {
	s := &Store{
		users:          map[string]*models.User{},
		usersByPhone:   map[string]*models.User{},
		usersByUID:     map[string]*models.User{},
		friendSet:      map[string]map[string]struct{}{},
		chats:          map[string]*models.Chat{},
		members:        map[string][]string{},
		memberRole:     map[string]map[string]string{},
		messages:       map[string][]*models.Message{},
		momentIndex:    map[string]*models.Moment{},
		momentLikes:    map[string]map[string]int64{},
		momentComments: map[string][]models.MomentComment{},
		pushTokens:     map[string]*models.PushToken{},
		atAllUsage:     map[string]*models.AtAllUsage{},
		permanentFiles: map[string]string{},
		dirtyCh:        make(chan struct{}, 1),
	}
	s.getOrCreateGlobalChat()
	return s
}

// MarkDirty flags the store as needing a snapshot. Non blocking.
func (s *Store) MarkDirty() {
	s.mu.Lock()
	s.dirty = true
	s.mu.Unlock()
	select {
	case s.dirtyCh <- struct{}{}:
	default:
	}
}

// DirtyChan exposes the notification channel used by the snapshot writer.
func (s *Store) DirtyChan() <-chan struct{} { return s.dirtyCh }

// ---------------------------------------------------------------- users ----

// NextUID reserves the next public account number (htm-001, htm-002, ...).
func (s *Store) NextUID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uidCounter++
	return fmt.Sprintf("htm-%03d", s.uidCounter)
}

// NextGroupUID reserves the next group number (htmq-001, htmq-002, ...).
func (s *Store) NextGroupUID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.groupUIDCounter++
	return fmt.Sprintf("htmq-%03d", s.groupUIDCounter)
}

// CreateUser registers a new account and indexes it.
func (s *Store) CreateUser(u *models.User) *models.User {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[u.ID] = u
	s.usersByPhone[u.Phone] = u
	s.usersByUID[u.UID] = u
	s.userOrder = append(s.userOrder, u.ID)
	if n := parseNumericSuffix(u.UID); n > s.uidCounter {
		s.uidCounter = n
	}
	s.markDirtyLocked()
	return u
}

// FindUserByID returns the user with the given id, or nil.
func (s *Store) FindUserByID(id string) *models.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.users[id]
}

// FindUserByPhone returns the user registered with the phone number, or nil.
func (s *Store) FindUserByPhone(phone string) *models.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.usersByPhone[phone]
}

// FindUserByUID returns the user with the given public id, or nil.
func (s *Store) FindUserByUID(uid string) *models.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.usersByUID[uid]
}

// UpdateUser applies the given fields to a user and returns the updated copy.
func (s *Store) UpdateUser(id string, update func(*models.User)) *models.User {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return nil
	}
	update(u)
	s.markDirtyLocked()
	return u
}

// DeleteUser removes a user and every record that references them.
func (s *Store) DeleteUser(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return false
	}
	delete(s.users, id)
	delete(s.usersByPhone, u.Phone)
	delete(s.usersByUID, u.UID)
	for i, uid := range s.userOrder {
		if uid == id {
			s.userOrder = append(s.userOrder[:i], s.userOrder[i+1:]...)
			break
		}
	}
	s.friendships = filterFriendships(s.friendships, id)
	delete(s.friendSet, id)
	for fid := range s.friendSet {
		delete(s.friendSet[fid], id)
	}
	s.friendRequests = filterFriendRequests(s.friendRequests, id)
	for chatID, members := range s.members {
		s.members[chatID] = removeString(members, id)
		if roles := s.memberRole[chatID]; roles != nil {
			delete(roles, id)
		}
	}
	for chatID, list := range s.messages {
		kept := list[:0]
		for _, m := range list {
			if m.SenderID != id {
				kept = append(kept, m)
			}
		}
		s.messages[chatID] = kept
	}
	kept := s.moments[:0]
	for _, m := range s.moments {
		if m.UserID != id {
			kept = append(kept, m)
		}
	}
	s.moments = kept
	for mid := range s.momentIndex {
		if m := s.momentIndex[mid]; m != nil && m.UserID == id {
			delete(s.momentIndex, mid)
		}
	}
	for mid, likes := range s.momentLikes {
		delete(likes, id)
		if len(likes) == 0 {
			delete(s.momentLikes, mid)
		}
	}
	for mid, list := range s.momentComments {
		kept := list[:0]
		for _, c := range list {
			if c.UserID != id {
				kept = append(kept, c)
			}
		}
		s.momentComments[mid] = kept
	}
	s.stickers = filterStickers(s.stickers, id)
	delete(s.pushTokens, id)
	delete(s.atAllUsage, id)
	s.auditLogs = filterAuditLogs(s.auditLogs, id)
	s.markDirtyLocked()
	return true
}

// ListUsers returns users ordered by creation time, newest first.
func (s *Store) ListUsers() []*models.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*models.User, 0, len(s.userOrder))
	for _, id := range s.userOrder {
		if u := s.users[id]; u != nil {
			out = append(out, u)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// CountUsers returns the number of registered accounts.
func (s *Store) CountUsers() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users)
}

// SearchUsers finds users by uid, phone or nickname substring.
func (s *Store) SearchUsers(query string, excludeID string) []*models.User {
	q := strings.ToLower(strings.TrimSpace(query))
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*models.User{}
	for _, id := range s.userOrder {
		u := s.users[id]
		if u == nil || u.ID == excludeID {
			continue
		}
		if q == "" ||
			strings.Contains(strings.ToLower(u.Nickname), q) ||
			strings.Contains(strings.ToLower(u.UID), q) ||
			strings.Contains(u.Phone, q) {
			out = append(out, u)
		}
	}
	return out
}

// RecommendedUsers returns up to limit random users that opted in for
// recommendations, excluding the given ids.
func (s *Store) RecommendedUsers(excludeIDs []string, limit int) []*models.User {
	excluded := map[string]struct{}{}
	for _, id := range excludeIDs {
		excluded[id] = struct{}{}
	}
	s.mu.RLock()
	candidates := []*models.User{}
	for _, id := range s.userOrder {
		u := s.users[id]
		if u == nil || !u.Recommendable {
			continue
		}
		if _, skip := excluded[u.ID]; skip {
			continue
		}
		candidates = append(candidates, u)
	}
	s.mu.RUnlock()
	shuffle(candidates)
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates
}

// ------------------------------------------------------------- friends ----

// AddFriendship creates the bidirectional friendship edge.
func (s *Store) AddFriendship(a, b string) {
	if a == b {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.friendSet[a] != nil {
		if _, ok := s.friendSet[a][b]; ok {
			return
		}
	}
	s.friendships = append(s.friendships, models.Friendship{UserID: a, FriendID: b, CreatedAt: models.Now()})
	if s.friendSet[a] == nil {
		s.friendSet[a] = map[string]struct{}{}
	}
	if s.friendSet[b] == nil {
		s.friendSet[b] = map[string]struct{}{}
	}
	s.friendSet[a][b] = struct{}{}
	s.friendSet[b][a] = struct{}{}
	s.markDirtyLocked()
}

// AreFriends reports whether the two users are friends.
func (s *Store) AreFriends(a, b string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	set := s.friendSet[a]
	if set == nil {
		return false
	}
	_, ok := set[b]
	return ok
}

// FriendIDs returns the ids of all friends of a user.
func (s *Store) FriendIDs(userID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []string{}
	for fid := range s.friendSet[userID] {
		out = append(out, fid)
	}
	return out
}

// FriendsOf returns the friend users of a user.
func (s *Store) FriendsOf(userID string) []*models.User {
	ids := s.FriendIDs(userID)
	out := make([]*models.User, 0, len(ids))
	for _, id := range ids {
		if u := s.FindUserByID(id); u != nil {
			out = append(out, u)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out
}

// RemoveFriendship deletes the friendship between two users.
func (s *Store) RemoveFriendship(a, b string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.friendships = filterFriendships(s.friendships, a, b)
	delete(s.friendSet[a], b)
	delete(s.friendSet[b], a)
	s.markDirtyLocked()
}

// AddFriendRequest stores a pending friend request.
func (s *Store) AddFriendRequest(r models.FriendRequest) *models.FriendRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.friendRequests = append(s.friendRequests, r)
	s.markDirtyLocked()
	return &s.friendRequests[len(s.friendRequests)-1]
}

// FindFriendRequest returns a friend request addressed to toUserID.
func (s *Store) FindFriendRequest(id, toUserID string) *models.FriendRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.friendRequests {
		if s.friendRequests[i].ID == id && s.friendRequests[i].ToUserID == toUserID {
			return &s.friendRequests[i]
		}
	}
	return nil
}

// PendingFriendRequests returns the pending requests addressed to a user.
func (s *Store) PendingFriendRequests(toUserID string) []models.FriendRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []models.FriendRequest{}
	for _, r := range s.friendRequests {
		if r.ToUserID == toUserID && r.Status == "PENDING" {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// SentFriendRequests returns the pending requests sent by a user.
func (s *Store) SentFriendRequests(fromUserID string) []models.FriendRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []models.FriendRequest{}
	for _, r := range s.friendRequests {
		if r.FromUserID == fromUserID && r.Status == "PENDING" {
			out = append(out, r)
		}
	}
	return out
}

// SetFriendRequestStatus marks a request accepted or rejected.
func (s *Store) SetFriendRequestStatus(id, toUserID, status string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.friendRequests {
		if s.friendRequests[i].ID == id && s.friendRequests[i].ToUserID == toUserID {
			s.friendRequests[i].Status = status
			s.friendRequests[i].HandledAt = models.Now()
			s.markDirtyLocked()
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- chats ----

// CreateChat stores a new conversation.
func (s *Store) CreateChat(c *models.Chat) *models.Chat {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chats[c.ID] = c
	s.chatOrder = append(s.chatOrder, c.ID)
	s.markDirtyLocked()
	return c
}

// GetChat returns a conversation by id, or nil.
func (s *Store) GetChat(id string) *models.Chat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.chats[id]
}

// UpdateChat mutates a conversation in place.
func (s *Store) UpdateChat(id string, update func(*models.Chat)) *models.Chat {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.chats[id]
	if !ok {
		return nil
	}
	update(c)
	s.markDirtyLocked()
	return c
}

// DeleteChat removes a conversation together with its members and messages.
func (s *Store) DeleteChat(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.chats[id]; !ok {
		return false
	}
	delete(s.chats, id)
	delete(s.members, id)
	delete(s.memberRole, id)
	delete(s.messages, id)
	for i, cid := range s.chatOrder {
		if cid == id {
			s.chatOrder = append(s.chatOrder[:i], s.chatOrder[i+1:]...)
			break
		}
	}
	s.markDirtyLocked()
	return true
}

// AllChats returns every conversation in creation order.
func (s *Store) AllChats() []*models.Chat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*models.Chat, 0, len(s.chatOrder))
	for _, id := range s.chatOrder {
		if c := s.chats[id]; c != nil {
			out = append(out, c)
		}
	}
	return out
}

// ChatsForUser returns every conversation the user participates in.
func (s *Store) ChatsForUser(userID string) []*models.Chat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*models.Chat{}
	for _, id := range s.chatOrder {
		c := s.chats[id]
		if c == nil {
			continue
		}
		for _, m := range s.members[id] {
			if m == userID {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// AddChatMember adds a member with the given role.
func (s *Store) AddChatMember(chatID, userID, role string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addMemberLocked(chatID, userID, role)
	s.markDirtyLocked()
}

func (s *Store) addMemberLocked(chatID, userID, role string) {
	for _, m := range s.members[chatID] {
		if m == userID {
			return
		}
	}
	s.members[chatID] = append(s.members[chatID], userID)
	if s.memberRole[chatID] == nil {
		s.memberRole[chatID] = map[string]string{}
	}
	s.memberRole[chatID][userID] = role
}

// RemoveChatMember removes a member from a conversation.
func (s *Store) RemoveChatMember(chatID, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.members[chatID] = removeString(s.members[chatID], userID)
	if roles := s.memberRole[chatID]; roles != nil {
		delete(roles, userID)
	}
	s.markDirtyLocked()
}

// ChatMembers returns the member ids of a conversation.
func (s *Store) ChatMembers(chatID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string{}, s.members[chatID]...)
}

// ChatMemberRole returns the role of a member inside a conversation.
func (s *Store) ChatMemberRole(chatID, userID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if roles := s.memberRole[chatID]; roles != nil {
		return roles[userID]
	}
	return ""
}

// IsChatMember reports whether the user belongs to the conversation.
func (s *Store) IsChatMember(chatID, userID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.members[chatID] {
		if m == userID {
			return true
		}
	}
	return false
}

// SetChatMemberRole updates the role of a member.
func (s *Store) SetChatMemberRole(chatID, userID, role string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.memberRole[chatID] == nil {
		s.memberRole[chatID] = map[string]string{}
	}
	s.memberRole[chatID][userID] = role
	s.markDirtyLocked()
}

// FindSingleChat returns the existing direct conversation between two users.
func (s *Store) FindSingleChat(a, b string) *models.Chat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, id := range s.chatOrder {
		c := s.chats[id]
		if c == nil || c.Type != models.ChatSingle {
			continue
		}
		members := s.members[id]
		if len(members) != 2 {
			continue
		}
		if (members[0] == a && members[1] == b) || (members[0] == b && members[1] == a) {
			return c
		}
	}
	return nil
}

// GetOrCreateGlobalChat returns the built-in public room, creating it if needed.
func (s *Store) GetOrCreateGlobalChat() *models.Chat {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getOrCreateGlobalChat()
}

func (s *Store) getOrCreateGlobalChat() *models.Chat {
	if c, ok := s.chats[models.GlobalChatID]; ok {
		return c
	}
	c := &models.Chat{
		ID:           models.GlobalChatID,
		Type:         models.ChatGroup,
		Name:         "用户圈",
		Description:  "所有用户的公共交流空间",
		Announcement: "用户圈始终置顶在聊天列表中。长按可隐藏用户圈，在发现页面可重新打开。",
		OwnerID:      "system",
		Admins:       []string{},
		MutedBy:      []string{},
		PinnedBy:     []string{},
		CreatedAt:    models.Now(),
	}
	s.chats[c.ID] = c
	s.chatOrder = append(s.chatOrder, c.ID)
	s.markDirtyLocked()
	return c
}

// AutoJoinGlobalChat makes sure a user is a member of the public room.
func (s *Store) AutoJoinGlobalChat(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getOrCreateGlobalChat()
	s.addMemberLocked(models.GlobalChatID, userID, models.RoleMember)
	s.markDirtyLocked()
}

// ------------------------------------------------------------- messages ----

// AddMessage stores a message and trims the per-chat history.
func (s *Store) AddMessage(m *models.Message) *models.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addMessageLocked(m)
	s.markDirtyLocked()
	return m
}

func (s *Store) addMessageLocked(m *models.Message) {
	list := append(s.messages[m.ChatID], m)
	if len(list) > MaxMessagesPerChat {
		list = list[len(list)-MaxMessagesPerChat:]
	}
	s.messages[m.ChatID] = list
}

// MessagesOf returns the most recent limit messages of a chat, oldest first.
func (s *Store) MessagesOf(chatID string, limit int) []*models.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := s.messages[chatID]
	if limit <= 0 || limit > len(list) {
		limit = len(list)
	}
	out := make([]*models.Message, 0, limit)
	for _, m := range list[len(list)-limit:] {
		out = append(out, m)
	}
	return out
}

// CountMessages returns the number of stored messages of a chat.
func (s *Store) CountMessages(chatID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.messages[chatID])
}

// TotalMessages returns the number of stored messages across all chats.
func (s *Store) TotalMessages() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := 0
	for _, list := range s.messages {
		total += len(list)
	}
	return total
}

// FindMessage looks a message up by id.
func (s *Store) FindMessage(id string) *models.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, list := range s.messages {
		for _, m := range list {
			if m.ID == id {
				return m
			}
		}
	}
	return nil
}

// UpdateMessageStatus changes the delivery status of a message.
func (s *Store) UpdateMessageStatus(id, status string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, list := range s.messages {
		for _, m := range list {
			if m.ID == id {
				m.Status = status
				s.markDirtyLocked()
				return true
			}
		}
	}
	return false
}

// RemoveMessage deletes a message from the store.
func (s *Store) RemoveMessage(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for chatID, list := range s.messages {
		for i, m := range list {
			if m.ID == id {
				s.messages[chatID] = append(list[:i], list[i+1:]...)
				s.markDirtyLocked()
				return true
			}
		}
	}
	return false
}

// MessagesSince returns messages of a chat newer than the given timestamp.
func (s *Store) MessagesSince(chatID string, since int64) []*models.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*models.Message{}
	for _, m := range s.messages[chatID] {
		if m.Timestamp > since {
			out = append(out, m)
		}
	}
	return out
}

// -------------------------------------------------------------- moments ----

// AddMoment stores a moment.
func (s *Store) AddMoment(m *models.Moment) *models.Moment {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.moments = append(s.moments, m)
	s.momentIndex[m.ID] = m
	s.markDirtyLocked()
	return m
}

// FindMoment returns a moment by id.
func (s *Store) FindMoment(id string) *models.Moment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.momentIndex[id]
}

// RemoveMoment deletes a moment together with its likes and comments.
func (s *Store) RemoveMoment(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, m := range s.moments {
		if m.ID == id {
			s.moments = append(s.moments[:i], s.moments[i+1:]...)
			delete(s.momentIndex, id)
			delete(s.momentLikes, id)
			delete(s.momentComments, id)
			s.markDirtyLocked()
			return true
		}
	}
	return false
}

// PublicMoments returns the moments published in the user circle.
func (s *Store) PublicMoments() []*models.Moment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*models.Moment{}
	for _, m := range s.moments {
		if m.Scope == models.ScopePublic {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// FriendMoments returns the moments visible to the friend feed of a user.
func (s *Store) FriendMoments(userID string) []*models.Moment {
	friends := map[string]struct{}{}
	s.mu.RLock()
	for fid := range s.friendSet[userID] {
		friends[fid] = struct{}{}
	}
	out := []*models.Moment{}
	for _, m := range s.moments {
		if m.Scope != models.ScopeFriends {
			continue
		}
		if m.UserID == userID {
			out = append(out, m)
			continue
		}
		if _, ok := friends[m.UserID]; ok {
			out = append(out, m)
		}
	}
	s.mu.RUnlock()
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// AllMoments returns every moment, newest first.
func (s *Store) AllMoments() []*models.Moment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]*models.Moment{}, s.moments...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// LikeMoment toggles a like and reports whether the moment is now liked.
func (s *Store) LikeMoment(momentID, userID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.momentLikes[momentID] == nil {
		s.momentLikes[momentID] = map[string]int64{}
	}
	if _, ok := s.momentLikes[momentID][userID]; ok {
		delete(s.momentLikes[momentID], userID)
		s.markDirtyLocked()
		return false
	}
	s.momentLikes[momentID][userID] = models.Now()
	s.markDirtyLocked()
	return true
}

// IsLiked reports whether the user likes the moment.
func (s *Store) IsLiked(momentID, userID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	likes := s.momentLikes[momentID]
	if likes == nil {
		return false
	}
	_, ok := likes[userID]
	return ok
}

// MomentLikes returns the user ids that liked the moment.
func (s *Store) MomentLikes(momentID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []string{}
	for uid := range s.momentLikes[momentID] {
		out = append(out, uid)
	}
	return out
}

// AddMomentComment appends a comment to a moment.
func (s *Store) AddMomentComment(c *models.MomentComment) *models.MomentComment {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.momentComments[c.MomentID] = append(s.momentComments[c.MomentID], *c)
	s.markDirtyLocked()
	return c
}

// MomentComments returns the comments of a moment.
func (s *Store) MomentComments(momentID string) []models.MomentComment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]models.MomentComment{}, s.momentComments[momentID]...)
}

// -------------------------------------------------------------- stickers ----

// AddSticker stores a sticker for a user.
func (s *Store) AddSticker(st models.Sticker) *models.Sticker {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stickers = append(s.stickers, &st)
	s.markDirtyLocked()
	return &st
}

// UserStickers returns the stickers of a user.
func (s *Store) UserStickers(userID string) []models.Sticker {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []models.Sticker{}
	for _, st := range s.stickers {
		if st.UserID == userID {
			out = append(out, *st)
		}
	}
	return out
}

// DeleteSticker removes a sticker owned by the user.
func (s *Store) DeleteSticker(id, userID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, st := range s.stickers {
		if st.ID == id && st.UserID == userID {
			s.stickers = append(s.stickers[:i], s.stickers[i+1:]...)
			s.markDirtyLocked()
			return true
		}
	}
	return false
}

// --------------------------------------------------------- announcements ----

// AddAnnouncement stores a group announcement.
func (s *Store) AddAnnouncement(a models.Announcement) *models.Announcement {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.announcements = append(s.announcements, &a)
	s.markDirtyLocked()
	return &a
}

// AnnouncementsOf returns the announcements of a chat, newest first.
func (s *Store) AnnouncementsOf(chatID string) []models.Announcement {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []models.Announcement{}
	for _, a := range s.announcements {
		if a.ChatID == chatID {
			out = append(out, *a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// AllAnnouncements returns every announcement, newest first.
func (s *Store) AllAnnouncements() []models.Announcement {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.Announcement, 0, len(s.announcements))
	for _, a := range s.announcements {
		out = append(out, *a)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// RemoveAnnouncement deletes an announcement.
func (s *Store) RemoveAnnouncement(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.announcements {
		if a.ID == id {
			s.announcements = append(s.announcements[:i], s.announcements[i+1:]...)
			s.markDirtyLocked()
			return true
		}
	}
	return false
}

// -------------------------------------------------------- join requests ----

// AddJoinRequest stores a group join request.
func (s *Store) AddJoinRequest(r models.GroupJoinRequest) *models.GroupJoinRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.groupJoinRequest = append(s.groupJoinRequest, &r)
	s.markDirtyLocked()
	return s.groupJoinRequest[len(s.groupJoinRequest)-1]
}

// JoinRequestsOf returns the pending requests of a chat.
func (s *Store) JoinRequestsOf(chatID string) []models.GroupJoinRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []models.GroupJoinRequest{}
	for _, r := range s.groupJoinRequest {
		if r.ChatID == chatID && r.Status == "PENDING" {
			out = append(out, *r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// AllJoinRequests returns every join request, newest first.
func (s *Store) AllJoinRequests() []models.GroupJoinRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.GroupJoinRequest, 0, len(s.groupJoinRequest))
	for _, r := range s.groupJoinRequest {
		out = append(out, *r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// ManagedJoinRequests returns the pending requests of every chat the user owns
// or administrates.
func (s *Store) ManagedJoinRequests(userID string) []models.GroupJoinRequest {
	s.mu.RLock()
	managed := map[string]bool{}
	for _, id := range s.chatOrder {
		c := s.chats[id]
		if c == nil || c.Type != models.ChatGroup {
			continue
		}
		if c.OwnerID == userID || contains(c.Admins, userID) {
			managed[id] = true
		}
	}
	out := []models.GroupJoinRequest{}
	for _, r := range s.groupJoinRequest {
		if managed[r.ChatID] && r.Status == "PENDING" {
			out = append(out, *r)
		}
	}
	s.mu.RUnlock()
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// FindJoinRequest returns a join request by id.
func (s *Store) FindJoinRequest(id string) *models.GroupJoinRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.groupJoinRequest {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// SetJoinRequestStatus updates the status of a join request.
func (s *Store) SetJoinRequestStatus(id, status string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.groupJoinRequest {
		if r.ID == id {
			r.Status = status
			r.HandledAt = models.Now()
			s.markDirtyLocked()
			return true
		}
	}
	return false
}

// ----------------------------------------------------------- push tokens ----

// SetPushToken registers or updates the push token of a user.
func (s *Store) SetPushToken(userID, cid, platform, deviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pushTokens[userID] = &models.PushToken{
		UserID:    userID,
		CID:       cid,
		Platform:  platform,
		DeviceID:  deviceID,
		UpdatedAt: models.Now(),
	}
	s.markDirtyLocked()
}

// GetPushToken returns the push token of a user.
func (s *Store) GetPushToken(userID string) *models.PushToken {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pushTokens[userID]
}

// RemovePushToken drops the push token of a user.
func (s *Store) RemovePushToken(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pushTokens, userID)
	s.markDirtyLocked()
}

// ---------------------------------------------------------------- @all ----

// MaxAtAllPerDay is the daily @所有人 budget of a single user.
const MaxAtAllPerDay = 20

// CheckAtAll consumes one unit of the daily @所有人 budget.
func (s *Store) CheckAtAll(userID string) (allowed bool, remaining int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	today := timeNowUTCDate()
	usage := s.atAllUsage[userID]
	if usage == nil || usage.Date != today {
		usage = &models.AtAllUsage{Date: today}
		s.atAllUsage[userID] = usage
	}
	if usage.Count >= MaxAtAllPerDay {
		return false, 0
	}
	usage.Count++
	return true, MaxAtAllPerDay - usage.Count
}

// ------------------------------------------------------------ audit log ----

// AddAuditLog records an administrative action.
func (s *Store) AddAuditLog(entry models.AuditLog) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auditLogs = append(s.auditLogs, &entry)
	if len(s.auditLogs) > 2000 {
		s.auditLogs = s.auditLogs[len(s.auditLogs)-2000:]
	}
	s.markDirtyLocked()
}

// AuditLogs returns the recorded administrative actions, newest first.
func (s *Store) AuditLogs(limit int) []models.AuditLog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.AuditLog, 0, len(s.auditLogs))
	for i := len(s.auditLogs) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, *s.auditLogs[i])
	}
	return out
}

// ------------------------------------------------------- offline queues ----

// AddToOfflineQueue queues a message for a disconnected recipient.
func (s *Store) AddToOfflineQueue(recipientID string, m *models.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.offlineQueue = append(s.offlineQueue, models.OfflineItem{
		ID:          newID(),
		RecipientID: recipientID,
		Message:     m,
		Timestamp:   models.Now(),
	})
	if len(s.offlineQueue) > MaxOfflineQueue {
		s.offlineQueue = s.offlineQueue[len(s.offlineQueue)-MaxOfflineQueue:]
	}
}

// TakePendingMessages returns and removes the queued messages of a user.
func (s *Store) TakePendingMessages(userID string) []*models.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []*models.Message{}
	kept := s.offlineQueue[:0]
	for _, item := range s.offlineQueue {
		if item.RecipientID == userID && item.Message != nil {
			out = append(out, item.Message)
		} else {
			kept = append(kept, item)
		}
	}
	s.offlineQueue = kept
	sort.SliceStable(out, func(i, j int) bool { return out[i].Timestamp < out[j].Timestamp })
	return out
}

// AddToRecallQueue queues a recall event for a disconnected recipient.
func (s *Store) AddToRecallQueue(recipientID, chatID, messageID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recallQueue = append(s.recallQueue, models.RecallItem{
		ID:          newID(),
		RecipientID: recipientID,
		ChatID:      chatID,
		MessageID:   messageID,
		Timestamp:   models.Now(),
	})
}

// TakePendingRecalls returns and removes the queued recall events of a user.
func (s *Store) TakePendingRecalls(userID string) []models.RecallItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []models.RecallItem{}
	kept := s.recallQueue[:0]
	for _, item := range s.recallQueue {
		if item.RecipientID == userID {
			out = append(out, item)
		} else {
			kept = append(kept, item)
		}
	}
	s.recallQueue = kept
	return out
}

// OfflineQueueSize returns the number of queued offline messages.
func (s *Store) OfflineQueueSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.offlineQueue)
}

// -------------------------------------------------------------- helpers ----

func (s *Store) markDirtyLocked() { s.dirty = true }
