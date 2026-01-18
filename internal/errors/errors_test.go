package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestCategorizedError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *CategorizedError
		expected string
	}{
		{
			name: "with underlying error",
			err: &CategorizedError{
				Category: STSHTTPError,
				Message:  "failed to call STS",
				Err:      fmt.Errorf("connection refused"),
			},
			expected: "failed to call STS: connection refused",
		},
		{
			name: "without underlying error",
			err: &CategorizedError{
				Category: ConfigMissing,
				Message:  "configuration file not found",
			},
			expected: "configuration file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestCategorizedError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("inner error")
	catErr := New(STSNon200, "STS returned error", inner)

	unwrapped := catErr.Unwrap()
	if unwrapped != inner {
		t.Errorf("expected inner error to be returned")
	}

	// Test errors.Is
	if !errors.Is(catErr, inner) {
		t.Error("errors.Is should find inner error")
	}
}

func TestNew(t *testing.T) {
	inner := fmt.Errorf("test error")
	catErr := New(IAMNon200, "IAM error", inner)

	if catErr.Category != IAMNon200 {
		t.Errorf("expected category %s, got %s", IAMNon200, catErr.Category)
	}
	if catErr.Message != "IAM error" {
		t.Errorf("expected message 'IAM error', got %s", catErr.Message)
	}
	if catErr.Err != inner {
		t.Error("expected inner error to be set")
	}
}

func TestWithOperation(t *testing.T) {
	catErr := New(STSHTTPError, "error", nil).WithOperation("sts_exchange")

	if catErr.Operation != "sts_exchange" {
		t.Errorf("expected operation 'sts_exchange', got %s", catErr.Operation)
	}
}

func TestWithStatusCode(t *testing.T) {
	catErr := New(STSNon200, "error", nil).WithStatusCode(403)

	if catErr.StatusCode != 403 {
		t.Errorf("expected status code 403, got %d", catErr.StatusCode)
	}
}

func TestCategorizeNetworkError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCategory
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: InternalError,
		},
		{
			name:     "dial error",
			err:      fmt.Errorf("dial tcp: lookup sts.googleapis.com: no such host"),
			expected: NetworkDNSError,
		},
		{
			name:     "timeout in message",
			err:      fmt.Errorf("context deadline exceeded"),
			expected: NetworkTimeout,
		},
		{
			name:     "generic error",
			err:      fmt.Errorf("something went wrong"),
			expected: InternalError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CategorizeNetworkError(tt.err)
			if got != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestGetCategory(t *testing.T) {
	catErr := New(AudienceInvalid, "invalid audience", nil)

	if got := GetCategory(catErr); got != AudienceInvalid {
		t.Errorf("expected %s, got %s", AudienceInvalid, got)
	}

	// Non-categorized error
	plainErr := fmt.Errorf("plain error")
	if got := GetCategory(plainErr); got != InternalError {
		t.Errorf("expected %s for plain error, got %s", InternalError, got)
	}
}

func TestGetOperation(t *testing.T) {
	catErr := New(STSHTTPError, "error", nil).WithOperation("sts_exchange")

	if got := GetOperation(catErr); got != "sts_exchange" {
		t.Errorf("expected 'sts_exchange', got %s", got)
	}

	// Non-categorized error
	plainErr := fmt.Errorf("plain error")
	if got := GetOperation(plainErr); got != "" {
		t.Errorf("expected empty string for plain error, got %s", got)
	}
}

func TestGetStatusCode(t *testing.T) {
	catErr := New(STSNon200, "error", nil).WithStatusCode(403)

	if got := GetStatusCode(catErr); got != 403 {
		t.Errorf("expected 403, got %d", got)
	}

	// Non-categorized error
	plainErr := fmt.Errorf("plain error")
	if got := GetStatusCode(plainErr); got != 0 {
		t.Errorf("expected 0 for plain error, got %d", got)
	}
}
