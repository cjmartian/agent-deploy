package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	"github.com/cjmartian/agent-deploy/internal/providers"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var httpAddr = flag.String("http", "", "if set, use streamable HTTP at this address instead of stdin/stdout")

func main() {
	flag.Parse()

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
	}

	// Serve over stdio or streamable HTTP.
	if *httpAddr != "" {
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return server
		}, nil)
		log.Printf("MCP server listening at %s", *httpAddr)
		log.Fatal(http.ListenAndServe(*httpAddr, handler))
	} else {
		if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			log.Fatal(err)
		}
	}
}
