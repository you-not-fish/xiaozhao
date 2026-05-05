// Package main is the composition root of the xiaozhao backend. It wires
// config, logger, database, cache, domain repositories, application
// services, HTTP handlers and the chi router together, then runs the HTTP
// server with graceful shutdown.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/xiaozhao/xiaozhao/internal/api/router"
	v1 "github.com/xiaozhao/xiaozhao/internal/api/v1"
	agentapp "github.com/xiaozhao/xiaozhao/internal/app/agent"
	authapp "github.com/xiaozhao/xiaozhao/internal/app/auth"
	fileapp "github.com/xiaozhao/xiaozhao/internal/app/file"
	knowledgeapp "github.com/xiaozhao/xiaozhao/internal/app/knowledge"
	orgapp "github.com/xiaozhao/xiaozhao/internal/app/org"
	projectapp "github.com/xiaozhao/xiaozhao/internal/app/project"
	"github.com/xiaozhao/xiaozhao/internal/app/rbac"
	toolapp "github.com/xiaozhao/xiaozhao/internal/app/tool"
	"github.com/xiaozhao/xiaozhao/internal/infra/cache"
	"github.com/xiaozhao/xiaozhao/internal/infra/database"
	"github.com/xiaozhao/xiaozhao/internal/infra/embedding"
	"github.com/xiaozhao/xiaozhao/internal/infra/migration"
	"github.com/xiaozhao/xiaozhao/internal/infra/model"
	"github.com/xiaozhao/xiaozhao/internal/infra/parser"
	"github.com/xiaozhao/xiaozhao/internal/infra/repository"
	"github.com/xiaozhao/xiaozhao/internal/infra/storage"
	"github.com/xiaozhao/xiaozhao/internal/observability"
	"github.com/xiaozhao/xiaozhao/internal/pkg/config"
	"github.com/xiaozhao/xiaozhao/internal/pkg/jwt"
	"github.com/xiaozhao/xiaozhao/internal/pkg/logger"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config.yaml")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load failed: %v\n", err)
		os.Exit(1)
	}

	lg, err := logger.New(cfg.Log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger init failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = lg.Sync() }()

	if err := run(cfg, lg); err != nil {
		lg.Error("server exited with error", zap.Error(err))
		os.Exit(1)
	}
}

