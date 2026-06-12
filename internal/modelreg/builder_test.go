package modelreg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/config"
)

func TestHTTPFetcherParsesOpenAIList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"data":[{"id":"mimo-pro"},{"id":"mimo-flash"}]}`))
	}))
	defer srv.Close()

	for _, baseURL := range []string{srv.URL, srv.URL + "/v1"} {
		t.Run(baseURL, func(t *testing.T) {
			f := &httpFetcher{}
			got, err := f.Fetch(context.Background(), config.ProviderConfigTOML{BaseURL: baseURL})
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if len(got) != 2 || got[0].ID != "mimo-pro" || got[1].ID != "mimo-flash" {
				t.Fatalf("got = %v", got)
			}
		})
	}
}
