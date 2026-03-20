package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cjmartian/agent-deploy/internal/providers"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// createTestServer creates a configured MCP server for testing.
func createTestServer() *mcp.Server {
	opts := &mcp.ServerOptions{
		Instructions: "Test MCP server for agent-deploy",
	}
	server := mcp.NewServer(
		&mcp.Implementation{Name: "agent-deploy-test", Version: "v0.0.1"},
		opts,
	)

	for _, p := range providers.All() {
		p.RegisterTools(server)
		p.RegisterResources(server)
		p.RegisterPrompts(server)
	}

	return server
}

// connectClientToServer creates an in-memory client-server connection for testing.
func connectClientToServer(ctx context.Context, t *testing.T, server *mcp.Server) *mcp.ClientSession {
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	// Start server in background
	go func() {
		if err := server.Run(ctx, serverTransport); err != nil && ctx.Err() == nil {
			t.Logf("Server run error: %v", err)
		}
	}()

	// Give server time to start
	time.Sleep(10 * time.Millisecond)

	// Connect client
	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "v1.0.0"},
		nil,
	)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}

	return session
}

// TestServerCreation verifies the MCP server can be created and configured.
func TestServerCreation(t *testing.T) {
	server := createTestServer()
	if server == nil {
		t.Fatal("Server should not be nil")
	}
}

// TestProviderRegistration verifies all providers register successfully.
func TestProviderRegistration(t *testing.T) {
	opts := &mcp.ServerOptions{
		Instructions: "Test MCP server",
	}
	server := mcp.NewServer(
		&mcp.Implementation{Name: "agent-deploy-test", Version: "v0.0.1"},
		opts,
	)

	// Register all providers - should not panic
	allProviders := providers.All()
	if len(allProviders) == 0 {
		t.Fatal("No providers returned from providers.All()")
	}

	for _, p := range allProviders {
		t.Run(p.Name(), func(t *testing.T) {
			// These should not panic
			p.RegisterTools(server)
			p.RegisterResources(server)
			p.RegisterPrompts(server)
		})
	}
}

// TestAllProvidersHaveNames verifies each provider has a non-empty name.
func TestAllProvidersHaveNames(t *testing.T) {
	for _, p := range providers.All() {
		name := p.Name()
		if name == "" {
			t.Error("Provider has empty name")
		}
	}
}

// TestToolListing verifies expected tools are registered and listable.
func TestToolListing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server := createTestServer()
	session := connectClientToServer(ctx, t, server)
	defer session.Close()

	// List tools
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	// Check for expected AWS tools
	expectedTools := map[string]bool{
		"aws_plan_infra":   false,
		"aws_create_infra": false,
		"aws_deploy":       false,
		"aws_status":       false,
		"aws_teardown":     false,
	}

	for _, tool := range result.Tools {
		if _, exists := expectedTools[tool.Name]; exists {
			expectedTools[tool.Name] = true
		}
	}

	for name, found := range expectedTools {
		if !found {
			t.Errorf("Expected tool %q not found", name)
		}
	}
}

// TestResourceListing verifies expected resources are registered and listable.
func TestResourceListing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server := createTestServer()
	session := connectClientToServer(ctx, t, server)
	defer session.Close()

	// List resources
	result, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list resources: %v", err)
	}

	// Check for aws:deployments resource
	found := false
	for _, res := range result.Resources {
		if res.URI == "aws:deployments" {
			found = true
			if res.MIMEType != "application/json" {
				t.Errorf("Expected MIME type application/json, got %s", res.MIMEType)
			}
			break
		}
	}
	if !found {
		t.Error("Expected resource 'aws:deployments' not found")
	}
}

