package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
	"gopkg.in/yaml.v2"

	gcp_config "github.com/UnitVectorY-Labs/gcpidentitytokenportal/internal/config"
	apperrors "github.com/UnitVectorY-Labs/gcpidentitytokenportal/internal/errors"
	"github.com/UnitVectorY-Labs/gcpidentitytokenportal/internal/handlers"
	"github.com/UnitVectorY-Labs/gcpidentitytokenportal/internal/logging"
	token "github.com/UnitVectorY-Labs/gcpidentitytokenportal/internal/token"
)

//go:embed templates/*
var templatesFS embed.FS

// Version information set at build time
var (
	Version   = "dev"
	BuildTime = "unknown"
)

// Config holds the application configuration
type Config struct {
	Audiences []string `yaml:"audiences"`
}

func handleIndex(tmpl *template.Template, cfg Config) http.HandlerFunc {
	logger := logging.Default().WithComponent("ui")
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		err := tmpl.ExecuteTemplate(w, "index.html", cfg)
		if err != nil {
			requestID := logging.GetRequestID(r.Context())
			logger.Error(r.Context(), "template execution error", logging.Fields{
				"error": err.Error(),
			})
			http.Error(w, fmt.Sprintf("Internal Server Error. request_id=%s", requestID), http.StatusInternalServerError)
		}
	}
}

func handleToken(ctx context.Context, cfg Config, credentialsFile string, googleApplicationCredentials *gcp_config.GoogleApplicationCredentials) http.HandlerFunc {
	logger := logging.Default().WithComponent("token")
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := logging.GetRequestID(r.Context())
		usesImpersonation := googleApplicationCredentials != nil && googleApplicationCredentials.UsesImpersonation()

		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := r.ParseForm(); err != nil {
			logger.Warn(r.Context(), "invalid form data", logging.Fields{
				"error": err.Error(),
			})
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}

		audience := r.FormValue("audience")

		if len(cfg.Audiences) > 0 {
			valid := false
			for _, a := range cfg.Audiences {
				if a == audience {
					valid = true
					break
				}
			}
			if !valid {
				logger.Warn(r.Context(), "invalid audience selected", logging.Fields{
					"error_category": string(apperrors.AudienceInvalid),
					"audience":       audience,
				})
				http.Error(w, fmt.Sprintf("Invalid audience selected. request_id=%s", requestID), http.StatusBadRequest)
				return
			}
		}

		if usesImpersonation {
			idToken, err := token.GetIdentityToken(r.Context(), googleApplicationCredentials, audience)
			if err != nil {
				category := apperrors.GetCategory(err)
				logger.Error(r.Context(), "failed to get identity token", logging.Fields{
					"error_category":     string(category),
					"audience":           audience,
					"uses_impersonation": true,
				})
				http.Error(w, fmt.Sprintf("Failed to get identity token. request_id=%s", requestID), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(idToken))
			return
		}

		var ts oauth2.TokenSource
		var err error
		if credentialsFile != "" {
			ts, err = idtoken.NewTokenSource(ctx, audience, idtoken.WithCredentialsFile(credentialsFile))
		} else {
			ts, err = idtoken.NewTokenSource(ctx, audience)
		}

		if err != nil {
			logger.Error(r.Context(), "failed to create token source", logging.Fields{
				"error_category":     string(apperrors.InternalError),
				"audience":           audience,
				"uses_impersonation": false,
				"error":              err.Error(),
			})
			http.Error(w, fmt.Sprintf("Failed to create token source. request_id=%s", requestID), http.StatusInternalServerError)
			return
		}

		idToken, err := ts.Token()
		if err != nil {
			logger.Error(r.Context(), "failed to get token", logging.Fields{
				"error_category":     string(apperrors.InternalError),
				"audience":           audience,
				"uses_impersonation": false,
				"error":              err.Error(),
			})
			http.Error(w, fmt.Sprintf("Failed to get token. request_id=%s", requestID), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(idToken.AccessToken))
	}
}

func handleServiceAccount(credentialsFile string, googleApplicationCredentials *gcp_config.GoogleApplicationCredentials) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var email string
		var err error

		if googleApplicationCredentials != nil && googleApplicationCredentials.UsesImpersonation() {
			email = googleApplicationCredentials.GetImpersonationEmail()
		} else if metadata.OnGCE() {
			email, err = metadata.EmailWithContext(context.Background(), "")
			if err != nil {
				http.Error(w, "Failed to get service account email", http.StatusInternalServerError)
				return
			}
		} else {
			credBytes, err := os.ReadFile(credentialsFile)
			if err != nil {
				http.Error(w, "Failed to read credentials file", http.StatusInternalServerError)
				return
			}

			var creds struct {
				ClientEmail string `json:"client_email"`
			}
			if err := json.Unmarshal(credBytes, &creds); err != nil {
				http.Error(w, "Failed to parse credentials", http.StatusInternalServerError)
				return
			}
			email = creds.ClientEmail
		}

		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`
		<label>Service Account:</label>
		<input type="text" value="%s" disabled>
	`, email)))
	}
}

