package auth

import (
	"net/http"
	"testing"
)

// GetBearerToken needs to find Authorization header
// and strip Bearer from the result
func TestGetBearerToken(t *testing.T) {
	headers := http.Header{}

	headers.Add("Authorization", "Bearer TOKEN_STRING")

	token, err := GetBearerToken(headers)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if token != "TOKEN_STRING" {
		t.Fatalf("expected TOKEN_STRING, got %v", token)
	}
}
