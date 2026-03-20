// Package errors defines domain-specific error types for agent-deploy.
package errors

import "errors"

// Domain errors returned by the state store and tool handlers.
var (
	ErrPlanNotFound       = errors.New("plan not found")
	ErrPlanNotApproved    = errors.New("plan not approved")
	ErrPlanExpired        = errors.New("plan expired")
	ErrInfraNotFound      = errors.New("infrastructure not found")
	ErrInfraNotReady      = errors.New("infrastructure not ready")
	ErrDeploymentNotFound = errors.New("deployment not found")
	ErrBudgetExceeded     = errors.New("spending budget exceeded")
	ErrProvisioningFailed = errors.New("provisioning failed")
	ErrInvalidState       = errors.New("invalid state transition")
)
