// Package state provides file-backed storage for deployment state.
package state

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	apperrors "github.com/cjmartian/agent-deploy/internal/errors"
)

// Store manages persistent state for plans, infrastructure, and deployments.
// State is stored as JSON files under ~/.agent-deploy/state/.
type Store struct {
	baseDir string
	mu      sync.RWMutex
}

// NewStore creates a new Store rooted at the given directory.
// If dir is empty, defaults to ~/.agent-deploy/state.
func NewStore(dir string) (*Store, error) {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		dir = filepath.Join(home, ".agent-deploy", "state")
	}

	// Create directory structure.
	for _, subdir := range []string{"plans", "infra", "deployments"} {
		if err := os.MkdirAll(filepath.Join(dir, subdir), 0755); err != nil {
			return nil, fmt.Errorf("create %s dir: %w", subdir, err)
		}
	}

	return &Store{baseDir: dir}, nil
}

// --- Plan operations ---

// CreatePlan persists a new plan.
func (s *Store) CreatePlan(plan *Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.planPath(plan.ID)
	return s.writeJSON(path, plan)
}

// GetPlan retrieves a plan by ID.
func (s *Store) GetPlan(id string) (*Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var plan Plan
	if err := s.readJSON(s.planPath(id), &plan); err != nil {
		if os.IsNotExist(err) {
			return nil, apperrors.ErrPlanNotFound
		}
		return nil, err
	}
	return &plan, nil
}

// ApprovePlan marks a plan as approved.
// Returns ErrPlanExpired if the plan has expired.
// Returns ErrInvalidState if the plan is already approved, rejected, or in an invalid state.
// Idempotent: returns nil if the plan is already approved.
func (s *Store) ApprovePlan(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	plan, err := s.getPlanLocked(id)
	if err != nil {
		return err
	}

	// Check for expiration first.
	if time.Now().After(plan.ExpiresAt) {
		plan.Status = PlanStatusExpired
		if writeErr := s.writeJSON(s.planPath(id), plan); writeErr != nil {
			slog.Error("failed to persist expired plan status",
				slog.String("plan_id", id),
				slog.Any("error", writeErr))
		}
		return apperrors.ErrPlanExpired
	}

	// Handle based on current status.
	switch plan.Status {
	case PlanStatusApproved:
		// Idempotent: already approved.
		return nil
	case PlanStatusRejected:
		return fmt.Errorf("%w: plan was rejected and cannot be approved", apperrors.ErrInvalidState)
	case PlanStatusExpired:
		return apperrors.ErrPlanExpired
	case PlanStatusCreated:
		// Valid transition: created -> approved.
		plan.Status = PlanStatusApproved
		return s.writeJSON(s.planPath(id), plan)
	default:
		return fmt.Errorf("%w: unknown plan status '%s'", apperrors.ErrInvalidState, plan.Status)
	}
}

// RejectPlan marks a plan as rejected.
// Returns ErrPlanExpired if the plan has expired.
// Returns ErrInvalidState if the plan is already approved, rejected, or in an invalid state.
func (s *Store) RejectPlan(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	plan, err := s.getPlanLocked(id)
	if err != nil {
		return err
	}

	// Check for expiration first.
	if time.Now().After(plan.ExpiresAt) {
		plan.Status = PlanStatusExpired
		if writeErr := s.writeJSON(s.planPath(id), plan); writeErr != nil {
			slog.Error("failed to persist expired plan status",
				slog.String("plan_id", id),
				slog.Any("error", writeErr))
		}
		return apperrors.ErrPlanExpired
	}

	// Handle based on current status.
	switch plan.Status {
	case PlanStatusRejected:
		// Idempotent: already rejected.
		return nil
	case PlanStatusApproved:
		return fmt.Errorf("%w: plan was already approved and cannot be rejected", apperrors.ErrInvalidState)
	case PlanStatusExpired:
		return apperrors.ErrPlanExpired
	case PlanStatusCreated:
		// Valid transition: created -> rejected.
		plan.Status = PlanStatusRejected
		return s.writeJSON(s.planPath(id), plan)
	default:
		return fmt.Errorf("%w: unknown plan status '%s'", apperrors.ErrInvalidState, plan.Status)
	}
}

// ListPlans returns all plans.
func (s *Store) ListPlans() ([]*Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.baseDir, "plans")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dir: %w", err)
	}

	plans := make([]*Plan, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var plan Plan
		path := filepath.Join(dir, entry.Name())
		if err := s.readJSON(path, &plan); err != nil {
			slog.Warn("skipping malformed state file",
				slog.String("file", path),
				slog.String("type", "plan"),
				slog.Any("error", err))
			continue
		}
		plans = append(plans, &plan)
	}
	return plans, nil
}

func (s *Store) planPath(id string) string {
	return filepath.Join(s.baseDir, "plans", id+".json")
}

