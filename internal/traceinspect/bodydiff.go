package traceinspect

// BodyDiff is the result of comparing two canonical wire-request bodies —
// the bytes MarshalCacheStable produced for two consecutive turns. For
// DeepSeek prefix-cache stability, the earlier turn's body MUST be a clean
// leading prefix of the later turn's body: only the appended tail should
// differ. Any divergence INSIDE the shared region is a historical-message
// byte change, which invalidates the prefix cache from that offset and
// forces a full-body re-send — the Layer-2 eviction this instrument hunts.
type BodyDiff struct {
	LenA         int    // earlier-turn body length, bytes
	LenB         int    // later-turn body length, bytes
	DivergeAt    int    // offset of first differing byte; == min(LenA,LenB) when one is a prefix of the other
	Diverged     bool   // a byte differs within the overlap [0,min(LenA,LenB))
	AIsPrefixOfB bool   // earlier body is a clean leading prefix of the later (cache-stable)
	ContextA     string // printable window of A around DivergeAt (only when Diverged)
	ContextB     string // printable window of B around DivergeAt (only when Diverged)
}

// contextWindow is the number of bytes shown on each side of the
// divergence offset, so a human sees what changed without the whole body.
const contextWindow = 60

// DiffBytes compares two wire bodies and reports the first divergence. a is
// the earlier turn, b the later. Never panics on empty input.
func DiffBytes(a, b []byte) BodyDiff {
	d := BodyDiff{LenA: len(a), LenB: len(b)}
	overlapLen := len(a)
	if len(b) < overlapLen {
		overlapLen = len(b)
	}
	i := 0
	for i < overlapLen && a[i] == b[i] {
		i++
	}
	d.DivergeAt = i
	d.Diverged = i < overlapLen
	d.AIsPrefixOfB = !d.Diverged && len(a) <= len(b)
	if d.Diverged {
		d.ContextA = window(a, i)
		d.ContextB = window(b, i)
	}
	return d
}

// window returns up to contextWindow bytes on each side of off, with any
// non-printable byte shown as '.', so the snippet is safe to print.
func window(s []byte, off int) string {
	start := off - contextWindow
	if start < 0 {
		start = 0
	}
	end := off + contextWindow
	if end > len(s) {
		end = len(s)
	}
	out := make([]byte, 0, end-start)
	for _, c := range s[start:end] {
		if c < 0x20 || c > 0x7e {
			out = append(out, '.')
		} else {
			out = append(out, c)
		}
	}
	return string(out)
}
