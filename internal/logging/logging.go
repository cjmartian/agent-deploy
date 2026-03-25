// Package logging provides structured logging for agent-deploy using log/slog.
//
// Why structured logging?
// - Machine-parseable output for log aggregation systems
// - Consistent log format across all components
// - Built-in support for log levels and contexts
// - Key-value attributes for filtering and searching
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Logger is the global structured logger.
var Logger *slog.Logger

// Component names used for log filtering and grouping.
const (
	ComponentServer      = "server"
	ComponentAWSProvider = "aws"
	ComponentState       = "state"
	ComponentSpending    = "spending"
	ComponentCleanup     = "cleanup"
	ComponentCostMonitor = "cost-monitor"
)

// Initialize sets up the global structured logger.
// Call this once at application startup.
func Initialize(opts ...Option) {
	config := &Config{
		Level:  slog.LevelInfo,
		Format: FormatText,
		Output: os.Stderr,
	}

	for _, opt := range opts {
		opt(config)
	}

	var handler slog.Handler
	handlerOpts := &slog.HandlerOptions{
		Level:     config.Level,
		AddSource: config.AddSource,
	}

	switch config.Format {
	case FormatJSON:
		handler = slog.NewJSONHandler(config.Output, handlerOpts)
	default:
		handler = slog.NewTextHandler(config.Output, handlerOpts)
	}

	Logger = slog.New(handler)
	slog.SetDefault(Logger)
}

// Config holds logging configuration.
type Config struct {
	Level     slog.Level
	Format    Format
	Output    io.Writer
	AddSource bool
}

// Format specifies the log output format.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

// Option configures the logger.
type Option func(*Config)

// WithLevel sets the minimum log level.
func WithLevel(level slog.Level) Option {
	return func(c *Config) {
		c.Level = level
	}
}

// WithFormat sets the output format (text or JSON).
func WithFormat(format Format) Option {
	return func(c *Config) {
		c.Format = format
	}
}

// WithOutput sets the output writer.
func WithOutput(w io.Writer) Option {
	return func(c *Config) {
		c.Output = w
	}
}

// WithSource includes source file and line in logs.
func WithSource() Option {
	return func(c *Config) {
		c.AddSource = true
	}
}

// ParseLevel converts a string to a slog.Level.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ParseFormat converts a string to a Format.
func ParseFormat(s string) Format {
	switch strings.ToLower(s) {
	case "json":
		return FormatJSON
	default:
		return FormatText
	}
}

// --- Convenience functions ---

// Debug logs at debug level.
func Debug(msg string, args ...any) {
	if Logger != nil {
		Logger.Debug(msg, args...)
	}
}

// Info logs at info level.
func Info(msg string, args ...any) {
	if Logger != nil {
		Logger.Info(msg, args...)
	}
}

// Warn logs at warn level.
func Warn(msg string, args ...any) {
	if Logger != nil {
		Logger.Warn(msg, args...)
	}
}

// Error logs at error level.
func Error(msg string, args ...any) {
	if Logger != nil {
		Logger.Error(msg, args...)
	}
}

// With returns a logger with the given attributes.
func With(args ...any) *slog.Logger {
	if Logger != nil {
		return Logger.With(args...)
	}
	return slog.Default().With(args...)
}

// WithComponent returns a logger tagged with a component name.
func WithComponent(component string) *slog.Logger {
	return With("component", component)
}

// WithContext returns a logger with context values.
func WithContext(ctx context.Context) *slog.Logger {
	// Future: extract trace IDs or request IDs from context
	if Logger != nil {
		return Logger
	}
	return slog.Default()
}

// --- Attribute helpers ---

// Attr creates a slog.Attr with the given key and value.
func Attr(key string, value any) slog.Attr {
	return slog.Any(key, value)
}

// Err creates an error attribute.
func Err(err error) slog.Attr {
	return slog.Any("error", err)
}

// DeploymentID creates a deployment_id attribute.
func DeploymentID(id string) slog.Attr {
	return slog.String("deployment_id", id)
}

// InfraID creates an infra_id attribute.
func InfraID(id string) slog.Attr {
	return slog.String("infra_id", id)
}

// PlanID creates a plan_id attribute.
func PlanID(id string) slog.Attr {
	return slog.String("plan_id", id)
}

// Region creates a region attribute.
func Region(region string) slog.Attr {
	return slog.String("region", region)
}

// Cost creates a cost_usd attribute.
func Cost(usd float64) slog.Attr {
	return slog.Float64("cost_usd", usd)
}

// Count creates a count attribute.
func Count(n int) slog.Attr {
	return slog.Int("count", n)
}