func (s *Store) getPlanLocked(id string) (*Plan, error) {
	var plan Plan
	if err := s.readJSON(s.planPath(id), &plan); err != nil {
		if os.IsNotExist(err) {
			return nil, apperrors.ErrPlanNotFound
		}
		return nil, err
	}
	return &plan, nil
}

// DeletePlan removes a plan from the store.
func (s *Store) DeletePlan(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.planPath(id)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete plan: %w", err)
	}
	return nil
}

// DeleteExpiredPlans removes all plans that have expired.
// Returns the number of plans deleted.
func (s *Store) DeleteExpiredPlans() (int, error) {
	plans, err := s.ListPlans()
	if err != nil {
		return 0, err
	}

	now := time.Now()
	deleted := 0

	for _, plan := range plans {
		if now.After(plan.ExpiresAt) {
			if err := s.DeletePlan(plan.ID); err != nil {
				slog.Warn("failed to delete expired plan",
					slog.String("plan_id", plan.ID),
					slog.Any("error", err))
				continue
			}
			deleted++
		}
	}

	return deleted, nil
}

// --- Infrastructure operations ---

// CreateInfra persists a new infrastructure record.
func (s *Store) CreateInfra(infra *Infrastructure) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.infraPath(infra.ID)
	return s.writeJSON(path, infra)
}

// GetInfra retrieves an infrastructure record by ID.
func (s *Store) GetInfra(id string) (*Infrastructure, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var infra Infrastructure
	if err := s.readJSON(s.infraPath(id), &infra); err != nil {
		if os.IsNotExist(err) {
			return nil, apperrors.ErrInfraNotFound
		}
		return nil, err
	}
	return &infra, nil
}

// UpdateInfraResource updates a single resource ARN in the infrastructure.
func (s *Store) UpdateInfraResource(id, resourceType, arn string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	infra, err := s.getInfraLocked(id)
	if err != nil {
		return err
	}

	if infra.Resources == nil {
		infra.Resources = make(map[string]string)
	}
	infra.Resources[resourceType] = arn
	return s.writeJSON(s.infraPath(id), infra)
}

// SetInfraStatus updates the infrastructure status with transition validation.
// Valid transitions:
//   - provisioning → ready (success) or failed (error)
//   - failed → provisioning (retry) or destroyed (teardown)
//   - ready → destroyed (teardown)
//   - destroyed → (terminal state, no transitions)
//
// Returns ErrInvalidState if the transition is not allowed.
func (s *Store) SetInfraStatus(id, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	infra, err := s.getInfraLocked(id)
	if err != nil {
		return err
	}

	// Validate the state transition.
	if err := validateInfraTransition(infra.Status, status); err != nil {
		return err
	}

	infra.Status = status
	return s.writeJSON(s.infraPath(id), infra)
}

// validateInfraTransition checks if the infrastructure state transition is valid.
// State machine:
//
//	provisioning ──┬──► ready ──────────► destroyed
//	               │                          ▲
//	               └──► failed ──┬────────────┘
//	                             │
//	                             └──► provisioning (retry)
func validateInfraTransition(from, to string) error {
	// Same state is always valid (idempotent).
	if from == to {
		return nil
	}

	valid := false
	switch from {
	case InfraStatusProvisioning:
		// Can transition to ready (success) or failed (error).
		valid = to == InfraStatusReady || to == InfraStatusFailed
	case InfraStatusReady:
		// Can only transition to destroyed (teardown).
		valid = to == InfraStatusDestroyed
	case InfraStatusFailed:
		// Can transition to provisioning (retry) or destroyed (teardown).
		valid = to == InfraStatusProvisioning || to == InfraStatusDestroyed
	case InfraStatusDestroyed:
		// Terminal state — no transitions allowed.
		valid = false
	default:
		return fmt.Errorf("%w: unknown infrastructure status '%s'", apperrors.ErrInvalidState, from)
	}

	if !valid {
		return fmt.Errorf("%w: cannot transition infrastructure from '%s' to '%s'", apperrors.ErrInvalidState, from, to)
	}
	return nil
}

// ListInfra returns all infrastructure records.
func (s *Store) ListInfra() ([]*Infrastructure, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.baseDir, "infra")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dir: %w", err)
	}

	items := make([]*Infrastructure, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var item Infrastructure
		path := filepath.Join(dir, entry.Name())
		if err := s.readJSON(path, &item); err != nil {
			slog.Warn("skipping malformed state file",
				slog.String("file", path),
				slog.String("type", "infrastructure"),
				slog.Any("error", err))
			continue
		}
		items = append(items, &item)
	}
	return items, nil
}

func (s *Store) infraPath(id string) string {
	return filepath.Join(s.baseDir, "infra", id+".json")
}

func (s *Store) getInfraLocked(id string) (*Infrastructure, error) {
	var infra Infrastructure
	if err := s.readJSON(s.infraPath(id), &infra); err != nil {
		if os.IsNotExist(err) {
			return nil, apperrors.ErrInfraNotFound
		}
		return nil, err
	}
	return &infra, nil
}

// --- Deployment operations ---

