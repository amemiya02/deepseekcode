// internal/agent/warm_test.go
package agent

import (
	"testing"
	"time"
)

func TestIsLikelyWarm(t *testing.T) {
	fp := "abc123"
	if !IsLikelyWarm(fp, fp, 30*time.Minute, time.Hour) {
		t.Fatal("same fp within ttl should be warm")
	}
	if IsLikelyWarm("old", fp, 1*time.Minute, time.Hour) {
		t.Fatal("changed fp should be cold")
	}
	if IsLikelyWarm(fp, fp, 5*time.Hour, time.Hour) {
		t.Fatal("beyond ttl should be cold")
	}
}
