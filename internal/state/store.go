// Package state provides file-backed storage for deployment state.
package state

import (
	"encoding/json"
	"fmt"
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
func (s *Store) ApprovePlan(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	plan, err := s.getPlanLocked(id)
	if err != nil {
		return err
	}

	if time.Now().After(plan.ExpiresAt) {
		plan.Status = PlanStatusExpired
		_ = s.writeJSON(s.planPath(id), plan)
		return apperrors.ErrPlanExpired
	}

	plan.Status = PlanStatusApproved
	return s.writeJSON(s.planPath(id), plan)
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

	var plans []*Plan
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var plan Plan
		path := filepath.Join(dir, entry.Name())
		if err := s.readJSON(path, &plan); err != nil {
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

// SetInfraStatus updates the infrastructure status.
func (s *Store) SetInfraStatus(id, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	infra, err := s.getInfraLocked(id)
	if err != nil {
		return err
	}

	infra.Status = status
	return s.writeJSON(s.infraPath(id), infra)
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

	var items []*Infrastructure
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var item Infrastructure
		path := filepath.Join(dir, entry.Name())
		if err := s.readJSON(path, &item); err != nil {
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

// UpdateDeploymentStatus updates the status and URLs of a deployment.
func (s *Store) UpdateDeploymentStatus(id, status string, urls []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	deploy, err := s.getDeployLocked(id)
	if err != nil {
		return err
	}

	deploy.Status = status
	if urls != nil {
		deploy.URLs = urls
	}
	deploy.LastUpdated = time.Now()
	return s.writeJSON(s.deployPath(id), deploy)
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

	var items []*Deployment
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var item Deployment
		path := filepath.Join(dir, entry.Name())
		if err := s.readJSON(path, &item); err != nil {
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

func (s *Store) writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
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
