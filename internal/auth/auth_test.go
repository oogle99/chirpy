package auth

import (
	"net/http"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestMakeJWTAndValidateJWT(t *testing.T) {
	userID := uuid.New()
	secret := "super-secret"

	token, err := MakeJWT(userID, secret)
	if err != nil {
		t.Fatalf("MakeJWT returned an error: %v", err)
	}

	gotUserID, err := ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("ValidateJWT returned an error: %v", err)
	}

	if gotUserID != userID {
		t.Errorf("expected user ID %v, got %v", userID, gotUserID)
	}
}

func TestValidateJWTWrongSecret(t *testing.T) {
	userID := uuid.New()

	token, err := MakeJWT(userID, "correct-secret")
	if err != nil {
		t.Fatalf("MakeJWT returned an error: %v", err)
	}

	_, err = ValidateJWT(token, "wrong-secret")
	if err == nil {
		t.Fatal("expected ValidateJWT to return an error")
	}
}

/*
	func TestValidateJWTExpired(t *testing.T) {
		userID := uuid.New()

		token, err := MakeJWT(userID, "secret", -time.Hour)
		if err != nil {
			t.Fatalf("MakeJWT returned an error: %v", err)
		}

		_, err = ValidateJWT(token, "secret")
		if err == nil {
			t.Fatal("expected ValidateJWT to return an error for an expired token")
		}
	}
*/

func TestValidateJWTMalformedToken(t *testing.T) {
	_, err := ValidateJWT("this-is-not-a-jwt", "secret")
	if err == nil {
		t.Fatal("expected ValidateJWT to return an error")
	}
}

func TestValidateJWTInvalidSubject(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject: "not-a-uuid",
	})

	tokenString, err := token.SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	_, err = ValidateJWT(tokenString, "secret")
	if err == nil {
		t.Fatal("expected ValidateJWT to return an error for invalid subject")
	}
}

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
