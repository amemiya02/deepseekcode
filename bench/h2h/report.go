package h2h

import (
	"fmt"
	"sort"
	"strings"
)

// median returns the median of a float64 slice; 0 for empty input.
func median(vals []float64) float64 {
	n := len(vals)
	if n == 0 {
		return 0
	}
	sorted := make([]float64, n)
	copy(sorted, vals)
	sort.Float64s(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// RenderReport renders the aggregate markdown report including the
// spec §4 win-gate verdict (hit-rate higher AND billable <= AND
// resolved >=).
func RenderReport(rr RunResult) string {
	type agg struct {
		hit, miss, out, resolved, dnf, n int
	}
	arms := map[string]*agg{}
	resolvedArms := map[string]*agg{}
	perRunRates := map[string][]float64{}
	resolvedRates := map[string][]float64{}
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

		// Per-run hit rate tracking.
		perRunRates[r.Arm] = append(perRunRates[r.Arm], 100*r.HitRate())

		// Resolved-only aggregation.
		if r.Resolved {
			ra, ok := resolvedArms[r.Arm]
			if !ok {
				ra = &agg{}
				resolvedArms[r.Arm] = ra
			}
			ra.n++
			for _, t := range r.Turns {
				ra.hit += t.HitTokens
				ra.miss += t.MissTokens
				ra.out += t.OutTokens
			}
			ra.resolved++
			resolvedRates[r.Arm] = append(resolvedRates[r.Arm], 100*r.HitRate())
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
	b.WriteString("| arm | runs | resolved | DNF | hit rate | billable (miss+out) | median hit rate |\n|---|---|---|---|---|---|---|\n")
	for _, arm := range []string{"dsc", "reasonix"} {
		a := arms[arm]
		if a == nil {
			continue
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %.1f%% | %d | %.1f%% |\n", arm, a.n, a.resolved, a.dnf, rate(a), a.miss+a.out, median(perRunRates[arm]))
	}
	// Win gate evaluated on the resolved-only aggregate (spec §5.1(4)+§7.1):
	// DNF spins must not inflate the gate's hit-rate or billable inputs.
	d, r := resolvedArms["dsc"], resolvedArms["reasonix"]
	if d != nil && r != nil {
		win := rate(d) > rate(r) && d.miss+d.out <= r.miss+r.out && d.resolved >= r.resolved
		fmt.Fprintf(&b, "\n**WIN GATE: %v** (resolved-only: hit %.1f%% vs %.1f%%, billable %d vs %d, resolved %d vs %d)\n",
			win, rate(d), rate(r), d.miss+d.out, r.miss+r.out, d.resolved, r.resolved)
	} else {
		b.WriteString("\n**WIN GATE: insufficient data** (one or both arms have no resolved runs)\n")
	}

	// Resolved-only aggregate section.
	b.WriteString("\n## Resolved-only aggregate\n\n")
	b.WriteString("| arm | runs | hit rate | billable | median hit rate |\n|---|---|---|---|---|\n")
	for _, arm := range []string{"dsc", "reasonix"} {
		ra := resolvedArms[arm]
		if ra == nil {
			continue
		}
		fmt.Fprintf(&b, "| %s | %d | %.1f%% | %d | %.1f%% |\n", arm, ra.n, rate(ra), ra.miss+ra.out, median(resolvedRates[arm]))
	}
	if len(resolvedArms) == 0 {
		b.WriteString("| (no resolved runs) | | | | |\n")
	}

	b.WriteString("\nDNF runs' tokens are included in the aggregate hit-rate/billable above (§3.3(5): failures stay visible); see per-run detail for which runs were DNF.\n")
	// Per-run detail — failures must be visible, never averaged away (§3.3(5)).
	b.WriteString("\n## Per-run detail\n\n| arm | task | repeat | turns | resolved | DNF | hit rate | billable | err |\n|---|---|---|---|---|---|---|---|---|\n")
	for _, x := range rr.Results {
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %v | %v | %.1f%% | %d | %s |\n",
			x.Arm, x.TaskID, x.Repeat, len(x.Turns), x.Resolved, x.DNF, 100*x.HitRate(), x.Billable(), x.Err)
	}

	// Per-run token attribution breakdown (W0.4).
	hasAttribution := false
	for _, x := range rr.Results {
		for _, t := range x.Turns {
			if t.Attribution != nil {
				hasAttribution = true
				break
			}
		}
		if hasAttribution {
			break
		}
	}
	if hasAttribution {
		b.WriteString("\n## Token attribution\n\n| arm | task | repeat | tool_result | assistant_text | reasoning | system |\n|---|---|---|---|---|---|---|\n")
		for _, x := range rr.Results {
			// Aggregate attribution across turns for this run.
			var ta TokenAttribution
			found := false
			for _, t := range x.Turns {
				if t.Attribution != nil {
					found = true
					ta.ToolResult += t.Attribution.ToolResult
					ta.AssistantText += t.Attribution.AssistantText
					ta.Reasoning += t.Attribution.Reasoning
					ta.System += t.Attribution.System
				}
			}
			if !found {
				continue
			}
			fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %d | %d |\n",
				x.Arm, x.TaskID, x.Repeat, ta.ToolResult, ta.AssistantText, ta.Reasoning, ta.System)
		}
	}

	return b.String()
}
