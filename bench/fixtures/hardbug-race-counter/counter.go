package counter

// Tally counts occurrences of each key. BUG: concurrent Add calls race on the
// map; under -race the test fails. Fix must make Add safe for concurrent use
// without changing the API.
type Tally struct {
	counts map[string]int
}

func New() *Tally { return &Tally{counts: map[string]int{}} }

func (t *Tally) Add(key string) { t.counts[key]++ }

func (t *Tally) Get(key string) int { return t.counts[key] }
