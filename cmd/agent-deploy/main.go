package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/cjmartian/agent-deploy/internal/awsclient"
	"github.com/cjmartian/agent-deploy/internal/logging"
	"github.com/cjmartian/agent-deploy/internal/providers"
	"github.com/cjmartian/agent-deploy/internal/spending"
	"github.com/cjmartian/agent-deploy/internal/state"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version is the application version. It can be overridden at build time via ldflags:
//
//	go build -ldflags "-X main.Version=v1.2.3" ./cmd/agent-deploy
var Version = "v0.1.0"

var (
	httpAddr           = flag.String("http", "", "if set, use streamable HTTP at this address instead of stdin/stdout")
	logLevel           = flag.String("log-level", "info", "log level: debug, info, warn, error")
	logFormat          = flag.String("log-format", "text", "log format: text, json")
	enableCostMonitor  = flag.Bool("enable-cost-monitor", false, "enable runtime cost monitoring (requires AWS credentials)")
	enableAutoTeardown = flag.Bool("enable-auto-teardown", false, "enable automatic teardown of over-budget deployments")
	enableReconcile    = flag.Bool("enable-reconcile", false, "enable state reconciliation on startup (requires AWS credentials)")
	reconcileRegion    = flag.String("reconcile-region", "us-east-1", "AWS region for state reconciliation")
)

