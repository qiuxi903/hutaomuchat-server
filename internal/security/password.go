package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"crypto/pbkdf2"
	"crypto/sha256"
)

// Password storage parameters.
const (
	passwordAlgorithm  = "pbkdf2_sha256"
	passwordIterations = 210000
	passwordSaltLen    = 16
	passwordKeyLen     = 32
)

// ErrInvalidPasswordHash is returned when a stored hash cannot be parsed.
var ErrInvalidPasswordHash = errors.New("密码哈希格式无效")

// HashPassword derives a salted PBKDF2-HMAC-SHA256 hash of the password.
//
// The returned string embeds the algorithm, iteration count, salt and derived
// key so that the parameters can be upgraded later without invalidating
// existing accounts.
func HashPassword(password string) (string, error) {
	salt := make([]byte, passwordSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, passwordKeyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s$%d$%s$%s",
		passwordAlgorithm,
		passwordIterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword checks a password against a stored hash in constant time.
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordAlgorithm {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(expected))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(key, expected) == 1
}

// DummyPasswordHash is used to equalise the response time of a login attempt
// for an unknown account.
var DummyPasswordHash, _ = HashPassword("hutaomu-dummy-password")

// DummyVerify burns the same amount of time as a real verification.
func DummyVerify(password string) {
	_ = VerifyPassword(password, DummyPasswordHash)
}
