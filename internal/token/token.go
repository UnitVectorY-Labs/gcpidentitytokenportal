package token

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	gcp_config "github.com/UnitVectorY-Labs/gcpidentitytokenportal/internal/config"
	apperrors "github.com/UnitVectorY-Labs/gcpidentitytokenportal/internal/errors"
	"github.com/UnitVectorY-Labs/gcpidentitytokenportal/internal/logging"
	"github.com/UnitVectorY-Labs/gcpidentitytokenportal/internal/sanitizer"
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
func GetIdentityToken(ctx context.Context, config *gcp_config.GoogleApplicationCredentials, audience string) (string, error) {
	logger := logging.Default().WithComponent("token")

	jwt, err := os.ReadFile(config.CredentialSource.File)
	if err != nil {
		catErr := apperrors.New(apperrors.TokenFileReadError, "failed to read Kubernetes token file", err)
		logger.Error(ctx, "token file read error", logging.Fields{
			"error_category": string(catErr.Category),
			"file_path":      config.CredentialSource.File,
		})
		return "", catErr
	}

	accessToken, err := exchangeToken(ctx, config, string(jwt))
	if err != nil {
		// Error already logged in exchangeToken
		return "", err
	}

	identityToken, err := generateIdentityToken(ctx, config, accessToken, audience)
	if err != nil {
		// Error already logged in generateIdentityToken
		return "", err
	}

	logger.Debug(ctx, "identity token generated successfully", logging.Fields{
		"audience": audience,
	})

	return identityToken, nil
}

// exchangeToken performs the STS token exchange
func exchangeToken(ctx context.Context, config *gcp_config.GoogleApplicationCredentials, subjectToken string) (string, error) {
	logger := logging.Default().WithComponent("sts")
	const operation = "sts_exchange"

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
		catErr := apperrors.New(apperrors.InternalError, "failed to marshal STS request", err).WithOperation(operation)
		logger.Error(ctx, "STS request marshal error", logging.Fields{
			"error_category": string(catErr.Category),
			"operation":      operation,
		})
		return "", catErr
	}

	start := time.Now()
	resp, err := http.Post(stsUrl, "application/json", bytes.NewBuffer(body))
	latency := time.Since(start)

	if err != nil {
		category := apperrors.CategorizeNetworkError(err)
		if category == apperrors.InternalError {
			category = apperrors.STSHTTPError
		}
		catErr := apperrors.New(category, "failed to call STS", err).WithOperation(operation)
		logger.Error(ctx, "STS call failed", logging.Fields{
			"error_category": string(catErr.Category),
			"operation":      operation,
			"host":           "sts.googleapis.com",
			"latency_ms":     latency.Milliseconds(),
		})
		return "", catErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		code, status, message := sanitizer.ExtractGoogleError(respBody)
		
		catErr := apperrors.New(apperrors.STSNon200, "STS returned non-OK status", nil).
			WithOperation(operation).
			WithStatusCode(resp.StatusCode)
		
		logger.Error(ctx, "STS returned error", logging.Fields{
			"error_category":   string(catErr.Category),
			"operation":        operation,
			"host":             "sts.googleapis.com",
			"http_status":      resp.StatusCode,
			"google_code":      code,
			"google_status":    status,
			"sanitized_message": message,
			"latency_ms":       latency.Milliseconds(),
		})
		return "", catErr
	}

	var stsResp STSResponse
	if err := json.NewDecoder(resp.Body).Decode(&stsResp); err != nil {
		catErr := apperrors.New(apperrors.STSResponseDecodeError, "failed to decode STS response", err).WithOperation(operation)
		logger.Error(ctx, "STS response decode error", logging.Fields{
			"error_category": string(catErr.Category),
			"operation":      operation,
			"latency_ms":     latency.Milliseconds(),
		})
		return "", catErr
	}

	if stsResp.AccessToken == "" {
		catErr := apperrors.New(apperrors.STSEmptyAccessToken, "empty access token received from STS", nil).WithOperation(operation)
		logger.Error(ctx, "STS returned empty access token", logging.Fields{
			"error_category": string(catErr.Category),
			"operation":      operation,
			"latency_ms":     latency.Milliseconds(),
		})
		return "", catErr
	}

	logger.Info(ctx, "STS token exchange successful", logging.Fields{
		"operation":   operation,
		"host":        "sts.googleapis.com",
		"http_status": resp.StatusCode,
		"latency_ms":  latency.Milliseconds(),
		"expires_in":  stsResp.ExpiresIn,
	})

	return stsResp.AccessToken, nil
}

