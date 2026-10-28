// Package security implements the authentication primitives of the server:
// password hashing and stateless JSON web tokens.
//
// The implementation deliberately relies on the Go standard library only, so
// the server can be built without network access to a module proxy.
package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Errors returned by the token helpers.
var (
	ErrTokenMalformed = errors.New("令牌格式错误")
	ErrTokenSignature = errors.New("令牌签名无效")
	ErrTokenExpired   = errors.New("令牌已过期")
)

// Claims is the payload of an issued token.
type Claims struct {
	UserID   string `json:"userId"`
	UID      string `json:"uid"`
	Nickname string `json:"nickname,omitempty"`
	Role     string `json:"role,omitempty"`
	Kind     string `json:"kind,omitempty"`
	JTI      string `json:"jti,omitempty"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
}

// Token kinds.
const (
	KindAccess  = "access"
	KindRefresh = "refresh"
)

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

var b64 = base64.RawURLEncoding

// SignToken issues a HS256 token for the given claims.
func SignToken(secret string, claims Claims) (string, error) {
	header, err := json.Marshal(jwtHeader{Alg: "HS256", Typ: "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := b64.EncodeToString(header) + "." + b64.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	signature := b64.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature, nil
}

// VerifyToken validates the signature and expiry of a token.
func VerifyToken(secret, token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrTokenMalformed
	}
	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	expected := b64.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(parts[2])) != 1 {
		return nil, ErrTokenSignature
	}
	payload, err := b64.DecodeString(parts[1])
	if err != nil {
		return nil, ErrTokenMalformed
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrTokenMalformed
	}
	if claims.Expires > 0 && time.Now().Unix() > claims.Expires {
		return nil, ErrTokenExpired
	}
	return &claims, nil
}

// UserTokenTTL is the lifetime of a user session token.
//
// The token is short lived on purpose: the client exchanges the long lived
// refresh token for a new access token instead of asking the user to log in
// again.
var UserTokenTTL = 7 * 24 * time.Hour

// RefreshTokenTTL is the lifetime of a refresh token.
//
// A refresh token stays valid for a long time so that the login state of the
// client survives an access token expiring.
var RefreshTokenTTL = 90 * 24 * time.Hour

// AdminTokenTTL is the lifetime of an administration session token.
const AdminTokenTTL = 24 * time.Hour

// SetUserTokenTTL overrides the lifetime of the access token.
func SetUserTokenTTL(d time.Duration) {
	if d > 0 {
		UserTokenTTL = d
	}
}

// SetRefreshTokenTTL overrides the lifetime of the refresh token.
func SetRefreshTokenTTL(d time.Duration) {
	if d > 0 {
		RefreshTokenTTL = d
	}
}

// NewUserClaims builds the claims of a user session.
func NewUserClaims(userID, uid, nickname, role string) Claims {
	now := time.Now()
	return Claims{
		UserID:   userID,
		UID:      uid,
		Nickname: nickname,
		Role:     role,
		Kind:     KindAccess,
		IssuedAt: now.Unix(),
		Expires:  now.Add(UserTokenTTL).Unix(),
	}
}

// NewRefreshClaims builds the claims of a refresh token.
//
// The claims carry a random identifier so that every refresh token is unique
// and can be rotated.
func NewRefreshClaims(userID, uid string) Claims {
	now := time.Now()
	return Claims{
		UserID:   userID,
		UID:      uid,
		Kind:     KindRefresh,
		JTI:      randomID(),
		IssuedAt: now.Unix(),
		Expires:  now.Add(RefreshTokenTTL).Unix(),
	}
}

// randomID returns a random 128 bit identifier encoded as hex.
func randomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}

// IsRefresh reports whether the claims belong to a refresh token.
func (c *Claims) IsRefresh() bool { return c.Kind == KindRefresh }

// NewAdminClaims builds the claims of an administration session.
func NewAdminClaims(username string) Claims {
	now := time.Now()
	return Claims{
		UserID:   username,
		UID:      username,
		Nickname: username,
		Role:     "admin",
		IssuedAt: now.Unix(),
		Expires:  now.Add(AdminTokenTTL).Unix(),
	}
}
