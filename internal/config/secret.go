package config

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// randomSecret returns a 256 bit random secret encoded as hex.
func randomSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// nowISO returns the current timestamp in ISO-8601 form.
func nowISO() string { return time.Now().Format(time.RFC3339) }
