package gateway_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
	"github.com/amemiya02/deepseekcode/internal/gateway"
)

func TestSessionsCRUD(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactory)
	h := gateway.NewHandler(sm, "")
	ts := httptest.NewServer(h)
	defer ts.Close()

	// POST /v1/sessions creates one and returns its id.
	cr, err := http.Post(ts.URL+"/v1/sessions", "application/json", strings.NewReader(`{"title":"first"}`))
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	json.NewDecoder(cr.Body).Decode(&created)
	cr.Body.Close()
	if created.ID == "" {
		t.Fatal("expected created session id")
	}
	if created.Title != "first" {
		t.Fatalf("title = %q, want first", created.Title)
	}

	// GET /v1/sessions lists it.
	lr, _ := http.Get(ts.URL + "/v1/sessions")
	var list struct {
		Sessions []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"sessions"`
	}
	json.NewDecoder(lr.Body).Decode(&list)
	lr.Body.Close()
	if len(list.Sessions) != 1 || list.Sessions[0].ID != created.ID {
		t.Fatalf("list = %+v, want one session %s", list.Sessions, created.ID)
	}

	// PATCH renames it.
	patchReq, _ := http.NewRequest(http.MethodPatch, ts.URL+"/v1/sessions/"+created.ID,
		strings.NewReader(`{"title":"renamed"}`))
	pr, _ := http.DefaultClient.Do(patchReq)
	if pr.StatusCode != http.StatusOK {
		t.Fatalf("PATCH: got %d", pr.StatusCode)
	}
	pr.Body.Close()

	// GET /v1/sessions/{id} returns the renamed session with a messages array.
	gr, _ := http.Get(ts.URL + "/v1/sessions/" + created.ID)
	var got struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Messages []any  `json:"messages"`
	}
	json.NewDecoder(gr.Body).Decode(&got)
	gr.Body.Close()
	if got.Title != "renamed" {
		t.Fatalf("title after patch = %q, want renamed", got.Title)
	}
	if got.Messages == nil {
		t.Fatal("expected a (possibly empty) messages array, got null")
	}

	// DELETE removes it.
	delReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/sessions/"+created.ID, nil)
	dr, _ := http.DefaultClient.Do(delReq)
	if dr.StatusCode != http.StatusOK {
		t.Fatalf("DELETE: got %d", dr.StatusCode)
	}
	dr.Body.Close()

	lr2, _ := http.Get(ts.URL + "/v1/sessions")
	var list2 struct {
		Sessions []struct{ ID string `json:"id"` } `json:"sessions"`
	}
	json.NewDecoder(lr2.Body).Decode(&list2)
	lr2.Body.Close()
	if len(list2.Sessions) != 0 {
		t.Fatalf("after delete, list = %+v, want empty", list2.Sessions)
	}
}

func TestGetUnknownSession404(t *testing.T) {
	sm := acp.NewSessionManager(stubAgentFactory)
	h := gateway.NewHandler(sm, "")
	ts := httptest.NewServer(h)
	defer ts.Close()
	r, _ := http.Get(ts.URL + "/v1/sessions/missing")
	if r.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown session: got %d, want 404", r.StatusCode)
	}
	r.Body.Close()
}
