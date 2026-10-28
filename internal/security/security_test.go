package security

import (
	"strings"
	"testing"
	"time"
)

func TestHashPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("正确马密码123")
	if err != nil {
		t.Fatalf("HashPassword 失败: %v", err)
	}
	if !strings.HasPrefix(hash, passwordAlgorithm+"$") {
		t.Fatalf("哈希格式不正确: %s", hash)
	}
	if !VerifyPassword("正确马密码123", hash) {
		t.Fatal("正确的密码应当校验通过")
	}
	if VerifyPassword("错误密码", hash) {
		t.Fatal("错误的密码不应校验通过")
	}
}

func TestHashPasswordUsesRandomSalt(t *testing.T) {
	first, _ := HashPassword("same-password")
	second, _ := HashPassword("same-password")
	if first == second {
		t.Fatal("相同密码的两次哈希应当不同（盐值随机）")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	cases := []string{
		"",
		"plaintext",
		"pbkdf2_sha256$notanumber$c2FsdA==$aGFzaA==",
		"pbkdf2_sha256$1000$!!!$aGFzaA==",
		"bcrypt$10$abc$def",
	}
	for _, hash := range cases {
		if VerifyPassword("anything", hash) {
			t.Fatalf("非法哈希 %q 不应校验通过", hash)
		}
	}
}

func TestSignAndVerifyToken(t *testing.T) {
	secret := "test-secret-value"
	claims := NewUserClaims("user-1", "htm-001", "小明", "user")
	token, err := SignToken(secret, claims)
	if err != nil {
		t.Fatalf("SignToken 失败: %v", err)
	}
	parsed, err := VerifyToken(secret, token)
	if err != nil {
		t.Fatalf("VerifyToken 失败: %v", err)
	}
	if parsed.UserID != "user-1" || parsed.UID != "htm-001" || parsed.Nickname != "小明" {
		t.Fatalf("令牌内容不正确: %+v", parsed)
	}
	if parsed.Expires <= time.Now().Unix() {
		t.Fatal("令牌应当带有未来的过期时间")
	}
}

func TestVerifyTokenRejectsTamperedToken(t *testing.T) {
	secret := "test-secret-value"
	token, _ := SignToken(secret, NewUserClaims("user-1", "htm-001", "小明", "user"))

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("令牌格式不正确: %s", token)
	}
	tampered := parts[0] + "." + parts[1] + "." + "AAAAinvalidSignature"
	if _, err := VerifyToken(secret, tampered); err == nil {
		t.Fatal("被篡改的令牌不应通过校验")
	}
	if _, err := VerifyToken("another-secret", token); err == nil {
		t.Fatal("使用错误密钥签发的令牌不应通过校验")
	}
}

func TestVerifyTokenRejectsExpiredToken(t *testing.T) {
	secret := "test-secret-value"
	expired := Claims{
		UserID:   "user-1",
		IssuedAt: time.Now().Add(-2 * time.Hour).Unix(),
		Expires:  time.Now().Add(-time.Hour).Unix(),
	}
	token, _ := SignToken(secret, expired)
	if _, err := VerifyToken(secret, token); err != ErrTokenExpired {
		t.Fatalf("过期令牌应返回 ErrTokenExpired，实际: %v", err)
	}
}

func TestAdminClaimsCarryAdminRole(t *testing.T) {
	claims := NewAdminClaims("admin")
	if claims.Role != "admin" {
		t.Fatalf("管理员令牌的 role 应为 admin，实际: %s", claims.Role)
	}
	if claims.Expires-time.Now().Unix() > int64(AdminTokenTTL.Seconds())+5 {
		t.Fatal("管理员令牌有效期超出预期")
	}
}