// TestPromptListing verifies expected prompts are registered and listable.
func TestPromptListing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server := createTestServer()
	session := connectClientToServer(ctx, t, server)
	defer session.Close()

	// List prompts
	result, err := session.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list prompts: %v", err)
	}

	// Check for aws_deploy_plan prompt
	found := false
	for _, prompt := range result.Prompts {
		if prompt.Name == "aws_deploy_plan" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected prompt 'aws_deploy_plan' not found")
	}
}

// TestServerInitialization verifies the server initializes correctly.
func TestServerInitialization(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server := createTestServer()
	session := connectClientToServer(ctx, t, server)
	defer session.Close()

	// Get initialize result
	initResult := session.InitializeResult()
	if initResult == nil {
		t.Fatal("InitializeResult should not be nil")
	}

	if initResult.ServerInfo.Name != "agent-deploy-test" {
		t.Errorf("Expected server name 'agent-deploy-test', got %s", initResult.ServerInfo.Name)
	}

	if initResult.ServerInfo.Version != "v0.0.1" {
		t.Errorf("Expected server version 'v0.0.1', got %s", initResult.ServerInfo.Version)
	}
}

// TestResourceRead verifies the deployments resource can be read.
func TestResourceRead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server := createTestServer()
	session := connectClientToServer(ctx, t, server)
	defer session.Close()

	// Read deployments resource
	result, err := session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "aws:deployments",
	})
	if err != nil {
		t.Fatalf("Failed to read resource: %v", err)
	}

	if len(result.Contents) == 0 {
		t.Fatal("Expected at least one content item")
	}

	// Verify the response is valid JSON with deployments array
	content := result.Contents[0]
	if content.MIMEType != "application/json" {
		t.Errorf("Expected MIME type application/json, got %s", content.MIMEType)
	}

	// Parse the text content
	var deployments struct {
		Deployments []interface{} `json:"deployments"`
	}
	if err := json.Unmarshal([]byte(content.Text), &deployments); err != nil {
		t.Fatalf("Failed to parse deployments JSON: %v", err)
	}

	// Empty array is valid (no deployments yet)
	if deployments.Deployments == nil {
		t.Error("Expected deployments array in response")
	}
}

// TestGetPrompt verifies the aws_deploy_plan prompt can be retrieved.
func TestGetPrompt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server := createTestServer()
	session := connectClientToServer(ctx, t, server)
	defer session.Close()

	// Get the aws_deploy_plan prompt
	result, err := session.GetPrompt(ctx, &mcp.GetPromptParams{
		Name: "aws_deploy_plan",
		Arguments: map[string]string{
			"app_description": "A simple web application",
		},
	})
	if err != nil {
		t.Fatalf("Failed to get prompt: %v", err)
	}

	if len(result.Messages) == 0 {
		t.Fatal("Expected at least one message in prompt")
	}

	// Check that a message with text content exists
	foundText := false
	for _, msg := range result.Messages {
		if msg.Content != nil {
			// Check if content contains text
			if textContent, ok := msg.Content.(*mcp.TextContent); ok && textContent.Text != "" {
				foundText = true
				break
			}
		}
	}
	if !foundText {
		t.Error("Expected prompt message with text content")
	}
}

// TestServerCapabilities verifies the server advertises correct capabilities.
func TestServerCapabilities(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server := createTestServer()
	session := connectClientToServer(ctx, t, server)
	defer session.Close()

	initResult := session.InitializeResult()
	if initResult == nil {
		t.Fatal("InitializeResult should not be nil")
	}

	// Verify capabilities are advertised
	caps := initResult.Capabilities
	if caps.Tools == nil {
		t.Error("Server should advertise tools capability")
	}
	if caps.Resources == nil {
		t.Error("Server should advertise resources capability")
	}
	if caps.Prompts == nil {
		t.Error("Server should advertise prompts capability")
	}
}

// TestPing verifies the server responds to ping.
func TestPing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server := createTestServer()
	session := connectClientToServer(ctx, t, server)
	defer session.Close()

	// Ping should not error
	if err := session.Ping(ctx, nil); err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}
