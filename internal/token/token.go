package token

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	gcp_config "github.com/UnitVectorY-Labs/gcpidentitytokenportal/internal/config"
)

const (
	stsUrl                   = "https://sts.googleapis.com/v1/token"
	workloadIdentityPattern  = "//iam.googleapis.com/projects/%s/locations/global/workloadIdentityPools/%s/providers/%s"
	serviceAccountUrlPattern = "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateIdToken"

	// OAuth
	grantType          = "urn:ietf:params:oauth:grant-type:token-exchange"
	scope              = "https://www.googleapis.com/auth/cloud-platform"
	requestedTokenType = "urn:ietf:params:oauth:token-type:access_token"
	subjectTokenType   = "urn:ietf:params:oauth:token-type:jwt"
)

// STSRequest represents the request payload for STS token exchange
type STSRequest struct {
	GrantType          string `json:"grant_type"`
	Audience           string `json:"audience"`
	Scope              string `json:"scope"`
	RequestedTokenType string `json:"requested_token_type"`
	SubjectTokenType   string `json:"subject_token_type"`
	SubjectToken       string `json:"subject_token"`
}

// STSResponse represents the response from STS token exchange
type STSResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// IAMRequest represents the request payload for IAM impersonation
type IAMRequest struct {
	Audience     string `json:"audience"`
	IncludeEmail bool   `json:"includeEmail"`
}

// IAMResponse represents the response from IAM impersonation
type IAMResponse struct {
	Token string `json:"token"`
}

// GetIdentityToken generates an identity token for the specified audience
func GetIdentityToken(config *gcp_config.GoogleApplicationCredentials, audience string) (string, error) {

	jwt, err := os.ReadFile(config.CredentialSource.File)
	if err != nil {
		return "", fmt.Errorf("failed to read JWT: %v", err)
	}

	accessToken, err := exchangeToken(config, string(jwt))
	if err != nil {
		return "", fmt.Errorf("failed to exchange token: %v", err)
	}

	identityToken, err := generateIdentityToken(config, accessToken, audience)
	if err != nil {
		return "", fmt.Errorf("failed to generate identity token: %v", err)
	}

	return identityToken, nil
}

// exchangeToken performs the STS token exchange
func exchangeToken(config *gcp_config.GoogleApplicationCredentials, subjectToken string) (string, error) {

	audience := config.Audience

	requestPayload := STSRequest{
		GrantType:          grantType,
		Audience:           audience,
		Scope:              scope,
		RequestedTokenType: requestedTokenType,
		SubjectTokenType:   subjectTokenType,
		SubjectToken:       subjectToken,
	}

	body, err := json.Marshal(requestPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal STS request: %v", err)
	}

	resp, err := http.Post(stsUrl, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to call STS: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("STS returned non-OK status: %s, body: %s", resp.Status, string(respBody))
	}

	var stsResp STSResponse
	if err := json.NewDecoder(resp.Body).Decode(&stsResp); err != nil {
		return "", fmt.Errorf("failed to decode STS response: %v", err)
	}

	if stsResp.AccessToken == "" {
		return "", errors.New("empty access token received from STS")
	}

	return stsResp.AccessToken, nil
}

// generateIdentityToken calls IAM to generate an identity token
func generateIdentityToken(config *gcp_config.GoogleApplicationCredentials, accessToken, audience string) (string, error) {

	// If the URL for the service account impersonation is for generating access
	// tokens, then change it to generate ID tokens which is what we need
	iamCredentialsURL := config.ServiceAccountImpersonationURL
	if strings.HasSuffix(iamCredentialsURL, ":generateAccessToken") {
		iamCredentialsURL = iamCredentialsURL[:len(iamCredentialsURL)-20] + ":generateIdToken"
	}

	requestPayload := IAMRequest{
		Audience:     audience,
		IncludeEmail: true,
	}

	body, err := json.Marshal(requestPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal IAM request: %v", err)
	}

	req, err := http.NewRequest("POST", iamCredentialsURL, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create IAM request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call IAM: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("IAM returned non-OK status: %s, body: %s", resp.Status, string(respBody))
	}

	var iamResp IAMResponse
	if err := json.NewDecoder(resp.Body).Decode(&iamResp); err != nil {
		return "", fmt.Errorf("failed to decode IAM response: %v", err)
	}

	if iamResp.Token == "" {
		return "", errors.New("empty identity token received from IAM")
	}

	return iamResp.Token, nil
}
