package api

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UploadRetention is the period after which an uploaded file that is no
// longer referenced by any record may be removed.
const UploadRetention = 7 * 24 * time.Hour

// UploadPrefix is the path prefix under which uploaded files are served.
const UploadPrefix = "/uploads/"

// referencedUploads returns the set of upload file names that are still
// referenced by the data.
//
// Files that are referenced are never deleted, so a user avatar, a profile
// background, a sticker or a picture sent in a chat stays available
// regardless of how long ago it was uploaded.
func (s *Server) referencedUploads() map[string]bool {
	referenced := map[string]bool{}

	mark := func(value string) {
		for _, name := range extractUploadNames(value) {
			referenced[name] = true
		}
	}

	for _, user := range s.Store.ListUsers() {
		mark(user.AvatarURL)
		mark(user.BackgroundURL)
		mark(user.BackgroundOriginalURL)
	}
	for _, sticker := range s.Store.AllStickers() {
		mark(sticker.URL)
	}
	for _, moment := range s.Store.AllMoments() {
		for _, image := range moment.Images {
			mark(image)
		}
	}
	for _, chat := range s.Store.AllChats() {
		mark(chat.AvatarURL)
		mark(chat.GroupAvatar)
		for _, message := range s.Store.MessagesOf(chat.ID, 0) {
			mark(message.Content)
		}
	}

	return referenced
}

// extractUploadNames pulls the uploaded file names out of a string that may
// contain one or more "/uploads/<name>" references.
func extractUploadNames(value string) []string {
	if value == "" || !strings.Contains(value, UploadPrefix) {
		return nil
	}
	names := []string{}
	rest := value
	for {
		index := strings.Index(rest, UploadPrefix)
		if index < 0 {
			break
		}
		rest = rest[index+len(UploadPrefix):]
		end := strings.IndexAny(rest, "\"' \t\r\n,;)}]")
		name := rest
		if end >= 0 {
			name = rest[:end]
		}
		name = strings.TrimSpace(name)
		if name != "" && !strings.Contains(name, "/") && !strings.Contains(name, "\\") {
			names = append(names, name)
		}
		if end < 0 {
			break
		}
		rest = rest[end+1:]
	}
	return names
}

// CleanupUploads removes uploaded files that are neither referenced by any
// record nor newer than the retention window.
//
// It returns the number of deleted files.
func (s *Server) CleanupUploads() int {
	referenced := s.referencedUploads()
	entries, err := os.ReadDir(s.UploadsDir)
	if err != nil {
		return 0
	}

	cutoff := time.Now().Add(-UploadRetention)
	deleted := 0
	for _, entry := range entries {
		name := entry.Name()
		if name == ".gitkeep" || entry.IsDir() {
			continue
		}
		if referenced[name] {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(s.UploadsDir, name)); err == nil {
			deleted++
		}
	}
	return deleted
}

// StartUploadCleanup runs the cleanup job once a day.
func (s *Server) StartUploadCleanup() {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if removed := s.CleanupUploads(); removed > 0 {
				s.Logger.Info("清理了未引用的上传文件", map[string]any{"count": removed})
			}
		}
	}()
}
