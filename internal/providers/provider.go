// Package providers implements cloud provider integrations for the MCP server.
package providers

import (
	"context"
	"log/slog"

	"github.com/cjmartian/agent-deploy/internal/logging"
	"github.com/cjmartian/agent-deploy/internal/state"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Provider is the interface that cloud providers implement to register
// their tools, resources, and prompts with the MCP server.
type Provider interface {
	Name() string
	RegisterTools(server *mcp.Server)
	RegisterResources(server *mcp.Server)
	RegisterPrompts(server *mcp.Server)
}

// TeardownProvider is an interface for providers that support programmatic teardown.
// This enables the cost monitor to trigger automatic teardowns when budgets are exceeded.
type TeardownProvider interface {
	Teardown(ctx context.Context, deploymentID string) error
}

// All returns every available provider with initialized state stores.
func All() []Provider {
	store, err := state.NewStore("")
	if err != nil {
		slog.Warn("could not initialize state store",
			slog.String("component", logging.ComponentAWSProvider),
			logging.Err(err))
		// Create provider anyway with nil store for graceful degradation.
		return []Provider{
			&AWSProvider{},
		}
	}
	return []Provider{
		NewAWSProvider(store),
	}
}

// AllWithStore returns every available provider using the provided store.
// This allows sharing a single store instance between providers and services.
func AllWithStore(store *state.Store) []Provider {
	if store == nil {
		// Graceful degradation with nil store.
		return []Provider{
			&AWSProvider{},
		}
	}
	return []Provider{
		NewAWSProvider(store),
	}
}

// GetAWSProvider returns the AWS provider with the given store.
// This is used when direct access to provider methods (like Teardown) is needed.
func GetAWSProvider(store *state.Store) *AWSProvider {
	return NewAWSProvider(store)
}
