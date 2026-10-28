package api

import (
	"net/http"
	"strings"

	"github.com/hutaomu/hutaomuchat/server/internal/httpx"
	"github.com/hutaomu/hutaomuchat/server/internal/models"
)

// handleSearchUsers looks up users by uid, phone number or nickname.
func (s *Server) handleSearchUsers(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		httpx.WriteError(w, http.StatusBadRequest, "缺少搜索关键字")
		return
	}

	matches := s.Store.SearchUsers(query, user.ID)
	out := make([]models.UserDTO, 0, len(matches))
	for _, candidate := range matches {
		// A user that hides their phone number can only be found by name or uid.
		if candidate.Phone == query && !candidate.PhoneSearchable {
			continue
		}
		dto := models.NewUserDTO(candidate)
		dto.IsFriend = s.Store.AreFriends(user.ID, candidate.ID)
		out = append(out, dto)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"users": out})
}

// handleRecommendedUsers returns a list of suggested contacts.
func (s *Server) handleRecommendedUsers(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	exclude := append([]string{user.ID}, s.Store.FriendIDs(user.ID)...)
	candidates := s.Store.RecommendedUsers(exclude, 10)
	out := make([]models.UserDTO, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, models.NewUserDTO(candidate))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"users": out})
}

// handleGetUser returns the public profile of a user.
func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) {
	viewer := currentUser(r)
	target := s.Store.FindUserByID(r.PathValue("id"))
	if target == nil {
		httpx.WriteError(w, http.StatusNotFound, "用户不存在")
		return
	}
	dto := models.NewUserDTO(target)
	dto.IsFriend = s.Store.AreFriends(viewer.ID, target.ID)
	if target.ID != viewer.ID && !target.PhoneSearchable {
		dto.Phone = ""
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user": dto})
}
