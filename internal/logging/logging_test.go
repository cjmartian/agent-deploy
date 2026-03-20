package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestInitialize_DefaultsToText(t *testing.T) {
	var buf bytes.Buffer
	Initialize(WithOutput(&buf))

	Info("test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Error("Expected log output to contain message")
	}
	if !strings.Contains(output, "key=value") {
		t.Error("Expected log output to contain key=value")
	}
}

func TestInitialize_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	Initialize(WithOutput(&buf), WithFormat(FormatJSON))

	Info("test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, `"msg":"test message"`) {
		t.Error("Expected JSON log to contain msg field")
	}
	if !strings.Contains(output, `"key":"value"`) {
		t.Error("Expected JSON log to contain key field")
	}
}

func TestInitialize_WithLevel(t *testing.T) {
	var buf bytes.Buffer
	Initialize(WithOutput(&buf), WithLevel(slog.LevelWarn))

	Info("should not appear")
	Warn("should appear")

	output := buf.String()
	if strings.Contains(output, "should not appear") {
		t.Error("Info should be filtered at Warn level")
	}
	if !strings.Contains(output, "should appear") {
		t.Error("Warn should not be filtered at Warn level")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"unknown", slog.LevelInfo}, // Default
		{"", slog.LevelInfo},        // Default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseLevel(tt.input)
			if got != tt.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input string
		want  Format
	}{
		{"json", FormatJSON},
		{"JSON", FormatJSON},
		{"text", FormatText},
		{"TEXT", FormatText},
		{"unknown", FormatText}, // Default
		{"", FormatText},        // Default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseFormat(tt.input)
			if got != tt.want {
				t.Errorf("ParseFormat(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestWithComponent(t *testing.T) {
	var buf bytes.Buffer
	Initialize(WithOutput(&buf))

	logger := WithComponent(ComponentAWSProvider)
	logger.Info("component test")

	output := buf.String()
	if !strings.Contains(output, "component=aws") {
		t.Errorf("Expected component=aws in output, got: %s", output)
	}
}

func TestAttrHelpers(t *testing.T) {
	// Test that helpers create proper attributes
	tests := []struct {
		name string
		attr slog.Attr
		key  string
	}{
		{"DeploymentID", DeploymentID("deploy-123"), "deployment_id"},
		{"InfraID", InfraID("infra-456"), "infra_id"},
		{"PlanID", PlanID("plan-789"), "plan_id"},
		{"Region", Region("us-east-1"), "region"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.attr.Key != tt.key {
				t.Errorf("%s key = %q, want %q", tt.name, tt.attr.Key, tt.key)
			}
		})
	}
}

func TestErr(t *testing.T) {
	attr := Err(nil)
	if attr.Key != "error" {
		t.Errorf("Err key = %q, want 'error'", attr.Key)
	}
}

func TestCost(t *testing.T) {
	attr := Cost(25.50)
	if attr.Key != "cost_usd" {
		t.Errorf("Cost key = %q, want 'cost_usd'", attr.Key)
	}
	if attr.Value.Float64() != 25.50 {
		t.Errorf("Cost value = %v, want 25.50", attr.Value.Float64())
	}
}

func TestCount(t *testing.T) {
	attr := Count(42)
	if attr.Key != "count" {
		t.Errorf("Count key = %q, want 'count'", attr.Key)
	}
	if attr.Value.Int64() != 42 {
		t.Errorf("Count value = %v, want 42", attr.Value.Int64())
	}
}

func TestLogLevels(t *testing.T) {
	var buf bytes.Buffer
	Initialize(WithOutput(&buf), WithLevel(slog.LevelDebug))

	Debug("debug msg")
	Info("info msg")
	Warn("warn msg")
	Error("error msg")

	output := buf.String()
	if !strings.Contains(output, "debug msg") {
		t.Error("Expected debug message")
	}
	if !strings.Contains(output, "info msg") {
		t.Error("Expected info message")
	}
	if !strings.Contains(output, "warn msg") {
		t.Error("Expected warn message")
	}
	if !strings.Contains(output, "error msg") {
		t.Error("Expected error message")
	}
}

func TestComponentConstants(t *testing.T) {
	// Verify component constants are defined
	components := []string{
		ComponentServer,
		ComponentAWSProvider,
		ComponentState,
		ComponentSpending,
		ComponentCleanup,
		ComponentCostMonitor,
	}

	for _, c := range components {
		if c == "" {
			t.Error("Component constant should not be empty")
		}
	}
}
