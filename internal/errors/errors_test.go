// Package errors tests verify domain error types and their behavior.
package errors

import (
	"errors"
	"fmt"
	"testing"
)

// TestErrorTypes_AreDistinct verifies each domain error has a unique identity.
func TestErrorTypes_AreDistinct(t *testing.T) {
	errs := []error{
		ErrPlanNotFound,
		ErrPlanNotApproved,
		ErrPlanExpired,
		ErrInfraNotFound,
		ErrInfraNotReady,
		ErrDeploymentNotFound,
		ErrBudgetExceeded,
		ErrProvisioningFailed,
		ErrInvalidState,
	}

	for i, err1 := range errs {
		for j, err2 := range errs {
			if i == j {
				continue
			}
			if errors.Is(err1, err2) {
				t.Errorf("errors.Is(%v, %v) = true, want false (errors should be distinct)", err1, err2)
			}
		}
	}
}

// TestErrorWrapping_Is verifies errors.Is() works through wrapping.
func TestErrorWrapping_Is(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		original error
		wantIs   bool
	}{
		{
			name:     "direct match",
			err:      ErrPlanNotFound,
			original: ErrPlanNotFound,
			wantIs:   true,
		},
		{
			name:     "wrapped with context",
			err:      fmt.Errorf("failed to process: %w", ErrPlanNotFound),
			original: ErrPlanNotFound,
			wantIs:   true,
		},
		{
			name:     "double wrapped",
			err:      fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", ErrBudgetExceeded)),
			original: ErrBudgetExceeded,
			wantIs:   true,
		},
		{
			name: "wrapped with %v loses chain",
			// Intentionally using %v (not %w) to test that error chain is broken.
			// This test verifies the behavior that production code should avoid.
			//nolint:errorlint // Intentionally testing %v behavior
			err:      fmt.Errorf("failed: %v", ErrPlanExpired),
			original: ErrPlanExpired,
			wantIs:   false, // %v breaks the error chain
		},
		{
			name:     "different error",
			err:      ErrPlanNotFound,
			original: ErrInfraNotFound,
			wantIs:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := errors.Is(tt.err, tt.original)
			if got != tt.wantIs {
				t.Errorf("errors.Is(%v, %v) = %v, want %v", tt.err, tt.original, got, tt.wantIs)
			}
		})
	}
}

// TestErrorMessages verifies error messages are human-readable.
func TestErrorMessages(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{ErrPlanNotFound, "plan not found"},
		{ErrPlanNotApproved, "plan not approved"},
		{ErrPlanExpired, "plan expired"},
		{ErrInfraNotFound, "infrastructure not found"},
		{ErrInfraNotReady, "infrastructure not ready"},
		{ErrDeploymentNotFound, "deployment not found"},
		{ErrBudgetExceeded, "spending budget exceeded"},
		{ErrProvisioningFailed, "provisioning failed"},
		{ErrInvalidState, "invalid state transition"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestWrappedErrorMessages verifies wrapped errors include context.
func TestWrappedErrorMessages(t *testing.T) {
	wrapped := fmt.Errorf("failed to get plan %s: %w", "plan-123", ErrPlanNotFound)

	want := "failed to get plan plan-123: plan not found"
	if got := wrapped.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	// errors.Is should still work
	if !errors.Is(wrapped, ErrPlanNotFound) {
		t.Error("errors.Is(wrapped, ErrPlanNotFound) = false, want true")
	}
}
