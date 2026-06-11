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

	f := &httpFetcher{}
	ids, err := f.Fetch(context.Background(), config.ProviderConfigTOML{BaseURL: srv.URL + "/v1"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(ids) != 2 || ids[0] != "mimo-pro" || ids[1] != "mimo-flash" {
		t.Fatalf("ids = %v", ids)
	}
}
