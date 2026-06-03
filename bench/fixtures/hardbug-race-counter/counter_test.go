package counter

import (
	"sync"
	"testing"
)

func TestConcurrentAddIsRaceFree(t *testing.T) {
	tally := New()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); tally.Add("k") }()
	}
	wg.Wait()
	if got := tally.Get("k"); got != 100 {
		t.Fatalf("Get(k) = %d, want 100", got)
	}
}
