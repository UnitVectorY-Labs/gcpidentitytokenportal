// Package handlers provides HTTP handlers for the application.
package handlers

import (
	"encoding/json"
	"html/template"
	"net/http"
	"os"

	gcp_config "github.com/UnitVectorY-Labs/gcpidentitytokenportal/internal/config"
)

// HealthzHandler returns a simple health check handler.
func HealthzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}
}

// ReadyzConfig holds the configuration for the readiness check.
type ReadyzConfig struct {
	Template                      *template.Template
	ConfigLoaded                  bool
	CredentialsRequired           bool
	CredentialsFile               string
	GoogleApplicationCredentials  *gcp_config.GoogleApplicationCredentials
}

// ReadyzHandler returns a readiness check handler.
func ReadyzHandler(cfg ReadyzConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check template
		if cfg.Template == nil {
			http.Error(w, "template not loaded", http.StatusServiceUnavailable)
			return
		}

		// Check config loaded (only if config file exists)
		// ConfigLoaded is true if config was successfully parsed (even if empty)

		// Check credentials file exists if required
		if cfg.CredentialsRequired && cfg.CredentialsFile != "" {
			if _, err := os.Stat(cfg.CredentialsFile); os.IsNotExist(err) {
				http.Error(w, "credentials file not found", http.StatusServiceUnavailable)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}
}

// DebugzConfig holds the configuration for the debug endpoint.
type DebugzConfig struct {
	Mode                         string // "impersonation" or "direct"
	ImpersonationEmail           string
	WIFAudience                  string
	TokenFilePath                string
	ConfigPath                   string
	ConfigExists                 bool
	AllowedAudiencesCount        int
	GoogleApplicationCredentials *gcp_config.GoogleApplicationCredentials
}

// DebugzResponse represents the response from the debug endpoint.
type DebugzResponse struct {
	Mode                   string `json:"mode"`
	ImpersonationEmail     string `json:"impersonation_email,omitempty"`
	WIFAudience            string `json:"wif_audience,omitempty"`
	TokenFileExists        bool   `json:"token_file_exists"`
	TokenFileReadable      bool   `json:"token_file_readable"`
	ConfigExists           bool   `json:"config_exists"`
	AllowedAudiencesCount  int    `json:"allowed_audiences_count"`
	RequestID              string `json:"request_id,omitempty"`
}

// DebugzHandler returns a debug diagnostics handler.
// It should only be enabled when ENABLE_DEBUG_ENDPOINTS=true.
func DebugzHandler(cfg DebugzConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := DebugzResponse{
			Mode:                  cfg.Mode,
			ImpersonationEmail:    cfg.ImpersonationEmail,
			WIFAudience:           cfg.WIFAudience,
			ConfigExists:          cfg.ConfigExists,
			AllowedAudiencesCount: cfg.AllowedAudiencesCount,
		}

		// Check token file exists and readable
		if cfg.TokenFilePath != "" {
			if _, err := os.Stat(cfg.TokenFilePath); err == nil {
				resp.TokenFileExists = true
				if file, err := os.Open(cfg.TokenFilePath); err == nil {
					file.Close()
					resp.TokenFileReadable = true
				}
			}
		}

		// Get request ID from header if present
		if reqID := r.Header.Get("X-Request-Id"); reqID != "" {
			resp.RequestID = reqID
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
