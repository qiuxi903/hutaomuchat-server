package api

import (
	"net/http"
	"strings"

	"github.com/hutaomu/hutaomuchat/server/internal/httpx"
)

// handleRegisterPushToken stores the system level push token of a device.
func (s *Server) handleRegisterPushToken(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var req struct {
		CID      string `json:"cid"`
		Platform string `json:"platform"`
		DeviceID string `json:"device_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	cid := strings.TrimSpace(req.CID)
	if cid == "" {
		httpx.WriteError(w, http.StatusBadRequest, "缺少 cid")
		return
	}
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = "android"
	}
	s.Store.SetPushToken(user.ID, cid, platform, req.DeviceID)
	s.Logger.Debug("推送令牌已注册", map[string]any{"userId": user.ID, "platform": platform})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleRemovePushToken drops the system level push token of a device.
func (s *Server) handleRemovePushToken(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	s.Store.RemovePushToken(user.ID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}
