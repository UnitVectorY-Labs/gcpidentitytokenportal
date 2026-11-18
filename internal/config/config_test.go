package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGoogleConfigJSONV2(t *testing.T) {
	for _, tt := range []struct {
		name, input string
		wantError   bool
		audience    string
	}{
		{"valid", `{"audience":"expected"}`, false, "expected"},
		{"duplicate keys", `{"audience":"first","audience":"second"}`, true, ""},
		{"invalid UTF-8", "{\"audience\":\"\xff\"}", true, ""},
		{"case sensitive", `{"AUDIENCE":"unexpected"}`, false, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "credentials.json")
			if err := os.WriteFile(path, []byte(tt.input), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadGoogleConfig(path)
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v, want error %v", err, tt.wantError)
			}
			if err == nil && cfg.Audience != tt.audience {
				t.Fatalf("audience = %q, want %q", cfg.Audience, tt.audience)
			}
		})
	}
}
