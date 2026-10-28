package api

import (
	"os"
	"path/filepath"

	"github.com/hutaomu/hutaomuchat/server/internal/push"
	"github.com/hutaomu/hutaomuchat/server/internal/security"
)

// RestartHandler is invoked when an administrator requests a restart. The
// main package installs a handler that shuts the server down gracefully.
var RestartHandler func()

// restartServer triggers the configured restart handler.
func restartServer() {
	if RestartHandler != nil {
		RestartHandler()
	}
}

// signAdminToken issues a token for the administration console.
func signAdminToken(secret, username string) (string, error) {
	return security.SignToken(secret, security.NewAdminClaims(username))
}

// getuiConfigPath returns the location of the GeTui configuration file.
func (s *Server) getuiConfigPath() string {
	return filepath.Join(s.DataDir, "getui.config.json")
}

// saveGetuiConfig stores the GeTui credentials and refreshes the client.
func (s *Server) saveGetuiConfig(appID, appKey, masterSecret string) error {
	if err := push.SaveConfig(s.getuiConfigPath(), push.Config{
		AppID:        appID,
		AppKey:       appKey,
		MasterSecret: masterSecret,
	}); err != nil {
		return err
	}
	s.Push = push.New(push.Config{AppID: appID, AppKey: appKey, MasterSecret: masterSecret})
	return nil
}

// resetEverything clears the data of the server and removes the uploaded
// files.
func (s *Server) resetEverything() {
	s.Store.Reset()
	s.Hub.Broadcast(map[string]any{
		"type":   "server_reset",
		"notice": "服务器数据已被管理员清空",
	})
	if entries, err := os.ReadDir(s.UploadsDir); err == nil {
		for _, entry := range entries {
			if entry.Name() == ".gitkeep" {
				continue
			}
			os.Remove(filepath.Join(s.UploadsDir, entry.Name()))
		}
	}
}
