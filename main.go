package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
	"gopkg.in/yaml.v2"
)

// Config holds the application configuration
type Config struct {
	Audiences []string `yaml:"audiences"`
}

func handleIndex(tmpl *template.Template, cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		err := tmpl.Execute(w, cfg)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			log.Printf("Template execution error: %v", err)
		}
	}
}

func handleToken(ctx context.Context, cfg Config, credentialsFile string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := r.ParseForm(); err != nil {
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
				http.Error(w, "Invalid audience selected", http.StatusBadRequest)
				return
			}
		}

		var ts oauth2.TokenSource
		var err error
		if credentialsFile != "" {
			ts, err = idtoken.NewTokenSource(ctx, audience, idtoken.WithCredentialsFile(credentialsFile))
		} else {
			ts, err = idtoken.NewTokenSource(ctx, audience)
		}

		if err != nil {
			log.Printf("Failed to create token source: %v", err)
			http.Error(w, "Failed to create token source", http.StatusInternalServerError)
			return
		}

		token, err := ts.Token()
		if err != nil {
			log.Printf("Failed to get token: %v", err)
			http.Error(w, "Failed to get token", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(token.AccessToken))
	}
}

func handleServiceAccount(credentialsFile string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var email string
		var err error

		if metadata.OnGCE() {
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

	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Parse HTML template
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		log.Fatalf("Failed to parse template: %v", err)
	}

	// Load credentials directly
	credentialsFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if credentialsFile == "" && !metadata.OnGCE() {
		log.Fatal("No credentials provided. Set GOOGLE_APPLICATION_CREDENTIALS or run on GCP.")
	}

	// Set up HTTP handlers
	http.HandleFunc("/", handleIndex(tmpl, cfg))
	http.HandleFunc("/token", handleToken(ctx, cfg, credentialsFile))
	http.HandleFunc("/service-account", handleServiceAccount(credentialsFile))

	// Start the server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server is running on port %s...", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// loadConfig reads the configuration from config.yaml if it exists
func loadConfig() (Config, error) {
	var cfg Config
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		// If the file doesn't exist, return empty config
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
