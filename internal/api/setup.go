package api

import (
	"net/http"
	"strings"

	"github.com/hutaomu/hutaomuchat/server/internal/config"
	"github.com/hutaomu/hutaomuchat/server/internal/httpx"
)

type setupRequest struct {
	AdminUser         string `json:"adminUser"`
	AdminPass         string `json:"adminPass"`
	ServerName        string `json:"serverName"`
	ServerPort        int    `json:"serverPort"`
	JWTSecret         string `json:"jwtSecret"`
	GetuiAppID        string `json:"getuiAppId"`
	GetuiAppKey       string `json:"getuiAppKey"`
	GetuiMasterSecret string `json:"getuiMasterSecret"`
}

// handleSetupStatus reports whether the first-run setup has been completed.
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"initialized": s.Config.Initialized(),
		"serverName":  s.Config.ServerName(),
	})
}

// handleSetup stores the configuration produced by the setup wizard.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if s.Config.Initialized() {
		httpx.WriteError(w, http.StatusForbidden, config.ErrAlreadyInitialized.Error())
		return
	}
	var req setupRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.AdminPass) < 6 {
		httpx.WriteError(w, http.StatusBadRequest, "管理员密码至少 6 个字符")
		return
	}
	if req.AdminUser == "" {
		req.AdminUser = config.DefaultAdminUser
	}
	if req.ServerName == "" {
		req.ServerName = config.DefaultServerName
	}
	if req.ServerPort <= 0 || req.ServerPort > 65535 {
		req.ServerPort = config.DefaultServerPort
	}

	if err := s.Config.ApplySetup(req.ServerName, req.ServerPort, req.AdminUser, req.AdminPass, req.JWTSecret); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "写入配置文件失败: "+err.Error())
		return
	}
	if req.GetuiAppID != "" && req.GetuiAppKey != "" && req.GetuiMasterSecret != "" {
		if err := s.saveGetuiConfig(req.GetuiAppID, req.GetuiAppKey, req.GetuiMasterSecret); err != nil {
			s.Logger.Warn("个推配置保存失败", map[string]any{"error": err.Error()})
		}
	}
	s.Logger.Info("服务器初始化完成", map[string]any{
		"serverName": req.ServerName,
		"adminUser":  req.AdminUser,
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "初始化完成，请重启服务端使配置生效",
	})
}

// handleHealth is a lightweight liveness probe.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"server":      s.Config.ServerName(),
		"timestamp":   nowMillis(),
		"initialized": s.Config.Initialized(),
		"online":      s.Hub.OnlineCount(),
	})
}

// handleAdminLogin authenticates an administrator.
func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if !s.Config.VerifyAdminPassword(req.Username, req.Password) {
		s.Logger.Warn("管理员登录失败", map[string]any{"ip": httpx.ClientIP(r), "username": req.Username})
		httpx.WriteError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	token, err := signAdminToken(s.jwtSecret(), req.Username)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "签发令牌失败")
		return
	}
	s.Logger.Info("管理员登录", map[string]any{"username": req.Username, "ip": httpx.ClientIP(r)})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"token":   token,
		"user":    req.Username,
	})
}
