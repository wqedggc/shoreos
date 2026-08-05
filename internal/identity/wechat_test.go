package identityauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWechatExchangePassesCodeAndReturnsIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("js_code") != "temporary-code" {
			t.Fatalf("unexpected code: %s", r.URL.Query().Get("js_code"))
		}
		_, _ = w.Write([]byte(`{"openid":"openid-1","unionid":"union-1"}`))
	}))
	defer server.Close()

	session, err := (WechatExchanger{AppID: "app", Secret: "secret", Endpoint: server.URL}).Exchange(context.Background(), "temporary-code")
	if err != nil {
		t.Fatal(err)
	}
	if session.OpenID != "openid-1" || session.UnionID != "union-1" {
		t.Fatalf("unexpected session: %#v", session)
	}
}

func TestWechatExchangeRequiresConfiguration(t *testing.T) {
	_, err := (WechatExchanger{}).Exchange(context.Background(), "code")
	if err != ErrWechatNotConfigured {
		t.Fatalf("expected config error, got %v", err)
	}
}
