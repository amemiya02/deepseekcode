// internal/doctor/checks_test.go
package doctor_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/doctor"
)

func alwaysPass(_ context.Context, _ config.Config, _ *http.Client) doctor.CheckResult {
	return doctor.CheckResult{Name: "always-pass", OK: true, Detail: "fine"}
}
func alwaysFail(_ context.Context, _ config.Config, _ *http.Client) doctor.CheckResult {
	return doctor.CheckResult{Name: "always-fail", OK: false, Detail: "broken"}
}

func TestRunChecks_AllPass(t *testing.T) {
	results := doctor.RunChecks(context.Background(), config.Config{}, nil, []doctor.Checker{alwaysPass, alwaysPass})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.OK {
			t.Fatalf("expected all pass, got failure: %+v", r)
		}
	}
}

func TestRunChecks_MixedResults(t *testing.T) {
	results := doctor.RunChecks(context.Background(), config.Config{}, nil, []doctor.Checker{alwaysPass, alwaysFail})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].OK {
		t.Fatalf("first should pass")
	}
	if results[1].OK {
		t.Fatalf("second should fail")
	}
}

func TestRun_Output(t *testing.T) {
	tests := []struct {
		name        string
		cfg         config.Config
		wantContain []string
		wantAbsent  []string
	}{
		{
			name:        "all checks produce output header",
			cfg:         config.Config{},
			wantContain: []string{"dsc doctor", "----------", "FAIL"},
		},
		{
			name:        "all fail summary line appears",
			cfg:         config.Config{},
			wantContain: []string{"Some checks failed"},
			wantAbsent:  []string{"All checks passed"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := doctor.Run(context.Background(), tc.cfg, &buf)
			if err != nil {
				t.Fatalf("Run returned error: %v", err)
			}
			out := buf.String()
			for _, want := range tc.wantContain {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q; got:\n%s", want, out)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(out, absent) {
					t.Errorf("output should not contain %q; got:\n%s", absent, out)
				}
			}
		})
	}
}

func TestCheckKeyPresent_Missing(t *testing.T) {
	cfg := config.Config{}
	cfg.Providers = map[string]config.ProviderConfigTOML{}
	cfg.Active.Provider = "deepseek"
	r := doctor.CheckKeyPresent(context.Background(), cfg, nil)
	if r.OK {
		t.Fatal("expected FAIL for missing key")
	}
}

func TestCheckKeyPresent_Present(t *testing.T) {
	cfg := config.Config{}
	cfg.Providers = map[string]config.ProviderConfigTOML{
		"deepseek": {Type: "deepseek", APIKey: "sk-present"},
	}
	cfg.Active.Provider = "deepseek"
	r := doctor.CheckKeyPresent(context.Background(), cfg, nil)
	if !r.OK {
		t.Fatalf("expected PASS, got: %s", r.Detail)
	}
}

func TestCheckKeyValid_ValidKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"content":"hi"}}]}`))
	}))
	defer srv.Close()

	cfg := config.Config{}
	cfg.API.BaseURL = srv.URL
	cfg.Providers = map[string]config.ProviderConfigTOML{
		"deepseek": {Type: "deepseek", BaseURL: srv.URL, APIKey: "sk-test"},
	}
	cfg.Active.Provider = "deepseek"
	r := doctor.CheckKeyValid(context.Background(), cfg, srv.Client())
	if !r.OK {
		t.Fatalf("expected PASS, got: %s", r.Detail)
	}
}

func TestCheckKeyValid_InvalidKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	defer srv.Close()

	cfg := config.Config{}
	cfg.API.BaseURL = srv.URL
	cfg.Providers = map[string]config.ProviderConfigTOML{
		"deepseek": {Type: "deepseek", BaseURL: srv.URL, APIKey: "sk-bad"},
	}
	cfg.Active.Provider = "deepseek"
	r := doctor.CheckKeyValid(context.Background(), cfg, srv.Client())
	if r.OK {
		t.Fatal("expected FAIL for invalid key")
	}
}
