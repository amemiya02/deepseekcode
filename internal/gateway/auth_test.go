package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestAuthRejectsRemoteAddr(t *testing.T) {
	h := withAuth("secret", okHandler())
	req := httptest.NewRequest(http.MethodGet, "/v1/cache", nil)
	req.RemoteAddr = "203.0.113.7:5555" // non-loopback
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("remote addr: got %d, want 403", rr.Code)
	}
}

func TestAuthRejectsBadToken(t *testing.T) {
	h := withAuth("secret", okHandler())
	req := httptest.NewRequest(http.MethodGet, "/v1/cache", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("Authorization", "Bearer wrong")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("bad token: got %d, want 401", rr.Code)
	}
}

func TestAuthAllowsLoopbackWithToken(t *testing.T) {
	h := withAuth("secret", okHandler())
	req := httptest.NewRequest(http.MethodGet, "/v1/cache", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("good request: got %d, want 200", rr.Code)
	}
}

func TestAuthEmptyTokenDisablesBearer(t *testing.T) {
	// An empty token means "no bearer required" (dev mode); loopback still enforced.
	h := withAuth("", okHandler())
	req := httptest.NewRequest(http.MethodGet, "/v1/cache", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("empty token loopback: got %d, want 200", rr.Code)
	}
}