func main() {
	ctx := context.Background()

	// Initialize logger from environment variables
	logLevel := logging.ParseLevel(os.Getenv("LOG_LEVEL"))
	logFormat := logging.ParseFormat(os.Getenv("LOG_FORMAT"))
	logger := logging.New(os.Stdout, logLevel, logFormat)
	logging.SetDefault(logger)

	startupLogger := logger.WithComponent("startup")

	// Log startup information
	startupLogger.Info(ctx, "starting gcpidentitytokenportal", logging.Fields{
		"version":    Version,
		"build_time": BuildTime,
	})

	// Load configuration
	cfg, configExists, err := loadConfig()
	if err != nil {
		startupLogger.Error(ctx, "failed to load configuration", logging.Fields{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	startupLogger.Info(ctx, "configuration loaded", logging.Fields{
		"config_exists":     configExists,
		"audiences_count":   len(cfg.Audiences),
	})

	// Parse HTML template from embedded filesystem with version function
	tmpl, err := template.New("index.html").Funcs(template.FuncMap{
		"version": func() string { return Version },
	}).ParseFS(templatesFS, "templates/index.html")
	if err != nil {
		startupLogger.Error(ctx, "failed to parse template", logging.Fields{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	startupLogger.Info(ctx, "template loaded successfully", nil)

	// Load credentials directly
	credentialsFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	credentialsSet := credentialsFile != ""
	onGCE := metadata.OnGCE()

	startupLogger.Info(ctx, "credentials configuration", logging.Fields{
		"google_application_credentials_set": credentialsSet,
		"running_on_gce":                     onGCE,
	})

	if credentialsFile == "" && !onGCE {
		startupLogger.Error(ctx, "no credentials provided", logging.Fields{
			"hint": "Set GOOGLE_APPLICATION_CREDENTIALS or run on GCP",
		})
		os.Exit(1)
	}

	// Initialize GoogleApplicationCredentials if the credentials file exists
	var googleApplicationCredentials *gcp_config.GoogleApplicationCredentials
	usesImpersonation := false
	impersonationEmail := ""
	wifAudience := ""
	tokenFilePath := ""

	if credentialsFile != "" {
		if _, err := os.Stat(credentialsFile); err == nil {
			googleApplicationCredentials, err = gcp_config.LoadGoogleConfig(credentialsFile)
			if err != nil {
				startupLogger.Error(ctx, "failed to load Google config", logging.Fields{
					"error": err.Error(),
				})
				os.Exit(1)
			}

			usesImpersonation = googleApplicationCredentials.UsesImpersonation()
			if usesImpersonation {
				impersonationEmail = googleApplicationCredentials.GetImpersonationEmail()
				wifAudience = googleApplicationCredentials.Audience
				tokenFilePath = googleApplicationCredentials.CredentialSource.File
			}

			startupLogger.Info(ctx, "credentials loaded", logging.Fields{
				"uses_impersonation":  usesImpersonation,
				"impersonation_email": impersonationEmail,
				"wif_audience":        wifAudience,
			})
		} else if !os.IsNotExist(err) {
			startupLogger.Error(ctx, "error checking credentials file", logging.Fields{
				"error": err.Error(),
			})
			os.Exit(1)
		}
	}

	// Determine mode
	mode := "direct"
	if usesImpersonation {
		mode = "impersonation"
	}

	// Create HTTP mux
	mux := http.NewServeMux()

	// Set up HTTP handlers
	mux.HandleFunc("/", handleIndex(tmpl, cfg))
	mux.HandleFunc("/token", handleToken(ctx, cfg, credentialsFile, googleApplicationCredentials))
	mux.HandleFunc("/service-account", handleServiceAccount(credentialsFile, googleApplicationCredentials))

	// Health and readiness endpoints
	mux.HandleFunc("/healthz", handlers.HealthzHandler())
	mux.HandleFunc("/readyz", handlers.ReadyzHandler(handlers.ReadyzConfig{
		Template:                     tmpl,
		ConfigLoaded:                 true,
		CredentialsRequired:          !onGCE,
		CredentialsFile:              credentialsFile,
		GoogleApplicationCredentials: googleApplicationCredentials,
	}))

	// Optional debug endpoint
	if os.Getenv("ENABLE_DEBUG_ENDPOINTS") == "true" {
		startupLogger.Info(ctx, "debug endpoints enabled", nil)
		mux.HandleFunc("/debugz", handlers.DebugzHandler(handlers.DebugzConfig{
			Mode:                         mode,
			ImpersonationEmail:           impersonationEmail,
			WIFAudience:                  wifAudience,
			TokenFilePath:                tokenFilePath,
			ConfigPath:                   "config.yaml",
			ConfigExists:                 configExists,
			AllowedAudiencesCount:        len(cfg.Audiences),
			GoogleApplicationCredentials: googleApplicationCredentials,
		}))
	}

	// Apply middleware
	handler := logging.ChainMiddleware(
		logging.RequestIDMiddleware,
		logging.RequestLoggingMiddleware(logger),
	)(mux)

	// Start the server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	startupLogger.Info(ctx, "server starting", logging.Fields{
		"port": port,
		"mode": mode,
	})

	if err := http.ListenAndServe(":"+port, handler); err != nil {
		startupLogger.Error(ctx, "server failed", logging.Fields{
			"error": err.Error(),
		})
		os.Exit(1)
	}
}

// loadConfig reads the configuration from config.yaml if it exists
// Returns the config, whether the file exists, and any error
func loadConfig() (Config, bool, error) {
	var cfg Config
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		// If the file doesn't exist, return empty config
		if os.IsNotExist(err) {
			return cfg, false, nil
		}
		return cfg, false, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, true, err
	}
	return cfg, true, nil
}
