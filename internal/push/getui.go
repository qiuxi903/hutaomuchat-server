// Package push integrates the server with the GeTui UniPush 2.0 REST API.
//
// Push notifications are delivered through the system level push channel of
// the device vendor, so they still arrive when the application process has
// been terminated by the operating system.
package push

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Config holds the GeTui credentials.
type Config struct {
	AppID        string `json:"appId"`
	AppKey       string `json:"appKey"`
	MasterSecret string `json:"masterSecret"`
}

// Client talks to the GeTui REST API.
type Client struct {
	baseURL string
	cfg     Config
	http    *http.Client

	mu          sync.Mutex
	authToken   string
	tokenExpiry time.Time
}

// New creates a push client from the given configuration.
func New(cfg Config) *Client {
	return &Client{
		baseURL: "https://restapi.getui.com/v2",
		cfg:     cfg,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// LoadConfig reads the GeTui configuration from disk.
func LoadConfig(path string) (Config, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, false
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, false
	}
	if cfg.AppID == "" || cfg.AppKey == "" || cfg.MasterSecret == "" {
		return Config{}, false
	}
	return cfg, true
}

// SaveConfig writes the GeTui configuration to disk.
func SaveConfig(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Available reports whether push notifications are configured.
func (c *Client) Available() bool {
	return c != nil && c.cfg.AppID != "" && c.cfg.AppKey != "" && c.cfg.MasterSecret != ""
}

// Status returns a short status string for the administration console.
func (c *Client) Status() string {
	if !c.Available() {
		return "unconfigured"
	}
	return "active"
}

func (c *Client) request(method, endpoint string, body any) (map[string]any, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, c.baseURL+endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.mu.Lock()
	if c.authToken != "" {
		req.Header.Set("token", c.authToken)
	}
	c.mu.Unlock()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("个推响应解析失败: %w", err)
	}
	return decoded, nil
}

// authenticate obtains (and caches) the REST API token.
func (c *Client) authenticate() bool {
	if !c.Available() {
		return false
	}
	c.mu.Lock()
	if c.authToken != "" && time.Now().Before(c.tokenExpiry) {
		c.mu.Unlock()
		return true
	}
	c.mu.Unlock()

	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	sum := sha256.Sum256([]byte(c.cfg.AppKey + timestamp + c.cfg.MasterSecret))
	sign := hex.EncodeToString(sum[:])

	result, err := c.request(http.MethodPost, "/"+c.cfg.AppID+"/auth", map[string]any{
		"sign":      sign,
		"timestamp": timestamp,
		"appkey":    c.cfg.AppKey,
	})
	if err != nil {
		return false
	}
	code, _ := result["code"].(float64)
	if int(code) != 0 {
		return false
	}
	data, _ := result["data"].(map[string]any)
	token, _ := data["token"].(string)
	if token == "" {
		return false
	}
	expire := int64(86400)
	if v, ok := data["expire_time"].(float64); ok && v > 0 {
		expire = int64(v)
	}
	c.mu.Lock()
	c.authToken = token
	c.tokenExpiry = time.Now().Add(time.Duration(expire)*time.Second - time.Hour)
	c.mu.Unlock()
	return true
}

// SendToUsers delivers a notification to the given client identifiers.
func (c *Client) SendToUsers(cids []string, title, body string, payload map[string]any) error {
	if !c.Available() {
		return fmt.Errorf("个推未配置")
	}
	if len(cids) == 0 {
		return nil
	}
	if !c.authenticate() {
		return fmt.Errorf("个推认证失败")
	}
	encodedPayload, _ := json.Marshal(payload)
	create, err := c.request(http.MethodPost, "/"+c.cfg.AppID+"/push/list/message", map[string]any{
		"request_id": randomRequestID(),
		"group_name": "hutaomuchat_msg",
		"push_message": map[string]any{
			"notification": map[string]any{
				"title":      title,
				"body":       body,
				"click_type": "startapp",
				"payload":    string(encodedPayload),
			},
		},
	})
	if err != nil {
		return err
	}
	if code, _ := create["code"].(float64); int(code) != 0 {
		return fmt.Errorf("个推创建消息失败: %v", create["msg"])
	}
	data, _ := create["data"].(map[string]any)
	taskID, _ := data["task_id"].(string)
	if taskID == "" {
		return fmt.Errorf("个推未返回任务编号")
	}
	send, err := c.request(http.MethodPost, "/"+c.cfg.AppID+"/push/list/cid", map[string]any{
		"audience": map[string]any{"cid": cids},
		"taskid":   taskID,
		"is_async": true,
	})
	if err != nil {
		return err
	}
	if code, _ := send["code"].(float64); int(code) != 0 {
		return fmt.Errorf("个推发送失败: %v", send["msg"])
	}
	return nil
}

func randomRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
