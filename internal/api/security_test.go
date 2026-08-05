package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeadersProtectAPIsFromCachingAndEmbedding(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ledger/human-transactions", nil))

	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("api cache control = %q, want no-store", got)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("nosniff header = %q", got)
	}
	if got := response.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("frame protection header = %q", got)
	}
}

func TestLocalBootstrapRejectsForwardedRequests(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	if !isLocalBootstrapRequest(request) {
		t.Fatal("direct loopback bootstrap should be allowed")
	}

	request.Header.Set("X-Forwarded-For", "203.0.113.10")
	if isLocalBootstrapRequest(request) {
		t.Fatal("proxied bootstrap must not be treated as a direct loopback request")
	}
}
