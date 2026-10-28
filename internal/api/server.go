// Package api implements the HTTP and WebSocket surface of the server.
package api

import (
	"time"

	"github.com/hutaomu/hutaomuchat/server/internal/config"
	"github.com/hutaomu/hutaomuchat/server/internal/logging"
	"github.com/hutaomu/hutaomuchat/server/internal/push"
	"github.com/hutaomu/hutaomuchat/server/internal/store"
	"github.com/hutaomu/hutaomuchat/server/internal/ws"
)

// Server carries the dependencies shared by all handlers.
type Server struct {
	Store      *store.Store
	Config     *config.Manager
	Logger     *logging.Logger
	Hub        *ws.Hub
	Push       *push.Client
	UploadsDir string
	WebDir     string
	DataDir    string
	LogsDir    string

	StartedAt time.Time
}

// New creates the API server.
func New(
	st *store.Store,
	cfg *config.Manager,
	logger *logging.Logger,
	hub *ws.Hub,
	pushClient *push.Client,
	uploadsDir, webDir, dataDir, logsDir string,
) *Server {
	return &Server{
		Store:      st,
		Config:     cfg,
		Logger:     logger,
		Hub:        hub,
		Push:       pushClient,
		UploadsDir: uploadsDir,
		WebDir:     webDir,
		DataDir:    dataDir,
		LogsDir:    logsDir,
		StartedAt:  time.Now(),
	}
}

// jwtSecret returns the signing secret of the server.
func (s *Server) jwtSecret() string {
	return s.Config.JWTSecret()
}
