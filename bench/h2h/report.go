package h2h

import (
	"fmt"
	"strings"
)

// RenderReport renders the aggregate markdown report including the
// spec §4 win-gate verdict (hit-rate higher AND billable <= AND
// resolve tied).
func RenderReport(rr RunResult) string {
	type agg struct {
		hit, miss, out, resolved, dnf, n int
	}
	arms := map[string]*agg{}
	for _, r := range rr.Results {
		a, ok := arms[r.Arm]
		if !ok {
			a = &agg{}
			arms[r.Arm] = a
		}
		a.n++
		for _, t := range r.Turns {
			a.hit += t.HitTokens
			a.miss += t.MissTokens
			a.out += t.OutTokens
		}
		if r.Resolved {
			a.resolved++
		}
		if r.DNF {
			a.dnf++
		}
	}
	rate := func(a *agg) float64 {
		if a.hit+a.miss == 0 {
			return 0
		}
		return 100 * float64(a.hit) / float64(a.hit+a.miss)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# h2h cache benchmark — %s (%s)\n\n", rr.Date, rr.Model)
	fmt.Fprintf(&b, "dsc commit `%s` · reasonix sha256 `%s`\n\n", rr.DscCommit, rr.ReasonixSHA256)
	b.WriteString("| arm | runs | resolved | DNF | hit rate | billable (miss+out) |\n|---|---|---|---|---|---|\n")
	for _, arm := range []string{"dsc", "reasonix"} {
		a := arms[arm]
		if a == nil {
			continue
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %.1f%% | %d |\n", arm, a.n, a.resolved, a.dnf, rate(a), a.miss+a.out)
	}
	d, r := arms["dsc"], arms["reasonix"]
	if d != nil && r != nil {
		win := rate(d) > rate(r) && d.miss+d.out <= r.miss+r.out && d.resolved == r.resolved
		fmt.Fprintf(&b, "\n**WIN GATE: %v** (hit %.1f%% vs %.1f%%, billable %d vs %d, resolved %d vs %d)\n",
			win, rate(d), rate(r), d.miss+d.out, r.miss+r.out, d.resolved, r.resolved)
	}
	// Per-run detail — failures must be visible, never averaged away (§3.3(5)).
	b.WriteString("\n## Per-run detail\n\n| arm | task | repeat | resolved | DNF | hit rate | billable | err |\n|---|---|---|---|---|---|---|---|\n")
	for _, x := range rr.Results {
		fmt.Fprintf(&b, "| %s | %s | %d | %v | %v | %.1f%% | %d | %s |\n",
			x.Arm, x.TaskID, x.Repeat, x.Resolved, x.DNF, 100*x.HitRate(), x.Billable(), x.Err)
	}
	return b.String()
}
