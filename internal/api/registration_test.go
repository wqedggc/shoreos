package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateRegistrationNormalizesUsername(t *testing.T) {
	username, code, _ := validateRegistration("  Alice-01  ", "correct-horse-battery")
	if code != "" || username != "alice-01" {
		t.Fatalf("validateRegistration = username %q code %q", username, code)
	}
}

func TestValidateRegistrationRejectsUnsafeInputs(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		wantCode string
	}{
		{name: "short username", username: "ab", password: "correct-horse", wantCode: "INVALID_USERNAME"},
		{name: "non ascii username", username: "用户一", password: "correct-horse", wantCode: "INVALID_USERNAME"},
		{name: "short password", username: "alice", password: "short", wantCode: "INVALID_PASSWORD"},
		{name: "bcrypt maximum", username: "alice", password: strings.Repeat("a", 73), wantCode: "INVALID_PASSWORD"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, code, _ := validateRegistration(test.username, test.password)
			if code != test.wantCode {
				t.Fatalf("code = %q, want %q", code, test.wantCode)
			}
		})
	}
}

func TestRegistrationRemoteKeyUsesSocketPeerNotForwardedHeader(t *testing.T) {
	request := httptest.NewRequest("POST", "/api/v1/auth/register", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	request.Header.Set("X-Forwarded-For", "203.0.113.10")
	if got := registrationRemoteKey(request); got != "127.0.0.1" {
		t.Fatalf("registration key = %q", got)
	}
}