func main() {
	flag.Parse()

	// Initialize structured logging.
	logging.Initialize(
		logging.WithLevel(logging.ParseLevel(*logLevel)),
		logging.WithFormat(logging.ParseFormat(*logFormat)),
	)

	log := logging.WithComponent(logging.ComponentServer)
	log.Info("starting agent-deploy server",
		slog.String("version", Version),
		slog.String("log_level", *logLevel),
	)

	// Initialize state store.
	store, err := state.NewStore("")
	if err != nil {
		log.Warn("could not initialize state store, some features may be unavailable",
			logging.Err(err))
	}

	// Create cancellable context for background services.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start cleanup service for expired plans.
	var cleanupService *state.CleanupService
	if store != nil {
		cleanupConfig := state.DefaultCleanupConfig()
		cleanupConfig.OnCleanup = func(deleted int) {
			if deleted > 0 {
				log.Info("cleaned up expired plans",
					logging.Count(deleted))
			}
		}
		cleanupService = state.NewCleanupService(store, cleanupConfig)
		if err := cleanupService.Start(ctx); err != nil {
			log.Warn("could not start cleanup service", logging.Err(err))
		} else {
			log.Debug("started cleanup service")
		}
	}

	// Perform state reconciliation on startup if enabled.
	if *enableReconcile && store != nil {
		awsCfg, err := awsclient.LoadConfig(ctx, *reconcileRegion)
		if err != nil {
			log.Warn("could not load AWS config for reconciliation, feature disabled",
				logging.Err(err))
		} else {
			reconciler := state.NewReconciler(store, awsCfg)
			result, err := reconciler.Reconcile(ctx)
			if err != nil {
				log.Error("reconciliation failed", logging.Err(err))
			} else {
				log.Info("state reconciliation complete",
					slog.Int("orphaned_resources", len(result.OrphanedResources)),
					slog.Int("stale_entries", len(result.StaleLocalEntries)),
					slog.Int("synced", result.SyncedCount))

				// Log warnings for orphaned resources
				for _, orphan := range result.OrphanedResources {
					log.Warn("orphaned AWS resource detected",
						slog.String("type", orphan.ResourceType),
						slog.String("id", orphan.ResourceID),
						logging.InfraID(orphan.InfraID))
				}

				// Log warnings for stale entries
				for _, stale := range result.StaleLocalEntries {
					log.Warn("stale local entry detected",
						slog.String("type", stale.EntryType),
						slog.String("id", stale.EntryID),
						slog.Any("missing", stale.MissingResources))
				}
			}
		}
	}

	// Create AWS provider for auto-teardown functionality.
	// This needs to be created before the cost monitor so we can wire up the teardown callback.
	awsProvider := providers.GetAWSProvider(store)

	// Start cost monitor if enabled and AWS credentials are available.
	// Note: Cost Explorer API is only available in us-east-1, so we use that region
	// regardless of the reconciliation region. The CostTracker internally overrides
	// the region to us-east-1 as well.
	var costMonitor *spending.CostMonitor
	if *enableCostMonitor {
		awsCfg, err := awsclient.LoadConfig(ctx, "us-east-1")
		if err != nil {
			log.Warn("could not load AWS config for cost monitoring, feature disabled",
				logging.Err(err))
		} else {
			limits, err := spending.LoadLimits()
			if err != nil {
				log.Warn("could not load spending limits, using defaults",
					logging.Err(err))
				limits = spending.DefaultLimits()
			}

			monitorConfig := spending.DefaultMonitorConfig()
			monitorConfig.EnableAutoTeardown = *enableAutoTeardown
			monitorConfig.AlertCallback = func(ctx context.Context, alert spending.CostSummary) {
				log.Warn("spending alert",
					slog.String("deployment_id", alert.DeploymentID),
					logging.Cost(alert.TotalCostUSD),
					slog.Bool("budget_exceeded", alert.BudgetExceeded),
				)
			}

			// Set up teardown callback if auto-teardown is enabled.
			// This wires the cost monitor to the AWS provider's actual teardown functionality.
			if *enableAutoTeardown && awsProvider != nil {
				monitorConfig.TeardownCallback = func(ctx context.Context, deploymentID string) error {
					log.Warn("auto-teardown triggered for over-budget deployment",
						logging.DeploymentID(deploymentID))
					if err := awsProvider.Teardown(ctx, deploymentID); err != nil {
						log.Error("auto-teardown failed",
							logging.DeploymentID(deploymentID),
							logging.Err(err))
						return err
					}
					log.Info("auto-teardown completed successfully",
						logging.DeploymentID(deploymentID))
					return nil
				}
			}

			costMonitor = spending.NewCostMonitor(awsCfg, limits, monitorConfig)
			if err := costMonitor.Start(ctx); err != nil {
				log.Warn("could not start cost monitor", logging.Err(err))
			} else {
				log.Info("started cost monitor",
					slog.Bool("auto_teardown", *enableAutoTeardown))
			}
		}
	}

	// Create MCP server.
	opts := &mcp.ServerOptions{
		Instructions: "MCP server for natural-language cloud deployments. " +
			"Supports planning, provisioning, deploying, monitoring, and tearing down infrastructure.",
	}
	server := mcp.NewServer(
		&mcp.Implementation{Name: "agent-deploy", Version: Version},
		opts,
	)

	// Register all provider tools, resources, and prompts.
	for _, p := range providers.AllWithStore(store) {
		p.RegisterTools(server)
		p.RegisterResources(server)
		p.RegisterPrompts(server)
		log.Debug("registered provider", slog.String("provider", p.Name()))
	}

	// Handle shutdown signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Graceful shutdown handler with sync.Once to ensure it runs exactly once.
	// WHY: Per P3.14 - shutdown may be called from multiple paths (defer, signal handler,
	// HTTP server shutdown). sync.Once prevents duplicate cleanup and log spam.
	var shutdownOnce sync.Once
	shutdown := func() {
		shutdownOnce.Do(func() {
			log.Info("shutting down...")
			cancel() // Cancel context for background services.

			if cleanupService != nil && cleanupService.IsRunning() {
				cleanupService.Stop()
				stats := cleanupService.Stats()
				log.Info("cleanup service stopped",
					slog.Int("total_deleted", stats.TotalDeleted))
			}

			if costMonitor != nil && costMonitor.IsRunning() {
				costMonitor.Stop()
				stats := costMonitor.Stats()
				log.Info("cost monitor stopped",
					slog.Int("alerts_sent", stats.AlertsSent),
					slog.Int("teardowns_done", stats.TeardownsDone))
			}
		})
	}

	// Ensure shutdown is called on any exit path.
	// WHY: Per P3.14 - if server startup fails after background services have
	// started, we must clean them up to prevent leaked goroutines and resources.
	defer shutdown()

	// Serve over stdio or streamable HTTP.
	if *httpAddr != "" {
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return server
		}, nil)

		httpServer := &http.Server{
			Addr:    *httpAddr,
			Handler: handler,
		}

		// Handle shutdown in a goroutine.
		go func() {
			<-sigCh
			shutdown()
			// Use Shutdown for graceful shutdown - waits for in-flight requests.
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer shutdownCancel()
			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				log.Error("failed to gracefully shutdown HTTP server", logging.Err(err))
			}
		}()

		log.Info("listening on HTTP", slog.String("address", *httpAddr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("HTTP server failed", logging.Err(err))
			return // Return instead of os.Exit to allow defers to run
		}
	} else {
		// For stdio, handle shutdown signal in goroutine.
		go func() {
			<-sigCh
			shutdown()
			// Don't os.Exit here - let main function return naturally
			// The server.Run() call will return when context is canceled
		}()

		log.Info("running on stdio transport")
		if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
			log.Error("server failed", logging.Err(err))
			return // Return instead of os.Exit to allow defers to run
		}
	}
}
