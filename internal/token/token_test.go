package token

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/UnitVectorY-Labs/gcpidentitytokenportal/internal/config"
)

func TestIAMResponseJSONV2(t *testing.T) {
	for _, tt := range []struct {
		name, body, want string
		wantError        bool
	}{
		{"valid", `{"token":"identity"}`, "identity", false},
		{"duplicate", `{"token":"first","token":"second"}`, "", true},
		{"trailing value", `{"token":"identity"} {}`, "", true},
		{"case sensitive", `{"TOKEN":"identity"}`, "", true},
		{"invalid UTF-8", "{\"token\":\"\xff\"}", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(tt.body)) }))
			defer server.Close()
			cfg := &config.GoogleApplicationCredentials{ServiceAccountImpersonationURL: server.URL}
			got, err := generateIdentityToken(context.Background(), cfg, "access", "audience")
			if (err != nil) != tt.wantError || got != tt.want {
				t.Fatalf("token = %q, error = %v; want %q, error %v", got, err, tt.want, tt.wantError)
			}
		})
	}
}
