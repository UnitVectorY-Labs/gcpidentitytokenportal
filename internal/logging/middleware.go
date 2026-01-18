// Package logging provides HTTP middleware for request logging and correlation.
package logging

import (
	"net/http"
	"time"

	"github.com/google/uuid"
)

// RequestIDHeader is the header name for request correlation.
const RequestIDHeader = "X-Request-Id"

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.statusCode = http.StatusOK
		rw.written = true
	}
	return rw.ResponseWriter.Write(b)
}

// RequestIDMiddleware adds request ID handling to HTTP requests.
// It reads X-Request-Id header if present, otherwise generates a new UUID.
// The request ID is added to the response headers and request context.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(RequestIDHeader)
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Add request ID to response header
		w.Header().Set(RequestIDHeader, requestID)

		// Add request ID to context
		ctx := WithRequestID(r.Context(), requestID)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

// RequestLoggingMiddleware logs incoming HTTP requests with route, method, status, and latency.
func RequestLoggingMiddleware(logger *Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Add route to context
			ctx := WithRoute(r.Context(), r.URL.Path)
			r = r.WithContext(ctx)

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Call the next handler
			next.ServeHTTP(wrapped, r)

			// Log the request
			latency := time.Since(start)
			logger.WithComponent("http").Info(r.Context(), "request completed", Fields{
				"method":      r.Method,
				"path":        r.URL.Path,
				"status_code": wrapped.statusCode,
				"latency_ms":  latency.Milliseconds(),
			})
		})
	}
}

// ChainMiddleware chains multiple middleware functions.
func ChainMiddleware(middlewares ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(final http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			final = middlewares[i](final)
		}
		return final
	}
}
