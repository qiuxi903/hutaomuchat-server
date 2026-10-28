// Command hutaomuchat-server runs the HutaomuChat backend.
//
// The server exposes a JSON API for the Android client, a WebSocket channel
// for realtime messaging and a web based administration console.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/hutaomu/hutaomuchat/server/internal/api"
	"github.com/hutaomu/hutaomuchat/server/internal/config"
	"github.com/hutaomu/hutaomuchat/server/internal/logging"
	"github.com/hutaomu/hutaomuchat/server/internal/models"
	"github.com/hutaomu/hutaomuchat/server/internal/push"
	"github.com/hutaomu/hutaomuchat/server/internal/security"
	"github.com/hutaomu/hutaomuchat/server/internal/store"
	"github.com/hutaomu/hutaomuchat/server/internal/ws"
)

// version is the release identifier of the server.
const version = "2.0.0-go"

func main() {
	baseDir := resolveBaseDir()
	dataDir := filepath.Join(baseDir, "data")
	uploadsDir := filepath.Join(baseDir, "uploads")
	logsDir := filepath.Join(baseDir, "logs")
	webDir := filepath.Join(baseDir, "web")

	applyTokenSettings()

	for _, dir := range []string{dataDir, uploadsDir, logsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "无法创建目录 %s: %v\n", dir, err)
			os.Exit(1)
		}
	}

	logger, err := logging.New(logsDir, logging.ParseLevel(os.Getenv("LOG_LEVEL")))
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
		os.Exit(1)
	}

	cfg := config.NewManager(filepath.Join(dataDir, "server.config.json"))
	st := store.New()
	persister := store.NewPersister(filepath.Join(dataDir, "store.json"))
	if loaded, err := persister.Load(st); err != nil {
		logger.Error("读取数据快照失败", map[string]any{"error": err.Error()})
	} else if loaded {
		logger.Info("数据快照已载入", map[string]any{
			"users":    st.CountUsers(),
			"chats":    len(st.AllChats()),
			"messages": st.TotalMessages(),
		})
	}
	persister.StartAutoSave(st, 30*time.Second, func(err error) {
		logger.Error("保存数据快照失败", map[string]any{"error": err.Error()})
	})

	hub := ws.NewHub()
	hub.OnStatusChange(func(userID string, online bool) {
		status := models.Offline
		if online {
			status = models.Online
		}
		st.UpdateUser(userID, func(u *models.User) {
			u.OnlineStatus = status
			if !online {
				u.LastSeenAt = models.Now()
			}
		})
		for _, friendID := range st.FriendIDs(userID) {
			hub.SendToUser(friendID, map[string]any{
				"type":   "online_status",
				"userId": userID,
				"status": status,
			})
		}
	})

	pushCfg, _ := push.LoadConfig(filepath.Join(dataDir, "getui.config.json"))
	pushClient := push.New(pushCfg)

	server := api.New(st, cfg, logger, hub, pushClient, uploadsDir, webDir, dataDir, logsDir)
	server.StartUploadCleanup()
	if removed := server.CleanupUploads(); removed > 0 {
		logger.Info("清理了未引用的上传文件", map[string]any{"count": removed})
	}

	port := cfg.ServerPort()
	if envPort := os.Getenv("PORT"); envPort != "" {
		fmt.Sscanf(envPort, "%d", &port)
	}

	httpServer := &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%d", port),
		Handler:           server.Routes(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	shutdown := make(chan struct{})
	api.RestartHandler = func() {
		close(shutdown)
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP 服务异常退出", map[string]any{"error": err.Error()})
		}
	}()

	printBanner(cfg, port)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	select {
	case <-signals:
		logger.Info("收到退出信号，正在关闭服务端")
	case <-shutdown:
		logger.Info("收到重启请求，正在关闭服务端")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("关闭 HTTP 服务失败", map[string]any{"error": err.Error()})
	}
	if err := persister.Save(st); err != nil {
		logger.Error("保存数据快照失败", map[string]any{"error": err.Error()})
	} else {
		logger.Info("数据快照已保存")
	}
}

// applyTokenSettings reads the token lifetimes from the environment.
func applyTokenSettings() {
	if value := os.Getenv("ACCESS_TOKEN_TTL"); value != "" {
		if days, err := strconv.Atoi(value); err == nil && days > 0 {
			security.SetUserTokenTTL(time.Duration(days) * 24 * time.Hour)
		}
	}
	if value := os.Getenv("REFRESH_TOKEN_TTL"); value != "" {
		if days, err := strconv.Atoi(value); err == nil && days > 0 {
			security.SetRefreshTokenTTL(time.Duration(days) * 24 * time.Hour)
		}
	}
}

// resolveBaseDir returns the directory the server stores its data in.
func resolveBaseDir() string {
	if dir := os.Getenv("HUTAOMUCHAT_DATA_DIR"); dir != "" {
		return dir
	}
	executable, err := os.Executable()
	if err == nil {
		return filepath.Dir(executable)
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// printBanner writes the startup banner to the console.
func printBanner(cfg *config.Manager, port int) {
	name := cfg.ServerName()
	fmt.Println()
	fmt.Printf("  %s 服务端 v%s\n", name, version)
	fmt.Println("  ------------------------------------------------")
	fmt.Printf("  HTTP    : http://0.0.0.0:%d\n", port)
	fmt.Printf("  控制台  : http://0.0.0.0:%d/admin/\n", port)
	fmt.Printf("  WebSocket: ws://0.0.0.0:%d/ws\n", port)
	if !cfg.Initialized() {
		fmt.Printf("  初始化  : http://0.0.0.0:%d/admin/setup\n", port)
	}
	fmt.Println("  ------------------------------------------------")
	fmt.Println("  基于 Go 标准库重写 | AGPL-3.0 License")
	fmt.Println()
}
