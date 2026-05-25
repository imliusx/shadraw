// Package main starts the shadraw API server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liusx/shadraw/internal/app"
	"github.com/liusx/shadraw/internal/auth"
	"github.com/liusx/shadraw/internal/config"
	"github.com/liusx/shadraw/internal/httpx"
	"github.com/liusx/shadraw/internal/store"
	"github.com/liusx/shadraw/internal/user"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	initLogger(cfg.LogLevel)

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.RunMigrations(rootCtx, "migrations", cfg.DBDSN); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	db, err := store.Open(rootCtx, cfg.DBDSN)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			slog.Warn("db close", "err", cerr)
		}
	}()

	userRepo := user.NewRepository(db.DB)
	refreshRepo := auth.NewRefreshRepository(db.DB)
	authSvc := auth.NewService(userRepo, refreshRepo, cfg.JWTSecret, time.Now)
	authHandler := auth.NewHandler(authSvc)

	if err := app.EnsureAdmin(rootCtx, userRepo, cfg.AdminEmail); err != nil {
		return fmt.Errorf("ensure admin: %w", err)
	}

	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(
		httpx.RequestID(),
		httpx.Logger(),
		httpx.Recovery(),
		httpx.SecurityHeaders(),
		httpx.CORS(cfg.CORSOrigins),
	)

	engine.GET("/healthz", func(c *gin.Context) {
		httpx.OK(c, gin.H{"status": "ok"})
	})

	v1 := engine.Group("/api/v1")
	{
		// register / login / refresh are unauthenticated but rate-limited per spec.
		authPublic := v1.Group("")
		authPublic.POST("/auth/register",
			httpx.RateLimit(5, time.Minute, httpx.KeyByIP), authHandler.RegisterEndpoint)
		authPublic.POST("/auth/login",
			httpx.RateLimit(5, time.Minute, httpx.KeyByIP), authHandler.LoginEndpoint)
		authPublic.POST("/auth/refresh",
			httpx.RateLimit(60, time.Minute, httpx.KeyByIP), authHandler.RefreshEndpoint)

		secured := v1.Group("")
		secured.Use(auth.RequireAuth(cfg.JWTSecret, userRepo))
		secured.POST("/auth/logout",
			httpx.RateLimit(60, time.Minute, httpx.KeyByUser), authHandler.LogoutEndpoint)
		secured.GET("/auth/me", authHandler.MeEndpoint)
		secured.POST("/auth/password",
			httpx.RateLimit(10, time.Minute, httpx.KeyByUser), authHandler.ChangePasswordEndpoint)
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("server listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "err", err)
			cancel()
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-stop:
		slog.Info("shutdown signal received")
	case <-rootCtx.Done():
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}
	slog.Info("server stopped")
	return nil
}

func initLogger(level string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(h))
}
