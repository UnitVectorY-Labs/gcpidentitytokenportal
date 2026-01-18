// Package logging provides structured logging utilities for the application.
package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Level represents the severity level of a log entry.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "unknown"
	}
}

// ParseLevel parses a log level string.
func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// Format represents the output format.
type Format int

const (
	FormatJSON Format = iota
	FormatText
)

// ParseFormat parses a format string.
func ParseFormat(s string) Format {
	switch strings.ToLower(s) {
	case "text":
		return FormatText
	default:
		return FormatJSON
	}
}

// Fields represents additional structured fields for a log entry.
type Fields map[string]interface{}

// Logger provides structured logging functionality.
type Logger struct {
	mu        sync.Mutex
	out       io.Writer
	level     Level
	format    Format
	component string
}

// contextKey is used for context values
type contextKey string

const (
	requestIDKey contextKey = "request_id"
	routeKey     contextKey = "route"
)

var defaultLogger *Logger

func init() {
	defaultLogger = New(os.Stdout, LevelInfo, FormatJSON)
}

// New creates a new Logger.
func New(out io.Writer, level Level, format Format) *Logger {
	return &Logger{
		out:    out,
		level:  level,
		format: format,
	}
}

// SetDefault sets the default logger.
func SetDefault(l *Logger) {
	defaultLogger = l
}

// Default returns the default logger.
func Default() *Logger {
	return defaultLogger
}

// WithComponent returns a new logger with the component field set.
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		out:       l.out,
		level:     l.level,
		format:    l.format,
		component: component,
	}
}

// logEntry represents a structured log entry.
type logEntry struct {
	Timestamp string                 `json:"timestamp"`
	Severity  string                 `json:"severity"`
	Component string                 `json:"component,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	Route     string                 `json:"route,omitempty"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

func (l *Logger) log(ctx context.Context, level Level, msg string, fields Fields) {
	if level < l.level {
		return
	}

	entry := logEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Severity:  level.String(),
		Component: l.component,
		Message:   msg,
	}

	// Extract context values
	if ctx != nil {
		if reqID, ok := ctx.Value(requestIDKey).(string); ok {
			entry.RequestID = reqID
		}
		if route, ok := ctx.Value(routeKey).(string); ok {
			entry.Route = route
		}
	}

	if len(fields) > 0 {
		entry.Fields = fields
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.format == FormatJSON {
		l.writeJSON(entry)
	} else {
		l.writeText(entry)
	}
}

func (l *Logger) writeJSON(entry logEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		// Fallback to text if JSON fails
		fmt.Fprintf(l.out, "%s [%s] %s\n", entry.Timestamp, entry.Severity, entry.Message)
		return
	}
	fmt.Fprintln(l.out, string(data))
}

func (l *Logger) writeText(entry logEntry) {
	var parts []string
	parts = append(parts, fmt.Sprintf("%s [%s]", entry.Timestamp, strings.ToUpper(entry.Severity)))

	if entry.Component != "" {
		parts = append(parts, fmt.Sprintf("[%s]", entry.Component))
	}
	if entry.RequestID != "" {
		parts = append(parts, fmt.Sprintf("request_id=%s", entry.RequestID))
	}
	if entry.Route != "" {
		parts = append(parts, fmt.Sprintf("route=%s", entry.Route))
	}

	parts = append(parts, entry.Message)

	if len(entry.Fields) > 0 {
		for k, v := range entry.Fields {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}

	fmt.Fprintln(l.out, strings.Join(parts, " "))
}

// Debug logs a message at debug level.
func (l *Logger) Debug(ctx context.Context, msg string, fields ...Fields) {
	f := mergeFields(fields)
	l.log(ctx, LevelDebug, msg, f)
}

// Info logs a message at info level.
func (l *Logger) Info(ctx context.Context, msg string, fields ...Fields) {
	f := mergeFields(fields)
	l.log(ctx, LevelInfo, msg, f)
}

// Warn logs a message at warn level.
func (l *Logger) Warn(ctx context.Context, msg string, fields ...Fields) {
	f := mergeFields(fields)
	l.log(ctx, LevelWarn, msg, f)
}

// Error logs a message at error level.
func (l *Logger) Error(ctx context.Context, msg string, fields ...Fields) {
	f := mergeFields(fields)
	l.log(ctx, LevelError, msg, f)
}

// mergeFields combines multiple Fields into one.
func mergeFields(fields []Fields) Fields {
	if len(fields) == 0 {
		return nil
	}
	result := make(Fields)
	for _, f := range fields {
		if f == nil {
			continue
		}
		for k, v := range f {
			result[k] = v
		}
	}
	return result
}

// WithRequestID adds a request ID to the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// GetRequestID retrieves the request ID from the context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// WithRoute adds a route to the context.
func WithRoute(ctx context.Context, route string) context.Context {
	return context.WithValue(ctx, routeKey, route)
}

// GetRoute retrieves the route from the context.
func GetRoute(ctx context.Context) string {
	if r, ok := ctx.Value(routeKey).(string); ok {
		return r
	}
	return ""
}

// Package-level functions using default logger

// Debug logs a message at debug level using the default logger.
func Debug(ctx context.Context, msg string, fields ...Fields) {
	defaultLogger.Debug(ctx, msg, fields...)
}

// Info logs a message at info level using the default logger.
func Info(ctx context.Context, msg string, fields ...Fields) {
	defaultLogger.Info(ctx, msg, fields...)
}

// Warn logs a message at warn level using the default logger.
func Warn(ctx context.Context, msg string, fields ...Fields) {
	defaultLogger.Warn(ctx, msg, fields...)
}

// Error logs a message at error level using the default logger.
func Error(ctx context.Context, msg string, fields ...Fields) {
	defaultLogger.Error(ctx, msg, fields...)
}
