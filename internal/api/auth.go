package api

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/hutaomu/hutaomuchat/server/internal/httpx"
	"github.com/hutaomu/hutaomuchat/server/internal/models"
	"github.com/hutaomu/hutaomuchat/server/internal/security"
	"github.com/hutaomu/hutaomuchat/server/internal/store"
)

var (
	phonePattern    = regexp.MustCompile(`^[0-9]{6,20}$`)
	nicknamePattern = regexp.MustCompile(`^[^\x00-\x1f]{1,24}$`)
)

// defaultSignatures are assigned to new accounts when the user does not pick
// a personal signature.
var defaultSignatures = []string{
	"这个人很懒，什么都没写~",
	"今天也要加油鸭！",
	"生活不止眼前的代码",
	"保持微笑，一切都会好的",
	"努力成为更好的自己",
	"愿你每天开心快乐",
	"世界那么大，我想去看看",
	"做自己喜欢的事",
	"平凡的日子里闪闪发光",
	"每天都要开开心心",
}

type registerRequest struct {
	Phone    string `json:"phone"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
}

type loginRequest struct {
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

type profileRequest struct {
	Nickname              *string `json:"nickname"`
	Signature             *string `json:"signature"`
	Gender                *string `json:"gender"`
	Region                *string `json:"region"`
	AvatarURL             *string `json:"avatar_url"`
	BackgroundURL         *string `json:"background_url"`
	BackgroundOriginalURL *string `json:"background_original_url"`
	FriendVerification    *bool   `json:"friend_verification"`
	PhoneSearchable       *bool   `json:"phone_searchable"`
	Recommendable         *bool   `json:"recommendable"`
}

// handleRegister creates a new account.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Phone = strings.TrimSpace(req.Phone)
	req.Nickname = strings.TrimSpace(req.Nickname)

	if !phonePattern.MatchString(req.Phone) {
		httpx.WriteError(w, http.StatusBadRequest, "手机号格式不正确")
		return
	}
	if len(req.Password) < 6 || len(req.Password) > 128 {
		httpx.WriteError(w, http.StatusBadRequest, "密码长度需在 6 到 128 个字符之间")
		return
	}
	if !nicknamePattern.MatchString(req.Nickname) {
		httpx.WriteError(w, http.StatusBadRequest, "昵称长度需在 1 到 24 个字符之间")
		return
	}
	if existing := s.Store.FindUserByPhone(req.Phone); existing != nil {
		httpx.WriteError(w, http.StatusConflict, "该手机号已注册")
		return
	}

	hash, err := security.HashPassword(req.Password)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "密码加密失败")
		return
	}

	signature := defaultSignatures[models.Now()%int64(len(defaultSignatures))]
	user := &models.User{
		ID:                 store.NewID(),
		UID:                s.Store.NextUID(),
		Phone:              req.Phone,
		PasswordHash:       hash,
		Nickname:           req.Nickname,
		Signature:          signature,
		Gender:             "UNKNOWN",
		Region:             "中国",
		FriendVerification: true,
		PhoneSearchable:    true,
		Recommendable:      true,
		OnlineStatus:       models.Offline,
		Role:               "user",
		CreatedAt:          models.Now(),
		LastSeenAt:         models.Now(),
	}
	s.Store.CreateUser(user)
	s.Store.AutoJoinGlobalChat(user.ID)

	s.Logger.User(user.ID, "REGISTER", map[string]any{"phone": user.Phone, "nickname": user.Nickname})
	s.Logger.Info("新用户注册", map[string]any{"userId": user.ID, "uid": user.UID})
	s.notifyAdmins("user.register", map[string]any{
		"userId":   user.ID,
		"nickname": user.Nickname,
		"uid":      user.UID,
	})

	token, err := security.SignToken(s.jwtSecret(), security.NewUserClaims(user.ID, user.UID, user.Nickname, user.Role))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "签发令牌失败")
		return
	}
	refreshToken, err := security.SignToken(s.jwtSecret(), security.NewRefreshClaims(user.ID, user.UID))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "签发令牌失败")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"token":         token,
		"refresh_token": refreshToken,
		"expires_in":    int64(security.UserTokenTTL.Seconds()),
		"user":          models.NewUserDTO(user),
	})
}

// handleLogin authenticates an account.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Phone = strings.TrimSpace(req.Phone)

	user := s.Store.FindUserByPhone(req.Phone)
	if user == nil {
		security.DummyVerify(req.Password)
		httpx.WriteError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if !security.VerifyPassword(req.Password, user.PasswordHash) {
		httpx.WriteError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if user.Banned {
		httpx.WriteError(w, http.StatusForbidden, "账号已被封禁，请联系管理员")
		return
	}

	token, err := security.SignToken(s.jwtSecret(), security.NewUserClaims(user.ID, user.UID, user.Nickname, user.Role))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "签发令牌失败")
		return
	}
	s.Store.UpdateUser(user.ID, func(u *models.User) { u.LastSeenAt = models.Now() })
	s.Logger.User(user.ID, "LOGIN", map[string]any{"ip": httpx.ClientIP(r)})

	refreshToken, err := security.SignToken(s.jwtSecret(), security.NewRefreshClaims(user.ID, user.UID))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "签发令牌失败")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"token":         token,
		"refresh_token": refreshToken,
		"expires_in":    int64(security.UserTokenTTL.Seconds()),
		"user":          models.NewUserDTO(user),
	})
}

// handleRefreshToken exchanges a refresh token for a new access token.
//
// This lets a client stay logged in after the short lived access token has
// expired, without asking the user for the password again.
func (s *Server) handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.RefreshToken == "" {
		httpx.WriteError(w, http.StatusBadRequest, "缺少 refresh_token")
		return
	}
	claims, err := security.VerifyToken(s.jwtSecret(), req.RefreshToken)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "刷新令牌无效或已过期，请重新登录")
		return
	}
	if !claims.IsRefresh() {
		httpx.WriteError(w, http.StatusUnauthorized, "这不是有效的刷新令牌")
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
	token, err := security.SignToken(s.jwtSecret(), security.NewUserClaims(user.ID, user.UID, user.Nickname, user.Role))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "签发令牌失败")
		return
	}
	// Rotate the refresh token so that an old one cannot be reused forever.
	refreshToken, err := security.SignToken(s.jwtSecret(), security.NewRefreshClaims(user.ID, user.UID))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "签发令牌失败")
		return
	}
	s.Logger.User(user.ID, "REFRESH_TOKEN", nil)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"token":         token,
		"refresh_token": refreshToken,
		"expires_in":    int64(security.UserTokenTTL.Seconds()),
	})
}

// handleMe returns the profile of the authenticated user.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	dto := models.NewUserDTO(user)
	dto.IsFriend = false
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user": dto})
}

// handleUpdateProfile updates the profile of the authenticated user.
func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var req profileRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Nickname != nil {
		nickname := strings.TrimSpace(*req.Nickname)
		if !nicknamePattern.MatchString(nickname) {
			httpx.WriteError(w, http.StatusBadRequest, "昵称长度需在 1 到 24 个字符之间")
			return
		}
		user.Nickname = nickname
	}
	if req.Signature != nil {
		user.Signature = truncate(*req.Signature, 200)
	}
	if req.Gender != nil {
		switch *req.Gender {
		case "MALE", "FEMALE", "UNKNOWN":
			user.Gender = *req.Gender
		default:
			httpx.WriteError(w, http.StatusBadRequest, "性别参数无效")
			return
		}
	}
	if req.Region != nil {
		user.Region = truncate(strings.TrimSpace(*req.Region), 32)
	}
	if req.AvatarURL != nil {
		user.AvatarURL = truncate(*req.AvatarURL, 512)
	}
	if req.BackgroundURL != nil {
		user.BackgroundURL = truncate(*req.BackgroundURL, 512)
	}
	if req.BackgroundOriginalURL != nil {
		user.BackgroundOriginalURL = truncate(*req.BackgroundOriginalURL, 512)
	}
	if req.FriendVerification != nil {
		user.FriendVerification = *req.FriendVerification
	}
	if req.PhoneSearchable != nil {
		user.PhoneSearchable = *req.PhoneSearchable
	}
	if req.Recommendable != nil {
		user.Recommendable = *req.Recommendable
	}

	s.Store.MarkDirty()
	s.Logger.User(user.ID, "UPDATE_PROFILE", map[string]any{"fields": "profile"})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user": models.NewUserDTO(user)})
}

// truncate shortens a string to the given number of runes.
func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
