package gateway

import (
	"crypto/subtle"
	"net"
	"net/http"
	"os"
	"strings"
)

// withAuth wraps next with two gates, in order:
//  1. Loopback-only: the request's remote address must be a loopback IP.
//     A non-loopback peer gets 403 regardless of token. This is defense in
//     depth on top of binding 127.0.0.1 (a future remote mode would relax it).
//  2. Bearer token: if token != "", the Authorization header must be
//     "Bearer <token>" (constant-time compared). A blank token disables the
//     bearer check (local dev) but loopback is still enforced.
func withAuth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopback(r.RemoteAddr) {
			http.Error(w, "forbidden: loopback only", http.StatusForbidden)
			return
		}
		if token != "" && !validBearer(r.Header.Get("Authorization"), token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopback reports whether remoteAddr (host:port) is a loopback address.
// A malformed address (no port) is treated as the bare host. An unparseable
// IP is rejected.
func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// validBearer constant-time compares the Authorization header against the
// expected "Bearer <token>".
func validBearer(header, token string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	got := header[len(prefix):]
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

// loadGatewayToken returns the bearer token the gateway enforces. It reads
// DSC_GATEWAY_TOKEN; an unset/empty value disables the bearer check (loopback
// still enforced). The in-process desktop shell sets this so the SPA can
// authenticate against its own gateway.
func loadGatewayToken() string { return os.Getenv("DSC_GATEWAY_TOKEN") }
