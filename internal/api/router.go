package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/hutaomu/hutaomuchat/server/internal/httpx"
)

// Routes returns the fully wired HTTP handler of the server.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// ---------------------------------------------------------- public ---
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/setup/status", s.handleSetupStatus)
	mux.HandleFunc("POST /api/setup", s.handleSetup)
	mux.HandleFunc("POST /api/auth/register", s.handleRegister)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/refresh", s.handleRefreshToken)
	mux.HandleFunc("POST /api/admin/login", s.handleAdminLogin)

	// ------------------------------------------------------- user scope ---
	mux.Handle("GET /api/auth/me", s.RequireAuth(http.HandlerFunc(s.handleMe)))
	mux.Handle("PUT /api/auth/profile", s.RequireAuth(http.HandlerFunc(s.handleUpdateProfile)))

	mux.Handle("GET /api/users/search", s.RequireAuth(http.HandlerFunc(s.handleSearchUsers)))
	mux.Handle("GET /api/users/recommended", s.RequireAuth(http.HandlerFunc(s.handleRecommendedUsers)))
	mux.Handle("GET /api/users/{id}", s.RequireAuth(http.HandlerFunc(s.handleGetUser)))

	mux.Handle("GET /api/friends", s.RequireAuth(http.HandlerFunc(s.handleListFriends)))
	mux.Handle("POST /api/friends/request", s.RequireAuth(http.HandlerFunc(s.handleSendFriendRequest)))
	mux.Handle("GET /api/friends/requests", s.RequireAuth(http.HandlerFunc(s.handleFriendRequests)))
	mux.Handle("POST /api/friends/requests/{id}/respond", s.RequireAuth(http.HandlerFunc(s.handleRespondFriendRequest)))
	mux.Handle("DELETE /api/friends/{friendId}", s.RequireAuth(http.HandlerFunc(s.handleDeleteFriend)))

	mux.Handle("GET /api/chats", s.RequireAuth(http.HandlerFunc(s.handleListChats)))
	mux.Handle("POST /api/chats/single", s.RequireAuth(http.HandlerFunc(s.handleCreateSingleChat)))
	mux.Handle("POST /api/chats/group", s.RequireAuth(http.HandlerFunc(s.handleCreateGroupChat)))
	mux.Handle("GET /api/chats/global", s.RequireAuth(http.HandlerFunc(s.handleGlobalChat)))
	mux.Handle("GET /api/chats/search", s.RequireAuth(http.HandlerFunc(s.handleSearchGroups)))
	mux.Handle("GET /api/chats/managed-join-requests", s.RequireAuth(http.HandlerFunc(s.handleManagedJoinRequests)))
	mux.Handle("POST /api/chats/join-requests/{requestId}/respond", s.RequireAuth(http.HandlerFunc(s.handleRespondJoinRequest)))
	mux.Handle("GET /api/chats/{id}", s.RequireAuth(http.HandlerFunc(s.handleGetChat)))
	mux.Handle("POST /api/chats/{id}/leave", s.RequireAuth(http.HandlerFunc(s.handleLeaveGroup)))
	mux.Handle("POST /api/chats/{id}/announcement", s.RequireAuth(http.HandlerFunc(s.handlePostAnnouncement)))
	mux.Handle("GET /api/chats/{id}/announcements", s.RequireAuth(http.HandlerFunc(s.handleListAnnouncements)))
	mux.Handle("DELETE /api/chats/{id}/announcements/{announcementId}", s.RequireAuth(http.HandlerFunc(s.handleDeleteAnnouncement)))
	mux.Handle("PUT /api/chats/{id}/avatar", s.RequireAuth(http.HandlerFunc(s.handleUpdateGroupAvatar)))
	mux.Handle("POST /api/chats/{id}/mute", s.RequireAuth(http.HandlerFunc(s.handleToggleMute)))
	mux.Handle("POST /api/chats/{id}/pin", s.RequireAuth(http.HandlerFunc(s.handleTogglePin)))
	mux.Handle("POST /api/chats/{id}/admins", s.RequireAuth(http.HandlerFunc(s.handleAddAdmin)))
	mux.Handle("DELETE /api/chats/{id}/admins/{userId}", s.RequireAuth(http.HandlerFunc(s.handleRemoveAdmin)))
	mux.Handle("GET /api/chats/{id}/files", s.RequireAuth(http.HandlerFunc(s.handleGroupFiles)))
	mux.Handle("POST /api/chats/{id}/files/{fileId}/permanent", s.RequireAuth(http.HandlerFunc(s.handleMarkFilePermanent)))
	mux.Handle("DELETE /api/chats/{id}/files/{fileId}", s.RequireAuth(http.HandlerFunc(s.handleDeleteGroupFile)))
	mux.Handle("POST /api/chats/{id}/join", s.RequireAuth(http.HandlerFunc(s.handleSendJoinRequest)))
	mux.Handle("GET /api/chats/{id}/join-requests", s.RequireAuth(http.HandlerFunc(s.handleListJoinRequests)))

	mux.Handle("GET /api/messages/{chatId}", s.RequireAuth(http.HandlerFunc(s.handleGetMessages)))
	mux.Handle("POST /api/messages/{chatId}", s.RequireAuth(http.HandlerFunc(s.handleSendMessage)))

	mux.Handle("GET /api/moments", s.RequireAuth(http.HandlerFunc(s.handleListMoments)))
	mux.Handle("GET /api/moments/friends", s.RequireAuth(http.HandlerFunc(s.handleFriendMoments)))
	mux.Handle("POST /api/moments", s.RequireAuth(http.HandlerFunc(s.handleCreateMoment)))
	mux.Handle("POST /api/moments/{id}/like", s.RequireAuth(http.HandlerFunc(s.handleLikeMoment)))
	mux.Handle("DELETE /api/moments/{id}/like", s.RequireAuth(http.HandlerFunc(s.handleUnlikeMoment)))
	mux.Handle("POST /api/moments/{id}/comment", s.RequireAuth(http.HandlerFunc(s.handleCommentMoment)))
	mux.Handle("DELETE /api/moments/{id}", s.RequireAuth(http.HandlerFunc(s.handleDeleteMoment)))

	mux.Handle("GET /api/stickers", s.RequireAuth(http.HandlerFunc(s.handleListStickers)))
	mux.Handle("POST /api/stickers", s.RequireAuth(http.HandlerFunc(s.handleAddSticker)))
	mux.Handle("DELETE /api/stickers/{id}", s.RequireAuth(http.HandlerFunc(s.handleDeleteSticker)))

	mux.Handle("POST /api/upload", s.RequireAuth(http.HandlerFunc(s.handleUpload)))
	mux.Handle("POST /api/upload/multiple", s.RequireAuth(http.HandlerFunc(s.handleUploadMultiple)))

	mux.Handle("POST /api/push/token", s.RequireAuth(http.HandlerFunc(s.handleRegisterPushToken)))
	mux.Handle("DELETE /api/push/token", s.RequireAuth(http.HandlerFunc(s.handleRemovePushToken)))

	// ----------------------------------------------------- admin scope ---
	mux.Handle("GET /api/admin/stats", s.RequireAdmin(http.HandlerFunc(s.handleAdminStats)))
	mux.Handle("GET /api/admin/users", s.RequireAdmin(http.HandlerFunc(s.handleAdminUsers)))
	mux.Handle("GET /api/admin/users/{id}", s.RequireAdmin(http.HandlerFunc(s.handleAdminUserDetail)))
	mux.Handle("PUT /api/admin/users/{id}", s.RequireAdmin(http.HandlerFunc(s.handleAdminUpdateUser)))
	mux.Handle("DELETE /api/admin/users/{id}", s.RequireAdmin(http.HandlerFunc(s.handleAdminDeleteUser)))
	mux.Handle("POST /api/admin/users/{id}/ban", s.RequireAdmin(http.HandlerFunc(s.handleAdminBanUser)))
	mux.Handle("POST /api/admin/users/{id}/unban", s.RequireAdmin(http.HandlerFunc(s.handleAdminUnbanUser)))
	mux.Handle("POST /api/admin/users/{id}/password", s.RequireAdmin(http.HandlerFunc(s.handleAdminResetPassword)))
	mux.Handle("GET /api/admin/chats", s.RequireAdmin(http.HandlerFunc(s.handleAdminChats)))
	mux.Handle("GET /api/admin/chats/{id}/messages", s.RequireAdmin(http.HandlerFunc(s.handleAdminChatMessages)))
	mux.Handle("DELETE /api/admin/chats/{id}", s.RequireAdmin(http.HandlerFunc(s.handleAdminDeleteChat)))
	mux.Handle("GET /api/admin/moments", s.RequireAdmin(http.HandlerFunc(s.handleAdminMoments)))
	mux.Handle("DELETE /api/admin/moments/{id}", s.RequireAdmin(http.HandlerFunc(s.handleAdminDeleteMoment)))
	mux.Handle("GET /api/admin/announcements", s.RequireAdmin(http.HandlerFunc(s.handleAdminAnnouncements)))
	mux.Handle("DELETE /api/admin/announcements/{id}", s.RequireAdmin(http.HandlerFunc(s.handleAdminDeleteAnnouncement)))
	mux.Handle("GET /api/admin/join-requests", s.RequireAdmin(http.HandlerFunc(s.handleAdminJoinRequests)))
	mux.Handle("GET /api/admin/online", s.RequireAdmin(http.HandlerFunc(s.handleAdminOnline)))
	mux.Handle("POST /api/admin/broadcast", s.RequireAdmin(http.HandlerFunc(s.handleAdminBroadcast)))
	mux.Handle("GET /api/admin/system", s.RequireAdmin(http.HandlerFunc(s.handleAdminSystem)))
	mux.Handle("DELETE /api/admin/system/files/{filename}", s.RequireAdmin(http.HandlerFunc(s.handleAdminDeleteFile)))
	mux.Handle("GET /api/admin/logs/system", s.RequireAdmin(http.HandlerFunc(s.handleAdminSystemLogs)))
	mux.Handle("GET /api/admin/logs/user", s.RequireAdmin(http.HandlerFunc(s.handleAdminUserLogs)))
	mux.Handle("GET /api/admin/logs/file/{type}", s.RequireAdmin(http.HandlerFunc(s.handleAdminLogFile)))
	mux.Handle("GET /api/admin/logs/files", s.RequireAdmin(http.HandlerFunc(s.handleAdminLogFiles)))
	mux.Handle("GET /api/admin/logs/stats", s.RequireAdmin(http.HandlerFunc(s.handleAdminLogStats)))
	mux.Handle("GET /api/admin/audit", s.RequireAdmin(http.HandlerFunc(s.handleAdminAudit)))
	mux.Handle("GET /api/admin/settings", s.RequireAdmin(http.HandlerFunc(s.handleAdminSettings)))
	mux.Handle("PUT /api/admin/settings", s.RequireAdmin(http.HandlerFunc(s.handleAdminUpdateSettings)))
	mux.Handle("PUT /api/admin/settings/getui", s.RequireAdmin(http.HandlerFunc(s.handleAdminUpdateGetui)))
	mux.Handle("POST /api/admin/settings/reset", s.RequireAdmin(http.HandlerFunc(s.handleAdminReset)))
	mux.Handle("POST /api/admin/settings/restart", s.RequireAdmin(http.HandlerFunc(s.handleAdminRestart)))

	// ------------------------------------------------------- websocket ---
	mux.HandleFunc("GET /ws", s.handleWebSocket)
	mux.HandleFunc("GET /ws/admin", s.handleAdminWebSocket)

	// --------------------------------------------------------- static ----
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(s.UploadsDir))))
	mux.HandleFunc("GET /api/download/{filename}", s.handleDownload)
	mux.HandleFunc("GET /admin/{rest...}", s.handleConsoleAsset)
	mux.HandleFunc("GET /", s.handleRoot)

	// Unknown API routes answer with a JSON error instead of the default
	// plain text body of the standard router.
	notFound := func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, "接口不存在: "+r.URL.Path)
	}
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		mux.HandleFunc(method+" /api/{rest...}", notFound)
	}

	return httpx.Chain(mux,
		httpx.Recoverer(func(format string, args ...any) {
			s.Logger.Error("panic recovered: "+format, nil)
		}),
		httpx.AccessLog(func(format string, args ...any) {
			s.Logger.Debug(format, nil)
		}),
		httpx.CORS,
		httpx.SecurityHeaders,
		s.RequireSetup,
	)
}