// generateIdentityToken calls IAM to generate an identity token
func generateIdentityToken(ctx context.Context, config *gcp_config.GoogleApplicationCredentials, accessToken, audience string) (string, error) {
	logger := logging.Default().WithComponent("iam")
	const operation = "generate_id_token"

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
		catErr := apperrors.New(apperrors.InternalError, "failed to marshal IAM request", err).WithOperation(operation)
		logger.Error(ctx, "IAM request marshal error", logging.Fields{
			"error_category": string(catErr.Category),
			"operation":      operation,
		})
		return "", catErr
	}

	req, err := http.NewRequest("POST", iamCredentialsURL, bytes.NewBuffer(body))
	if err != nil {
		catErr := apperrors.New(apperrors.InternalError, "failed to create IAM request", err).WithOperation(operation)
		logger.Error(ctx, "IAM request creation error", logging.Fields{
			"error_category": string(catErr.Category),
			"operation":      operation,
		})
		return "", catErr
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)

	if err != nil {
		category := apperrors.CategorizeNetworkError(err)
		if category == apperrors.InternalError {
			category = apperrors.IAMHTTPError
		}
		catErr := apperrors.New(category, "failed to call IAM", err).WithOperation(operation)
		logger.Error(ctx, "IAM call failed", logging.Fields{
			"error_category": string(catErr.Category),
			"operation":      operation,
			"host":           "iamcredentials.googleapis.com",
			"latency_ms":     latency.Milliseconds(),
		})
		return "", catErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		code, status, message := sanitizer.ExtractGoogleError(respBody)
		
		catErr := apperrors.New(apperrors.IAMNon200, "IAM returned non-OK status", nil).
			WithOperation(operation).
			WithStatusCode(resp.StatusCode)
		
		logger.Error(ctx, "IAM returned error", logging.Fields{
			"error_category":    string(catErr.Category),
			"operation":         operation,
			"host":              "iamcredentials.googleapis.com",
			"http_status":       resp.StatusCode,
			"google_code":       code,
			"google_status":     status,
			"sanitized_message": message,
			"latency_ms":        latency.Milliseconds(),
		})
		return "", catErr
	}

	var iamResp IAMResponse
	if err := json.NewDecoder(resp.Body).Decode(&iamResp); err != nil {
		catErr := apperrors.New(apperrors.IAMResponseDecodeError, "failed to decode IAM response", err).WithOperation(operation)
		logger.Error(ctx, "IAM response decode error", logging.Fields{
			"error_category": string(catErr.Category),
			"operation":      operation,
			"latency_ms":     latency.Milliseconds(),
		})
		return "", catErr
	}

	if iamResp.Token == "" {
		catErr := apperrors.New(apperrors.IAMEmptyToken, "empty identity token received from IAM", nil).WithOperation(operation)
		logger.Error(ctx, "IAM returned empty token", logging.Fields{
			"error_category": string(catErr.Category),
			"operation":      operation,
			"latency_ms":     latency.Milliseconds(),
		})
		return "", catErr
	}

	logger.Info(ctx, "IAM identity token generated", logging.Fields{
		"operation":   operation,
		"host":        "iamcredentials.googleapis.com",
		"http_status": resp.StatusCode,
		"latency_ms":  latency.Milliseconds(),
	})

	return iamResp.Token, nil
}
