package id

import (
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		prefix string
		want   string
	}{
		{"plan", "plan-"},
		{"infra", "infra-"},
		{"deploy", "deploy-"},
	}

	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			got := New(tt.prefix)
			if !strings.HasPrefix(got, tt.want) {
				t.Errorf("New(%q) = %q, want prefix %q", tt.prefix, got, tt.want)
			}
			// ULID is 26 characters, plus prefix and hyphen.
			expectedLen := len(tt.prefix) + 1 + 26
			if len(got) != expectedLen {
				t.Errorf("New(%q) length = %d, want %d", tt.prefix, len(got), expectedLen)
			}
		})
	}
}

func TestUniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := NewPlan()
		if ids[id] {
			t.Errorf("duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestLexicographicOrdering(t *testing.T) {
	// ULIDs should be lexicographically sortable by time.
	id1 := NewPlan()
	id2 := NewPlan()
	id3 := NewPlan()

	if id1 >= id2 || id2 >= id3 {
		t.Errorf("IDs not in lexicographic order: %s, %s, %s", id1, id2, id3)
	}
}

func TestConvenienceFunctions(t *testing.T) {
	planID := NewPlan()
	if !strings.HasPrefix(planID, "plan-") {
		t.Errorf("NewPlan() = %q, want prefix 'plan-'", planID)
	}

	infraID := NewInfra()
	if !strings.HasPrefix(infraID, "infra-") {
		t.Errorf("NewInfra() = %q, want prefix 'infra-'", infraID)
	}

	deployID := NewDeploy()
	if !strings.HasPrefix(deployID, "deploy-") {
		t.Errorf("NewDeploy() = %q, want prefix 'deploy-'", deployID)
	}
}
