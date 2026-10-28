// Package config manages the persistent server configuration that is written
// during the first-run setup wizard.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/hutaomu/hutaomuchat/server/internal/security"
)

// Config is the persisted server configuration.
type Config struct {
	Initialized   bool   `json:"initialized"`
	ServerName    string `json:"serverName"`
	ServerPort    int    `json:"serverPort"`
	AdminUser     string `json:"adminUser"`
	AdminPassHash string `json:"adminPassHash"`
	JWTSecret     string `json:"jwtSecret"`
	SetupTime     string `json:"setupTime"`
}

// Default values used when nothing has been configured yet.
const (
	DefaultServerName = "HutaomuChat"
	DefaultServerPort = 3000
	DefaultAdminUser  = "admin"
)

// Manager owns the configuration file and guards it with a mutex.
type Manager struct {
	path string
	mu   sync.RWMutex
	cfg  *Config
}

// NewManager creates a manager for the given configuration file.
func NewManager(path string) *Manager {
	m := &Manager{path: path}
	m.cfg = m.read()
	return m
}

// Path returns the configuration file path.
func (m *Manager) Path() string { return m.path }

// read loads the configuration from disk, returning defaults when absent.
func (m *Manager) read() *Config {
	cfg := &Config{
		ServerName: DefaultServerName,
		ServerPort: DefaultServerPort,
		AdminUser:  DefaultAdminUser,
	}
	data, err := os.ReadFile(m.path)
	if err != nil {
		return cfg
	}
	var stored Config
	if err := json.Unmarshal(data, &stored); err != nil {
		return cfg
	}
	if stored.ServerName == "" {
		stored.ServerName = DefaultServerName
	}
	if stored.ServerPort == 0 {
		stored.ServerPort = DefaultServerPort
	}
	if stored.AdminUser == "" {
		stored.AdminUser = DefaultAdminUser
	}
	return &stored
}

// Initialized reports whether the first-run setup has been completed.
func (m *Manager) Initialized() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.Initialized
}

// Get returns a copy of the current configuration.
func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *m.cfg
}

// Update applies the given mutation to the configuration and persists it.
func (m *Manager) Update(mutate func(*Config)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	mutate(m.cfg)
	return m.writeLocked()
}

// SetAdminPassword hashes and stores the administration password.
func (m *Manager) SetAdminPassword(password string) error {
	hash, err := security.HashPassword(password)
	if err != nil {
		return err
	}
	return m.Update(func(c *Config) { c.AdminPassHash = hash })
}

// VerifyAdminPassword checks the administration credentials.
func (m *Manager) VerifyAdminPassword(user, password string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cfg.AdminUser != user || m.cfg.AdminPassHash == "" {
		security.DummyVerify(password)
		return false
	}
	return security.VerifyPassword(password, m.cfg.AdminPassHash)
}

func (m *Manager) writeLocked() error {
	data, err := json.MarshalIndent(m.cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}

// ErrAlreadyInitialized is returned when the setup wizard runs twice.
var ErrAlreadyInitialized = errors.New("服务器已初始化，如需修改请在管理后台操作")

// ApplySetup stores the configuration produced by the setup wizard.
func (m *Manager) ApplySetup(serverName string, serverPort int, adminUser, adminPass, jwtSecret string) error {
	if m.Initialized() {
		return ErrAlreadyInitialized
	}
	hash, err := security.HashPassword(adminPass)
	if err != nil {
		return err
	}
	if jwtSecret == "" {
		secret, err := randomSecret()
		if err != nil {
			return err
		}
		jwtSecret = secret
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = &Config{
		Initialized:   true,
		ServerName:    serverName,
		ServerPort:    serverPort,
		AdminUser:     adminUser,
		AdminPassHash: hash,
		JWTSecret:     jwtSecret,
		SetupTime:     nowISO(),
	}
	return m.writeLocked()
}

// JWTSecret returns the signing secret, generating one when necessary.
func (m *Manager) JWTSecret() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cfg.JWTSecret == "" {
		if secret, err := randomSecret(); err == nil {
			m.cfg.JWTSecret = secret
			_ = m.writeLocked()
		}
	}
	return m.cfg.JWTSecret
}

// ServerName returns the display name of the server.
func (m *Manager) ServerName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.ServerName
}

// ServerPort returns the configured listen port.
func (m *Manager) ServerPort() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.ServerPort
}

// AdminUser returns the administration user name.
func (m *Manager) AdminUser() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.AdminUser
}
