// Package sanitizer provides utilities for redacting sensitive data from logs.
package sanitizer

import (
	"encoding/json"
	"regexp"
	"strings"
)

// sensitiveFields lists JSON keys whose values should be redacted
var sensitiveFields = []string{
	"access_token",
	"id_token",
	"token",
	"subject_token",
	"authorization",
	"Authorization",
}

// jwtPattern matches JWT-like strings (xxx.yyy.zzz where each part has minimum length typical of JWTs)
var jwtPattern = regexp.MustCompile(`\b[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}\b`)

// RedactedValue is the replacement for sensitive field values
const RedactedValue = "[REDACTED]"

// RedactedJWT is the replacement for JWT-like strings
const RedactedJWT = "[REDACTED_JWT]"

// SanitizeJSON redacts sensitive fields from a JSON byte slice.
// Returns the sanitized JSON string.
func SanitizeJSON(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		// If not valid JSON, try to sanitize as plain text
		return SanitizeString(string(data))
	}

	sanitized := sanitizeMap(parsed)
	result, err := json.Marshal(sanitized)
	if err != nil {
		return SanitizeString(string(data))
	}

	return string(result)
}

// sanitizeMap recursively redacts sensitive fields from a map
func sanitizeMap(m map[string]any) map[string]any {
	result := make(map[string]any)

	for key, value := range m {
		if isSensitiveField(key) {
			result[key] = RedactedValue
			continue
		}

		switch v := value.(type) {
		case map[string]any:
			result[key] = sanitizeMap(v)
		case []any:
			result[key] = sanitizeSlice(v)
		case string:
			result[key] = SanitizeString(v)
		default:
			result[key] = v
		}
	}

	return result
}

// sanitizeSlice recursively sanitizes elements in a slice
func sanitizeSlice(s []any) []any {
	result := make([]any, len(s))

	for i, value := range s {
		switch v := value.(type) {
		case map[string]any:
			result[i] = sanitizeMap(v)
		case []any:
			result[i] = sanitizeSlice(v)
		case string:
			result[i] = SanitizeString(v)
		default:
			result[i] = v
		}
	}

	return result
}

// SanitizeString redacts JWT-like patterns from a string
func SanitizeString(s string) string {
	return jwtPattern.ReplaceAllString(s, RedactedJWT)
}

// isSensitiveField checks if a field name should be redacted
func isSensitiveField(field string) bool {
	fieldLower := strings.ToLower(field)
	for _, sensitive := range sensitiveFields {
		if strings.ToLower(sensitive) == fieldLower {
			return true
		}
	}
	return false
}

// ExtractGoogleError extracts and sanitizes error details from a Google API error response.
// Returns the error code, status, and sanitized message.
func ExtractGoogleError(body []byte) (code int, status string, message string) {
	if len(body) == 0 {
		return 0, "", ""
	}

	var errResp struct {
		Error struct {
			Code    int    `json:"code"`
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil {
		// Try alternative format without nested error object
		var altResp struct {
			ErrorCode string `json:"error"`
			ErrorDesc string `json:"error_description"`
		}
		if err := json.Unmarshal(body, &altResp); err != nil {
			return 0, "", SanitizeString(string(body))
		}
		return 0, altResp.ErrorCode, SanitizeString(altResp.ErrorDesc)
	}

	return errResp.Error.Code, errResp.Error.Status, SanitizeString(errResp.Error.Message)
}
