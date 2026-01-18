package sanitizer

import (
	"strings"
	"testing"
)

func TestSanitizeJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
		contains []string
		excludes []string
	}{
		{
			name:     "empty input",
			input:    []byte{},
			expected: "",
		},
		{
			name:  "access_token field",
			input: []byte(`{"access_token":"ya29.secret-token-value"}`),
			contains: []string{"[REDACTED]"},
			excludes: []string{"ya29", "secret-token-value"},
		},
		{
			name:  "id_token field",
			input: []byte(`{"id_token":"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature"}`),
			contains: []string{"[REDACTED]"},
			excludes: []string{"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9"},
		},
		{
			name:  "token field",
			input: []byte(`{"token":"secret-identity-token"}`),
			contains: []string{"[REDACTED]"},
			excludes: []string{"secret-identity-token"},
		},
		{
			name:  "subject_token field",
			input: []byte(`{"subject_token":"kubernetes-sa-token.jwt.value"}`),
			contains: []string{"[REDACTED]"},
			excludes: []string{"kubernetes-sa-token"},
		},
		{
			name:  "nested sensitive field",
			input: []byte(`{"response":{"access_token":"secret","expires_in":3600}}`),
			contains: []string{"[REDACTED]", "expires_in", "3600"},
			excludes: []string{"secret"},
		},
		{
			name:  "error response preserved",
			input: []byte(`{"error":{"code":403,"status":"PERMISSION_DENIED","message":"Access denied"}}`),
			contains: []string{"403", "PERMISSION_DENIED", "Access denied"},
		},
		{
			name:  "STS error response",
			input: []byte(`{"error":"invalid_grant","error_description":"Token has expired"}`),
			contains: []string{"invalid_grant", "Token has expired"},
		},
		{
			name:  "mixed content with JWT in message",
			input: []byte(`{"error":{"message":"Invalid token: eyJhbG.eyJzdWIi.sig123"}}`),
			contains: []string{"[REDACTED_JWT]"},
			excludes: []string{"eyJhbG.eyJzdWIi.sig123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeJSON(tt.input)

			if tt.expected != "" && result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}

			for _, contain := range tt.contains {
				if !strings.Contains(result, contain) {
					t.Errorf("expected result to contain %q, got %q", contain, result)
				}
			}

			for _, exclude := range tt.excludes {
				if strings.Contains(result, exclude) {
					t.Errorf("expected result to NOT contain %q, got %q", exclude, result)
				}
			}
		})
	}
}

func TestSanitizeString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no JWT",
			input:    "simple error message",
			expected: "simple error message",
		},
		{
			name:     "JWT pattern",
			input:    "Invalid token: eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature",
			expected: "Invalid token: [REDACTED_JWT]",
		},
		{
			name:     "multiple JWT patterns",
			input:    "Token1: abc.def.ghi Token2: xyz.uvw.rst",
			expected: "Token1: [REDACTED_JWT] Token2: [REDACTED_JWT]",
		},
		{
			name:     "preserve non-JWT dots",
			input:    "api.example.com returned error",
			expected: "[REDACTED_JWT] returned error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeString(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestExtractGoogleError(t *testing.T) {
	tests := []struct {
		name         string
		input        []byte
		expectedCode int
		expectedStatus string
		expectedMsg  string
	}{
		{
			name:         "empty input",
			input:        []byte{},
			expectedCode: 0,
			expectedStatus: "",
			expectedMsg:  "",
		},
		{
			name:         "standard Google error",
			input:        []byte(`{"error":{"code":403,"status":"PERMISSION_DENIED","message":"Access denied to resource"}}`),
			expectedCode: 403,
			expectedStatus: "PERMISSION_DENIED",
			expectedMsg:  "Access denied to resource",
		},
		{
			name:         "STS OAuth error format",
			input:        []byte(`{"error":"invalid_grant","error_description":"Token has expired"}`),
			expectedCode: 0,
			expectedStatus: "invalid_grant",
			expectedMsg:  "Token has expired",
		},
		{
			name:         "error with JWT in message",
			input:        []byte(`{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"Bad token: abc.def.ghi"}}`),
			expectedCode: 400,
			expectedStatus: "INVALID_ARGUMENT",
			expectedMsg:  "Bad token: [REDACTED_JWT]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, status, msg := ExtractGoogleError(tt.input)
			
			if code != tt.expectedCode {
				t.Errorf("expected code %d, got %d", tt.expectedCode, code)
			}
			if status != tt.expectedStatus {
				t.Errorf("expected status %q, got %q", tt.expectedStatus, status)
			}
			if msg != tt.expectedMsg {
				t.Errorf("expected message %q, got %q", tt.expectedMsg, msg)
			}
		})
	}
}
