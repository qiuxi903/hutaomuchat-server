package api

import (
	"net/http"
	"strings"

	"github.com/hutaomu/hutaomuchat/server/internal/httpx"
	"github.com/hutaomu/hutaomuchat/server/internal/models"
	"github.com/hutaomu/hutaomuchat/server/internal/store"
)

// handleListStickers returns the stickers of the authenticated user.
func (s *Server) handleListStickers(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stickers := s.Store.UserStickers(user.ID)
	out := make([]models.StickerDTO, 0, len(stickers))
	for _, sticker := range stickers {
		out = append(out, models.StickerDTO{
			ID:        sticker.ID,
			URL:       sticker.URL,
			CreatedAt: sticker.CreatedAt,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"stickers": out})
}

// handleAddSticker stores a new sticker.
func (s *Server) handleAddSticker(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var req struct {
		URL string `json:"url"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	url := strings.TrimSpace(req.URL)
	if url == "" {
		httpx.WriteError(w, http.StatusBadRequest, "缺少表情包地址")
		return
	}
	sticker := s.Store.AddSticker(models.Sticker{
		ID:        store.NewID(),
		UserID:    user.ID,
		URL:       truncate(url, 512),
		CreatedAt: models.Now(),
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"sticker": models.StickerDTO{ID: sticker.ID, URL: sticker.URL, CreatedAt: sticker.CreatedAt},
	})
}

// handleDeleteSticker removes a sticker.
func (s *Server) handleDeleteSticker(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if !s.Store.DeleteSticker(r.PathValue("id"), user.ID) {
		httpx.WriteError(w, http.StatusNotFound, "表情包不存在")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}
