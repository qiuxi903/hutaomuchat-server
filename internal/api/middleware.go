package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/hutaomu/hutaomuchat/server/internal/httpx"
	"github.com/hutaomu/hutaomuchat/server/internal/models"
	"github.com/hutaomu/hutaomuchat/server/internal/security"
)

type contextKey string

const (
	userContextKey   contextKey = "api.user"
	claimsContextKey contextKey = "api.claims"
)

// RequireAuth rejects requests without a valid user token.
func (s *Server) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "缺少认证信息")
			return
		}
		claims, err := security.VerifyToken(s.jwtSecret(), token)
		if err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, "认证已过期，请重新登录")
			return
		}
		user := s.Store.FindUserByID(claims.UserID)
		if user == nil {
			httpx.WriteError(w, http.StatusUnauthorized, "用户不存在")
			return
		}
		if user.Banned {
			httpx.WriteError(w, http.StatusForbidden, "账号已被封禁")
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		ctx = context.WithValue(ctx, claimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAdmin rejects requests without a valid administration token.
func (s *Server) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "需要管理员认证")
			return
		}
		claims, err := security.VerifyToken(s.jwtSecret(), token)
		if err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, "认证已过期")
			return
		}
		if claims.Role != "admin" {
			httpx.WriteError(w, http.StatusForbidden, "权限不足")
			return
		}
		ctx := context.WithValue(r.Context(), claimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireSetup blocks every API call until the first-run setup is finished.
func (s *Server) RequireSetup(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Config.Initialized() || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/setup") || r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		httpx.WriteError(w, http.StatusServiceUnavailable, "服务器尚未初始化，请先完成初始化向导")
	})
}

// currentUser returns the authenticated user of a request.
func currentUser(r *http.Request) *models.User {
	user, _ := r.Context().Value(userContextKey).(*models.User)
	return user
}

// adminName returns the name of the authenticated administrator.
func adminName(r *http.Request) string {
	claims, _ := r.Context().Value(claimsContextKey).(*security.Claims)
	if claims == nil {
		return ""
	}
	return claims.Nickname
}

// bearerToken extracts the token from the Authorization header.
func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}
	if strings.HasPrefix(strings.ToLower(header), "bearer ") {
		token := strings.TrimSpace(header[7:])
		return token, token != ""
	}
	return strings.TrimSpace(header), true
}
