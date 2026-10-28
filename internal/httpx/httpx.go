// Package httpx contains the HTTP plumbing shared by all API handlers:
// JSON encoding helpers, error responses and middleware.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// MaxBodySize is the maximum size of a request body accepted by the API.
const MaxBodySize = 8 << 20 // 8 MiB

// ErrorResponse is the canonical error payload of the API.
type ErrorResponse struct {
	Error string `json:"error"`
}

// WriteJSON writes v as a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return
	}
}

// WriteError writes a JSON error response.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, ErrorResponse{Error: message})
}

// WriteOK writes a plain success payload.
func WriteOK(w http.ResponseWriter) {
	WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// WriteSuccess writes a success payload with an optional message.
func WriteSuccess(w http.ResponseWriter, message string) {
	WriteJSON(w, http.StatusOK, map[string]any{"success": true, "message": message})
}

// DecodeJSON reads a JSON request body into v.
func DecodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return errors.New("请求体为空")
	}
	reader := http.MaxBytesReader(nil, r.Body, MaxBodySize)
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()
	if err := decoder.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("请求体为空")
		}
		return fmt.Errorf("JSON 解析失败: %w", err)
	}
	return nil
}

// ClientIP extracts the address of the client, honouring proxy headers.
func ClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	if real := r.Header.Get("X-Real-IP"); real != "" {
		return strings.TrimSpace(real)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Chain composes middleware into a single handler.
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