// CreateDeployment persists a new deployment.
func (s *Store) CreateDeployment(deploy *Deployment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.deployPath(deploy.ID)
	return s.writeJSON(path, deploy)
}

// GetDeployment retrieves a deployment by ID.
func (s *Store) GetDeployment(id string) (*Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var deploy Deployment
	if err := s.readJSON(s.deployPath(id), &deploy); err != nil {
		if os.IsNotExist(err) {
			return nil, apperrors.ErrDeploymentNotFound
		}
		return nil, err
	}
	return &deploy, nil
}

// UpdateDeploymentStatus updates the status and URLs of a deployment with transition validation.
// Valid transitions:
//   - deploying → running (success), failed (error), or stopped (teardown)
//   - running → deploying (update), failed (error), or stopped (teardown)
//   - failed → deploying (retry), or stopped (teardown)
//   - stopped → (terminal state, no transitions)
//
// Returns ErrInvalidState if the transition is not allowed.
func (s *Store) UpdateDeploymentStatus(id, status string, urls []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	deploy, err := s.getDeployLocked(id)
	if err != nil {
		return err
	}

	// Validate the state transition.
	if err := validateDeploymentTransition(deploy.Status, status); err != nil {
		return err
	}

	deploy.Status = status
	if urls != nil {
		deploy.URLs = urls
	}
	deploy.LastUpdated = time.Now()
	return s.writeJSON(s.deployPath(id), deploy)
}

// validateDeploymentTransition checks if the deployment state transition is valid.
// State machine:
//
//	deploying ──┬──► running ──┬──► deploying (update)
//	            │              │
//	            │              ├──► failed ──┬──► deploying (retry)
//	            │              │             │
//	            │              └──► stopped  │
//	            │                    ▲       │
//	            ├──► failed ─────────┼───────┘
//	            │                    │
//	            └──► stopped ────────┘
func validateDeploymentTransition(from, to string) error {
	// Same state is always valid (idempotent).
	if from == to {
		return nil
	}

	valid := false
	switch from {
	case DeploymentStatusDeploying:
		// Can transition to running (success), failed (error), or stopped (teardown).
		valid = to == DeploymentStatusRunning || to == DeploymentStatusFailed || to == DeploymentStatusStopped
	case DeploymentStatusRunning:
		// Can transition to deploying (update), failed (error), or stopped (teardown).
		valid = to == DeploymentStatusDeploying || to == DeploymentStatusFailed || to == DeploymentStatusStopped
	case DeploymentStatusFailed:
		// Can transition to deploying (retry) or stopped (teardown).
		valid = to == DeploymentStatusDeploying || to == DeploymentStatusStopped
	case DeploymentStatusStopped:
		// Terminal state — no transitions allowed.
		valid = false
	default:
		return fmt.Errorf("%w: unknown deployment status '%s'", apperrors.ErrInvalidState, from)
	}

	if !valid {
		return fmt.Errorf("%w: cannot transition deployment from '%s' to '%s'", apperrors.ErrInvalidState, from, to)
	}
	return nil
}

// ListDeployments returns all deployments.
func (s *Store) ListDeployments() ([]*Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.baseDir, "deployments")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dir: %w", err)
	}

	items := make([]*Deployment, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var item Deployment
		path := filepath.Join(dir, entry.Name())
		if err := s.readJSON(path, &item); err != nil {
			slog.Warn("skipping malformed state file",
				slog.String("file", path),
				slog.String("type", "deployment"),
				slog.Any("error", err))
			continue
		}
		items = append(items, &item)
	}
	return items, nil
}

// DeleteDeployment removes a deployment from the store.
func (s *Store) DeleteDeployment(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.deployPath(id)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete deployment: %w", err)
	}
	return nil
}

func (s *Store) deployPath(id string) string {
	return filepath.Join(s.baseDir, "deployments", id+".json")
}

func (s *Store) getDeployLocked(id string) (*Deployment, error) {
	var deploy Deployment
	if err := s.readJSON(s.deployPath(id), &deploy); err != nil {
		if os.IsNotExist(err) {
			return nil, apperrors.ErrDeploymentNotFound
		}
		return nil, err
	}
	return &deploy, nil
}

// --- Helper methods ---

// writeJSON atomically writes JSON data to a file using a temp file + rename pattern.
// This prevents data corruption if the process is interrupted during write.
func (s *Store) writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	// Write to temp file in the same directory to ensure atomic rename works
	// (temp file must be on the same filesystem as the target).
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on any error
	defer func() {
		if tmpPath != "" {
			os.Remove(tmpPath) // Best-effort cleanup; ignore errors
		}
	}()

	// Write data to temp file
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}

	// Sync to ensure data is flushed to disk before rename
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Set correct permissions before rename
	if err := os.Chmod(tmpPath, 0644); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	// Atomic rename - this is the key to preventing corruption
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	// Clear tmpPath so deferred cleanup doesn't try to remove the final file
	tmpPath = ""
	return nil
}

func (s *Store) readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	return nil
}
