package store

import "github.com/hutaomu/hutaomuchat/server/internal/models"

// FileRetentionWindow is the period after which a group file is removed
// unless an administrator marked it as permanent.
const FileRetentionWindow = 7 * 24 * 60 * 60 * 1000

// MarkFilePermanent protects a group file from the automatic clean up.
func (s *Store) MarkFilePermanent(fileID, chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.permanentFiles[fileID] = chatID
	s.markDirtyLocked()
}

// IsFilePermanent reports whether a group file is protected.
func (s *Store) IsFilePermanent(fileID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.permanentFiles[fileID]
	return ok
}

// UnmarkFilePermanent removes the protection of a group file.
func (s *Store) UnmarkFilePermanent(fileID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.permanentFiles, fileID)
	s.markDirtyLocked()
}

// GroupFile is a shared file of a conversation together with its retention
// information.
type GroupFile struct {
	*models.Message
	Permanent bool  `json:"permanent"`
	Expired   bool  `json:"expired"`
	AgeMillis int64 `json:"age_ms"`
}

// GroupFiles returns the shared files of a conversation that are still
// available.
func (s *Store) GroupFiles(chatID string, now int64) []GroupFile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []GroupFile{}
	for _, m := range s.messages[chatID] {
		if m.Type != models.MsgFile {
			continue
		}
		_, permanent := s.permanentFiles[m.ID]
		age := now - m.Timestamp
		expired := !permanent && age > FileRetentionWindow
		if expired {
			continue
		}
		out = append(out, GroupFile{Message: m, Permanent: permanent, Expired: false, AgeMillis: age})
	}
	return out
}
