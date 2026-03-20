// Package id provides ULID-based identifier generation for plans, infrastructure, and deployments.
package id

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// entropy is a shared source of randomness, protected by a mutex.
var (
	entropyMu sync.Mutex
	entropy   = ulid.Monotonic(rand.Reader, 0)
)

// New generates a new prefixed ULID. Valid prefixes are "plan", "infra", "deploy".
// The format is "{prefix}-{ulid}" (e.g., "plan-01HX...").
func New(prefix string) string {
	entropyMu.Lock()
	defer entropyMu.Unlock()
	id := ulid.MustNew(ulid.Timestamp(time.Now()), entropy)
	return prefix + "-" + id.String()
}

// NewPlan generates a new plan ID.
func NewPlan() string {
	return New("plan")
}

// NewInfra generates a new infrastructure ID.
func NewInfra() string {
	return New("infra")
}

// NewDeploy generates a new deployment ID.
func NewDeploy() string {
	return New("deploy")
}