// handleConsoleAsset serves the assets of the administration console.
//
// Static files are always available so that the setup wizard can load its
// stylesheet. When the server has not been initialized yet the page routes
// fall back to the setup wizard instead of the console.
func (s *Server) handleConsoleAsset(w http.ResponseWriter, r *http.Request) {
	// The console assets are small and change with every release. They are
	// therefore never cached, and conditional requests are dropped so that a
	// browser can never end up with a stale copy of a page.
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	r.Header.Del("If-Modified-Since")
	r.Header.Del("If-None-Match")

	rest := r.PathValue("rest")
	target := filepath.Join(s.WebDir, filepath.Clean("/"+rest))
	if info, err := os.Stat(target); err == nil && !info.IsDir() {
		http.StripPrefix("/admin/", http.FileServer(http.Dir(s.WebDir))).ServeHTTP(w, r)
		return
	}

	if !s.Config.Initialized() {
		http.ServeFile(w, r, filepath.Join(s.WebDir, "setup.html"))
		return
	}

	index := filepath.Join(s.WebDir, "index.html")
	if _, err := os.Stat(index); err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, index)
}

// handleRoot redirects the site root to the administration console.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(r.Header.Get("Accept"), "text/html") {
		http.Redirect(w, r, "/admin/", http.StatusFound)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"name":    s.Config.ServerName(),
		"status":  "ok",
		"console": "/admin/",
	})
}
