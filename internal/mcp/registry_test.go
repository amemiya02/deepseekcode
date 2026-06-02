package mcp

import "testing"

func TestTransportForStreamableHTTP(t *testing.T) {
	tr, err := transportFor("streamable-http", "http://localhost:9/mcp", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tr.(*StreamableHTTPTransport); !ok {
		t.Fatalf("want *StreamableHTTPTransport, got %T", tr)
	}
	_ = tr.Close()
}
