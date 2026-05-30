package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeepSeekAPIRoot(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://api.deepseek.com", "https://api.deepseek.com"},
		{"https://api.deepseek.com/", "https://api.deepseek.com"},
		{"https://api.deepseek.com/v1", "https://api.deepseek.com"},
		{"https://api.deepseek.com/v1/", "https://api.deepseek.com"},
		{"https://example.test/v1", "https://example.test"},
	}
	for _, tc := range cases {
		got := deepSeekAPIRoot(tc.input)
		if got != tc.want {
			t.Errorf("deepSeekAPIRoot(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestGetUserBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/balance" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("unexpected auth: %s", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(UserBalance{
			IsAvailable: true,
			BalanceInfos: []BalanceInfo{
				{Currency: "CNY", TotalBalance: "110.00", GrantedBalance: "10.00", ToppedUpBalance: "100.00"},
			},
		})
	}))
	defer srv.Close()

	c := NewClient("sk-test", srv.URL)
	ub, err := c.GetUserBalance(context.Background())
	if err != nil {
		t.Fatalf("GetUserBalance: %v", err)
	}
	if !ub.IsAvailable {
		t.Error("expected IsAvailable=true")
	}
	if len(ub.BalanceInfos) != 1 {
		t.Fatalf("got %d balance infos, want 1", len(ub.BalanceInfos))
	}
	bi := ub.BalanceInfos[0]
	if bi.Currency != "CNY" || bi.TotalBalance != "110.00" {
		t.Errorf("unexpected balance info: %+v", bi)
	}
}

func TestGetUserBalancePath(t *testing.T) {
	// Prove that /v1 base URL calls /user/balance, not /v1/user/balance.
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(UserBalance{IsAvailable: true})
	}))
	defer srv.Close()

	c := NewClient("sk-test", srv.URL+"/v1")
	_, err := c.GetUserBalance(context.Background())
	if err != nil {
		t.Fatalf("GetUserBalance: %v", err)
	}
	if gotPath != "/user/balance" {
		t.Errorf("path = %q, want /user/balance", gotPath)
	}
}

func TestGetUserBalanceNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer srv.Close()

	c := NewClient("sk-test", srv.URL)
	_, err := c.GetUserBalance(context.Background())
	if err == nil {
		t.Fatal("expected error for 401")
	}
	var apiErr *APIError
	if !isAPIError(err, &apiErr) {
		t.Fatalf("want *APIError, got %T: %v", err, err)
	}
	if apiErr.Status != 401 {
		t.Errorf("status = %d, want 401", apiErr.Status)
	}
}

func isAPIError(err error, target **APIError) bool {
	if e, ok := err.(*APIError); ok {
		*target = e
		return true
	}
	return false
}
