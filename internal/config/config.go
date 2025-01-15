package config

import (
	"encoding/json"
	"os"
	"strings"
)

// GoogleApplicationCredentials holds the Google external account configuration file if it exists
type GoogleApplicationCredentials struct {
	UniverseDomain   string `json:"universe_domain"`
	Type             string `json:"type"`
	Audience         string `json:"audience"`
	SubjectTokenType string `json:"subject_token_type"`
	TokenURL         string `json:"token_url"`
	CredentialSource struct {
		File   string `json:"file"`
		Format struct {
			Type string `json:"type"`
		} `json:"format"`
	} `json:"credential_source"`
	ServiceAccountImpersonationURL string `json:"service_account_impersonation_url"`
}

func (g *GoogleApplicationCredentials) UsesImpersonation() bool {
	return g.ServiceAccountImpersonationURL != ""
}

func (g *GoogleApplicationCredentials) GetImpersonationEmail() string {
	if g.ServiceAccountImpersonationURL == "" {
		return ""
	}
	// Split the URL at "serviceAccounts/" and take the part before the colon
	parts := strings.Split(g.ServiceAccountImpersonationURL, "serviceAccounts/")
	if len(parts) < 2 {
		return ""
	}
	email := strings.Split(parts[1], ":")[0]
	return email
}

// Load the google config from a provided file path, return an error if it doesn't exist
func LoadGoogleConfig(path string) (*GoogleApplicationCredentials, error) {
	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Unmarshal the data into the struct
	var googleConfig GoogleApplicationCredentials
	if err := json.Unmarshal(data, &googleConfig); err != nil {
		return nil, err
	}

	return &googleConfig, nil
}
