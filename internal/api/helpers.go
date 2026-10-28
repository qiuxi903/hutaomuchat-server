package api

import (
	"encoding/json"
	"strings"
)

// parseImageList accepts the image list of a moment either as a JSON array or
// as a JSON encoded string and normalises it into a slice of URLs.
func parseImageList(raw json.RawMessage) []string {
	out := []string{}
	if len(raw) == 0 {
		return out
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return out
	}
	if trimmed[0] == '"' {
		var encoded string
		if err := json.Unmarshal(raw, &encoded); err != nil {
			return out
		}
		trimmed = strings.TrimSpace(encoded)
		if trimmed == "" || trimmed == "null" {
			return out
		}
	}
	var list []string
	if err := json.Unmarshal([]byte(trimmed), &list); err != nil {
		return out
	}
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	if len(out) > 9 {
		out = out[:9]
	}
	return out
}
