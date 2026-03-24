// Package providers tests verify provider registration and initialization.
package providers

import (
	"testing"

	"github.com/cjmartian/agent-deploy/internal/state"
)

// TestAll_ReturnsProviders tests that All() returns at least one provider.
func TestAll_ReturnsProviders(t *testing.T) {
	providers := All()

	if len(providers) == 0 {
		t.Error("All() returned no providers")
	}
}

// TestAll_HasAWSProvider tests that All() includes the AWS provider.
func TestAll_HasAWSProvider(t *testing.T) {
	providers := All()

	var found bool
	for _, p := range providers {
		if p.Name() == "aws" {
			found = true
			break
		}
	}

	if !found {
		t.Error("All() did not return AWS provider")
	}
}

// TestAllWithStore_ReturnsProviders tests AllWithStore with a valid store.
func TestAllWithStore_ReturnsProviders(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	providers := AllWithStore(store)

	if len(providers) == 0 {
		t.Error("AllWithStore() returned no providers")
	}

	// Verify AWS provider is present.
	var found bool
	for _, p := range providers {
		if p.Name() == "aws" {
			found = true
			break
		}
	}
	if !found {
		t.Error("AllWithStore() did not return AWS provider")
	}
}

// TestAllWithStore_NilStore tests AllWithStore with nil store (graceful degradation).
func TestAllWithStore_NilStore(t *testing.T) {
	providers := AllWithStore(nil)

	// Should still return providers, just with nil store.
	if len(providers) == 0 {
		t.Error("AllWithStore(nil) returned no providers")
	}
}

// TestProviderInterface_Implementation tests that AWSProvider implements Provider.
func TestProviderInterface_Implementation(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	// Verify it implements Provider interface.
	var _ Provider = provider

	// Check Name() method.
	if provider.Name() != "aws" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "aws")
	}
}

// TestTeardownProviderInterface tests that AWSProvider implements TeardownProvider.
func TestTeardownProviderInterface(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	// Verify it implements TeardownProvider interface.
	var _ TeardownProvider = provider
}
