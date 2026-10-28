package store

import "github.com/hutaomu/hutaomuchat/server/internal/models"

// CountStickers returns the total number of stored stickers.
func (s *Store) CountStickers() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.stickers)
}

// AllStickers returns every stored sticker.
func (s *Store) AllStickers() []models.Sticker {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.Sticker, 0, len(s.stickers))
	for _, sticker := range s.stickers {
		out = append(out, *sticker)
	}
	return out
}

// CountAnnouncements returns the total number of stored announcements.
func (s *Store) CountAnnouncements() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.announcements)
}

// CountPushTokens returns the total number of registered push tokens.
func (s *Store) CountPushTokens() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.pushTokens)
}

// RemoveStickerByID removes a sticker regardless of its owner.
func (s *Store) RemoveStickerByID(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, st := range s.stickers {
		if st.ID == id {
			s.stickers = append(s.stickers[:i], s.stickers[i+1:]...)
			s.markDirtyLocked()
			return true
		}
	}
	return false
}

// Reset clears every record of the store.
func (s *Store) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users = map[string]*models.User{}
	s.usersByPhone = map[string]*models.User{}
	s.usersByUID = map[string]*models.User{}
	s.userOrder = nil
	s.friendships = nil
	s.friendSet = map[string]map[string]struct{}{}
	s.friendRequests = nil
	s.chats = map[string]*models.Chat{}
	s.chatOrder = nil
	s.members = map[string][]string{}
	s.memberRole = map[string]map[string]string{}
	s.messages = map[string][]*models.Message{}
	s.moments = nil
	s.momentIndex = map[string]*models.Moment{}
	s.momentLikes = map[string]map[string]int64{}
	s.momentComments = map[string][]models.MomentComment{}
	s.stickers = nil
	s.announcements = nil
	s.groupJoinRequest = nil
	s.pushTokens = map[string]*models.PushToken{}
	s.atAllUsage = map[string]*models.AtAllUsage{}
	s.permanentFiles = map[string]string{}
	s.auditLogs = nil
	s.offlineQueue = nil
	s.recallQueue = nil
	s.uidCounter = 0
	s.groupUIDCounter = 0
	s.getOrCreateGlobalChat()
	s.markDirtyLocked()
}