func run(cfg *config.Config, lg *zap.Logger) error {
	ctx, cancel := context.WithCancel(context.Background())

	db, err := database.Open(ctx, cfg.Postgres, lg)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer func() { _ = database.Close(db) }()

	if cfg.Postgres.AutoMigrate {
		if err := migration.Apply(ctx, db, lg); err != nil {
			return fmt.Errorf("apply migrations: %w", err)
		}
	}

	rdb, err := cache.Open(ctx, cfg.Redis)
	if err != nil {
		// Redis outage shouldn't block boot — rate limiting will fail-open.
		lg.Warn("redis unavailable, rate limiting disabled", zap.Error(err))
		rdb = nil
	}
	defer func() {
		if rdb != nil {
			_ = rdb.Close()
		}
	}()

	// Repositories.
	userRepo := repository.NewUserRepo(db)
	orgRepo := repository.NewOrgRepo(db)
	memberRepo := repository.NewOrgMemberRepo(db)
	projectRepo := repository.NewProjectRepo(db)
	conversationRepo := repository.NewConversationRepo(db)
	messageRepo := repository.NewMessageRepo(db)
	eventRepo := repository.NewResponseEventRepo(db)
	modelInvocationRepo := repository.NewModelInvocationRepo(db)
	toolCallRepo := repository.NewToolCallRepo(db)
	traceSpanRepo := repository.NewTraceSpanRepo(db)
	auditRepo := repository.NewAuditLogRepo(db)
	usageRepo := repository.NewUsageRepo(db)
	fileRepo := repository.NewFileRepo(db)
	fileObjectRepo := repository.NewFileObjectRepo(db)
	knowledgeRepo := repository.NewKnowledgeBaseRepo(db)
	documentRepo := repository.NewDocumentRepo(db)
	documentChunkRepo := repository.NewDocumentChunkRepo(db)

	// Cross-cutting.
	rbacChecker := rbac.NewChecker(memberRepo)
	jwtMgr := jwt.NewManager(cfg.Auth.JWTSecret, cfg.Auth.JWTIssuer, cfg.Auth.AccessTokenTTL())
	tracer := observability.NewTracer(traceSpanRepo)
	modelRouter, err := model.NewRouter(cfg.Model)
	if err != nil {
		return fmt.Errorf("init model router: %w", err)
	}
	embeddingProvider, err := embedding.NewProvider(cfg.Embedding)
	if err != nil {
		return fmt.Errorf("init embedding provider: %w", err)
	}
	toolRegistry := toolapp.NewRegistry()
	if err := toolRegistry.Register(toolapp.NewCurrentTime()); err != nil {
		return fmt.Errorf("register current_time: %w", err)
	}
	if err := toolRegistry.Register(toolapp.NewCalculator()); err != nil {
		return fmt.Errorf("register calculator: %w", err)
	}
	objectStore, err := storage.NewMinIOStore(ctx, storage.MinIOConfig{
		Endpoint:         cfg.Storage.Endpoint,
		AccessKey:        cfg.Storage.AccessKey,
		SecretKey:        cfg.Storage.SecretKey,
		Bucket:           cfg.Storage.Bucket,
		Region:           cfg.Storage.Region,
		UseSSL:           cfg.Storage.UseSSL,
		AutoCreateBucket: cfg.Storage.AutoCreateBucket,
	})
	if err != nil {
		return fmt.Errorf("init object storage: %w", err)
	}

	// Application services.
	authSvc := authapp.NewService(userRepo, jwtMgr, cfg.Auth.PasswordMinLength)
	orgSvc := orgapp.NewService(orgRepo, memberRepo, userRepo, rbacChecker)
	projectSvc := projectapp.NewService(projectRepo, orgRepo, rbacChecker)
	fileSvc := fileapp.NewService(fileRepo, fileObjectRepo, projectRepo, auditRepo, rbacChecker, objectStore, fileapp.Options{
		Bucket:         cfg.Storage.Bucket,
		Provider:       cfg.Storage.Provider,
		MaxUploadBytes: cfg.Storage.MaxUploadBytes(),
	})
	knowledgeSvc := knowledgeapp.NewService(
		knowledgeRepo, documentRepo, documentChunkRepo,
		fileRepo, fileObjectRepo, projectRepo,
		auditRepo, usageRepo, rbacChecker,
		objectStore, parser.NewDefaultParser(), embeddingProvider,
		knowledgeapp.Options{ChunkSize: 1000, ChunkOverlap: 150},
	)
	waitKnowledgeWorker := knowledgeSvc.StartWorker(ctx, 5*time.Second, 10)
	if err := toolRegistry.Register(toolapp.NewKnowledgeSearch(knowledgeSvc)); err != nil {
		return fmt.Errorf("register knowledge_search: %w", err)
	}
	conversationSvc := agentapp.NewConversationService(conversationRepo, projectRepo, rbacChecker)
	orchestrator := agentapp.NewDefaultOrchestrator(agentapp.OrchestratorDeps{
		Conversations: conversationSvc,
		Messages:      messageRepo,
		Events:        eventRepo,
		ModelProvider: modelRouter.Primary(),
		ModelName:     modelRouter.DefaultModel(),
		Tools:         toolRegistry,
		ModelCalls:    modelInvocationRepo,
		ToolCalls:     toolCallRepo,
		Audit:         auditRepo,
		Usage:         usageRepo,
		Tracer:        tracer,
		Config:        cfg.Agent,
	})

	// HTTP handlers.
	authHandler := v1.NewAuthHandler(authSvc, orgSvc)
	orgHandler := v1.NewOrgHandler(orgSvc)
	projectHandler := v1.NewProjectHandler(projectSvc)
	responseHandler := v1.NewResponseHandler(orchestrator, eventRepo)
	fileHandler := v1.NewFileHandler(fileSvc, cfg.Storage.MaxUploadBytes())
	knowledgeHandler := v1.NewKnowledgeHandler(knowledgeSvc)
	healthHandler := v1.NewHealthHandler()

	handler := router.New(router.Deps{
		Log:       lg,
		Config:    cfg,
		JWT:       jwtMgr,
		Redis:     rdb,
		RBAC:      rbacChecker,
		Auth:      authHandler,
		Org:       orgHandler,
		Project:   projectHandler,
		Response:  responseHandler,
		File:      fileHandler,
		Knowledge: knowledgeHandler,
		Health:    healthHandler,
	})

	srv := &http.Server{
		Addr:         cfg.Server.Addr(),
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout(),
		WriteTimeout: cfg.Server.WriteTimeout(),
	}

	errCh := make(chan error, 1)
	go func() {
		lg.Info("http server listening", zap.String("addr", cfg.Server.Addr()))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		lg.Info("received signal, shutting down", zap.String("signal", sig.String()))
	case err := <-errCh:
		lg.Error("server error", zap.Error(err))
		cancel()
		waitKnowledgeWorker()
		return err
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout())
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		cancel()
		waitKnowledgeWorker()
		return fmt.Errorf("shutdown: %w", err)
	}
	cancel()
	waitKnowledgeWorker()
	lg.Info("server stopped cleanly")
	_ = shutdownCtx
	_ = time.Now
	return nil
}
