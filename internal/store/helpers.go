package store

import (
	"crypto/rand"
	"encoding/hex"
	randv2 "math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/hutaomu/hutaomuchat/server/internal/models"
)

// newID returns a random 128 bit identifier encoded as hex.
func newID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}

// NewID exposes the identifier generator to other packages.
func NewID() string { return newID() }

// timeNowUTCDate returns the current UTC date as YYYY-MM-DD.
func timeNowUTCDate() string { return time.Now().UTC().Format("2006-01-02") }

// parseNumericSuffix extracts the numeric part of ids such as htm-042.
func parseNumericSuffix(s string) int {
	idx := strings.LastIndex(s, "-")
	if idx < 0 || idx == len(s)-1 {
		return 0
	}
	n, err := strconv.Atoi(s[idx+1:])
	if err != nil {
		return 0
	}
	return n
}

func removeString(list []string, value string) []string {
	out := list[:0]
	for _, v := range list {
		if v != value {
			out = append(out, v)
		}
	}
	return out
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

// shuffle randomises a slice in place using the maths/rand/v2 generator.
func shuffle[T any](list []T) {
	for i := len(list) - 1; i > 0; i-- {
		j := randv2.IntN(i + 1)
		list[i], list[j] = list[j], list[i]
	}
}

func filterFriendships(list []models.Friendship, remove ...string) []models.Friendship {
	out := list[:0]
	for _, f := range list {
		skip := false
		for _, r := range remove {
			if f.UserID == r || f.FriendID == r {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, f)
		}
	}
	return out
}

func filterFriendRequests(list []models.FriendRequest, remove ...string) []models.FriendRequest {
	out := list[:0]
	for _, r := range list {
		skip := false
		for _, id := range remove {
			if r.FromUserID == id || r.ToUserID == id {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, r)
		}
	}
	return out
}

func filterStickers(list []*models.Sticker, remove string) []*models.Sticker {
	out := list[:0]
	for _, st := range list {
		if st.UserID != remove {
			out = append(out, st)
		}
	}
	return out
}

func filterAuditLogs(list []*models.AuditLog, remove string) []*models.AuditLog {
	out := list[:0]
	for _, l := range list {
		if l.Target != remove {
			out = append(out, l)
		}
	}
	return out
}
