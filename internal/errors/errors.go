// Package errors provides error categories and utilities for consistent error handling.
package errors

import (
	"errors"
	"net"
	"strings"
)

// ErrorCategory represents a high-level error classification for debugging.
type ErrorCategory string

const (
	// Configuration errors
	ConfigMissing    ErrorCategory = "CONFIG_MISSING"
	ConfigParseError ErrorCategory = "CONFIG_PARSE_ERROR"

	// Token file errors
	TokenFileReadError ErrorCategory = "TOKEN_FILE_READ_ERROR"

	// STS errors
	STSHTTPError          ErrorCategory = "STS_HTTP_ERROR"
	STSNon200             ErrorCategory = "STS_NON_200"
	STSResponseDecodeError ErrorCategory = "STS_RESPONSE_DECODE_ERROR"
	STSEmptyAccessToken   ErrorCategory = "STS_EMPTY_ACCESS_TOKEN"

	// IAM errors
	IAMHTTPError          ErrorCategory = "IAM_HTTP_ERROR"
	IAMNon200             ErrorCategory = "IAM_NON_200"
	IAMResponseDecodeError ErrorCategory = "IAM_RESPONSE_DECODE_ERROR"
	IAMEmptyToken         ErrorCategory = "IAM_EMPTY_TOKEN"

	// Audience errors
	AudienceInvalid ErrorCategory = "AUDIENCE_INVALID"

	// Network errors
	NetworkDNSError ErrorCategory = "NETWORK_DNS_ERROR"
	NetworkTimeout  ErrorCategory = "NETWORK_TIMEOUT"

	// Generic errors
	InternalError ErrorCategory = "INTERNAL_ERROR"
)

// CategorizedError wraps an error with a category and additional context.
type CategorizedError struct {
	Category   ErrorCategory
	Message    string
	StatusCode int    // HTTP status code if applicable
	Operation  string // e.g., "sts_exchange", "generate_id_token"
	Err        error  // underlying error
}

func (e *CategorizedError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *CategorizedError) Unwrap() error {
	return e.Err
}

// New creates a new CategorizedError.
func New(category ErrorCategory, message string, err error) *CategorizedError {
	return &CategorizedError{
		Category: category,
		Message:  message,
		Err:      err,
	}
}

// WithOperation adds operation context to the error.
func (e *CategorizedError) WithOperation(op string) *CategorizedError {
	e.Operation = op
	return e
}

// WithStatusCode adds HTTP status code context to the error.
func (e *CategorizedError) WithStatusCode(code int) *CategorizedError {
	e.StatusCode = code
	return e
}

// CategorizeNetworkError determines the appropriate error category for network errors.
func CategorizeNetworkError(err error) ErrorCategory {
	if err == nil {
		return InternalError
	}

	// Check for DNS errors
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return NetworkDNSError
	}

	// Check for timeout
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return NetworkTimeout
	}

	// Check error message for common patterns
	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "dial") || strings.Contains(errStr, "dns") || strings.Contains(errStr, "lookup") {
		return NetworkDNSError
	}
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline") {
		return NetworkTimeout
	}

	return InternalError
}

// GetCategory extracts the ErrorCategory from an error if it's a CategorizedError.
func GetCategory(err error) ErrorCategory {
	var catErr *CategorizedError
	if errors.As(err, &catErr) {
		return catErr.Category
	}
	return InternalError
}

// GetOperation extracts the operation from an error if it's a CategorizedError.
func GetOperation(err error) string {
	var catErr *CategorizedError
	if errors.As(err, &catErr) {
		return catErr.Operation
	}
	return ""
}

// GetStatusCode extracts the HTTP status code from an error if available.
func GetStatusCode(err error) int {
	var catErr *CategorizedError
	if errors.As(err, &catErr) {
		return catErr.StatusCode
	}
	return 0
}
