package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/cjmartian/agent-deploy/internal/awsclient"
	"github.com/cjmartian/agent-deploy/internal/logging"
	"github.com/cjmartian/agent-deploy/internal/providers"
	"github.com/cjmartian/agent-deploy/internal/spending"
	"github.com/cjmartian/agent-deploy/internal/state"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	httpAddr          = flag.String("http", "", "if set, use streamable HTTP at this address instead of stdin/stdout")
	logLevel          = flag.String("log-level", "info", "log level: debug, info, warn, error")
	logFormat         = flag.String("log-format", "text", "log format: text, json")
	enableCostMonitor = flag.Bool("enable-cost-monitor", false, "enable runtime cost monitoring (requires AWS credentials)")
	enableAutoTeardown = flag.Bool("enable-auto-teardown", false, "enable automatic teardown of over-budget deployments")
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
		slog.String("version", "v0.1.0"),
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

	// Start cost monitor if enabled and AWS credentials are available.
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
			if *enableAutoTeardown {
				monitorConfig.TeardownCallback = func(ctx context.Context, deploymentID string) error {
					log.Warn("auto-teardown triggered",
						logging.DeploymentID(deploymentID))
					// Note: The actual teardown would be performed via the AWS provider.
					// This requires access to the provider, which we'll add in a future iteration.
					// For now, we log the intent. Users can manually teardown using aws_teardown tool.
					log.Info("deployment marked for teardown - use aws_teardown tool to complete",
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
		&mcp.Implementation{Name: "agent-deploy", Version: "v0.1.0"},
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

	// Graceful shutdown handler.
	shutdown := func() {
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
	}

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
			httpServer.Close()
		}()

		log.Info("listening on HTTP", slog.String("address", *httpAddr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("HTTP server failed", logging.Err(err))
			os.Exit(1)
		}
	} else {
		// For stdio, handle shutdown signal in goroutine.
		go func() {
			<-sigCh
			shutdown()
			os.Exit(0)
		}()

		log.Info("running on stdio transport")
		if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
			log.Error("server failed", logging.Err(err))
			os.Exit(1)
		}
	}
}
