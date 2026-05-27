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

	"github.com/liusx/shadraw/internal/admin"
	"github.com/liusx/shadraw/internal/app"
	"github.com/liusx/shadraw/internal/auth"
	"github.com/liusx/shadraw/internal/blob"
	"github.com/liusx/shadraw/internal/config"
	"github.com/liusx/shadraw/internal/crypto"
	"github.com/liusx/shadraw/internal/httpx"
	"github.com/liusx/shadraw/internal/record"
	"github.com/liusx/shadraw/internal/store"
	"github.com/liusx/shadraw/internal/upstream"
	"github.com/liusx/shadraw/internal/user"
	"github.com/liusx/shadraw/internal/worker"
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

	cipher, err := crypto.New(cfg.MasterKey)
	if err != nil {
		return fmt.Errorf("crypto: %w", err)
	}

	blobStore, err := newBlobStore(rootCtx, cfg)
	if err != nil {
		return fmt.Errorf("blob store: %w", err)
	}

	// repositories
	userRepo := user.NewRepository(db.DB)
	refreshRepo := auth.NewRefreshRepository(db.DB)
	recordRepo := record.NewRepository(db.DB)
	projectRepo := record.NewProjectRepository(db.DB)

	// services
	authSvc := auth.NewService(userRepo, refreshRepo, cfg.JWTSecret, time.Now)
	authHandler := auth.NewHandler(authSvc)

	upstreamCli := upstream.NewClient()
	adminStore := admin.NewStore(db.DB, cipher)
	if err := adminStore.Load(rootCtx); err != nil {
		return fmt.Errorf("load upstream config: %w", err)
	}

	recordHandler := record.NewHandler(recordRepo, projectRepo, blobStore, adminStore)

	pool := worker.New(recordRepo, blobStore, upstreamCli, adminStore)
	adminStore.SetResizer(pool.Resize)

	adminHandler := admin.NewHandler(adminStore, userRepo, recordRepo, upstreamCli, authSvc)

	// boot: ensure admin, sweep stale running, start pool
	if err := app.EnsureAdmin(rootCtx, userRepo, cfg.AdminEmail); err != nil {
		return fmt.Errorf("ensure admin: %w", err)
	}
	if swept, err := recordRepo.SweepRunningToWaiting(rootCtx); err != nil {
		slog.Warn("sweep stale running", "err", err)
	} else if swept > 0 {
		slog.Info("swept stale running records", "count", swept)
	}
	pool.Start(adminStore.WorkerConcurrency())

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
		// public (rate-limited) auth endpoints
		v1.POST("/auth/register", httpx.RateLimit(5, time.Minute, httpx.KeyByIP), authHandler.RegisterEndpoint)
		v1.POST("/auth/login", httpx.RateLimit(5, time.Minute, httpx.KeyByIP), authHandler.LoginEndpoint)
		v1.POST("/auth/refresh", httpx.RateLimit(60, time.Minute, httpx.KeyByIP), authHandler.RefreshEndpoint)

		// public config (enabled models + site title so the front-end can hydrate shells)
		v1.GET("/config", func(c *gin.Context) {
			httpx.OK(c, adminStore.AppConfig())
		})

		// authenticated user-scope endpoints
		secured := v1.Group("")
		secured.Use(auth.RequireAuth(cfg.JWTSecret, userRepo))
		secured.POST("/auth/logout", httpx.RateLimit(60, time.Minute, httpx.KeyByUser), authHandler.LogoutEndpoint)
		secured.GET("/auth/me", authHandler.MeEndpoint)
		secured.POST("/auth/password", httpx.RateLimit(10, time.Minute, httpx.KeyByUser), authHandler.ChangePasswordEndpoint)

		secured.POST("/records", httpx.RateLimit(30, time.Minute, httpx.KeyByUser), func(c *gin.Context) {
			recordHandler.Create(c)
			pool.Wake()
		})
		secured.GET("/records", recordHandler.List)
		secured.GET("/records/:id", recordHandler.Get)
		secured.PATCH("/records/:id", recordHandler.Update)
		secured.POST("/records/:id/retry", httpx.RateLimit(30, time.Minute, httpx.KeyByUser), func(c *gin.Context) {
			recordHandler.Retry(c)
			pool.Wake()
		})
		secured.DELETE("/records/:id", recordHandler.Delete)
		secured.GET("/images/:id", recordHandler.StreamImage)

		secured.GET("/projects", recordHandler.ListProjects)
		secured.POST("/projects", recordHandler.CreateProject)
		secured.PATCH("/projects/:id", recordHandler.RenameProject)
		secured.DELETE("/projects/:id", recordHandler.DeleteProject)

		// admin-only endpoints
		adminGroup := secured.Group("/admin")
		adminGroup.Use(admin.RequireAdmin())
		adminGroup.GET("/upstream-configs", adminHandler.GetUpstream)
		adminGroup.PUT("/upstream-configs", adminHandler.UpdateUpstream)
		adminGroup.POST("/upstream-configs/test", adminHandler.TestUpstream)
		adminGroup.GET("/runtime", adminHandler.GetRuntime)
		adminGroup.PATCH("/runtime", adminHandler.UpdateRuntime)
		adminGroup.GET("/site-settings", adminHandler.GetSite)
		adminGroup.PATCH("/site-settings", adminHandler.UpdateSite)
		adminGroup.GET("/users", adminHandler.ListUsers)
		adminGroup.PATCH("/users/:id", adminHandler.UpdateUser)
		adminGroup.POST("/users/:id/reset-password", adminHandler.ResetPassword)
		adminGroup.GET("/records", adminHandler.ListRecords)
		adminGroup.DELETE("/records/:id", adminHandler.DeleteRecord)
		adminGroup.GET("/stats/overview", adminHandler.StatsOverview)
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

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := pool.Stop(shutdownCtx); err != nil {
		slog.Warn("worker pool stop", "err", err)
	}
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}
	slog.Info("server stopped")
	return nil
}

func newBlobStore(ctx context.Context, cfg *config.Config) (blob.Store, error) {
	switch cfg.BlobDriver {
	case "s3":
		return blob.NewS3(ctx, blob.S3Config{
			Endpoint:     cfg.S3Endpoint,
			Region:       cfg.S3Region,
			Bucket:       cfg.S3Bucket,
			AccessKey:    cfg.S3AccessKey,
			SecretKey:    cfg.S3SecretKey,
			UsePathStyle: cfg.S3UsePathStyle,
		})
	default:
		return blob.NewLocalFS(cfg.DataDir)
	}
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
