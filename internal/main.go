package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/cjmartian/agent-deploy/internal/logging"
	"github.com/cjmartian/agent-deploy/internal/providers"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	httpAddr = flag.String("http", "", "if set, use streamable HTTP at this address instead of stdin/stdout")
	logLevel = flag.String("log-level", "info", "log level: debug, info, warn, error")
	logFormat = flag.String("log-format", "text", "log format: text, json")
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

	opts := &mcp.ServerOptions{
		Instructions: "MCP server for natural-language cloud deployments. " +
			"Supports planning, provisioning, deploying, monitoring, and tearing down infrastructure.",
	}
	server := mcp.NewServer(
		&mcp.Implementation{Name: "agent-deploy", Version: "v0.1.0"},
		opts,
	)

	// Register all provider tools, resources, and prompts.
	for _, p := range providers.All() {
		p.RegisterTools(server)
		p.RegisterResources(server)
		p.RegisterPrompts(server)
		log.Debug("registered provider", slog.String("provider", p.Name()))
	}

	// Serve over stdio or streamable HTTP.
	if *httpAddr != "" {
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return server
		}, nil)
		log.Info("listening on HTTP", slog.String("address", *httpAddr))
		if err := http.ListenAndServe(*httpAddr, handler); err != nil {
			log.Error("HTTP server failed", logging.Err(err))
			os.Exit(1)
		}
	} else {
		log.Info("running on stdio transport")
		if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			log.Error("server failed", logging.Err(err))
			os.Exit(1)
		}
	}
}
