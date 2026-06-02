# Routing Experiment — Pre-registration (2026-06-02)

## Hypothesis
dsc's automatic flash→pro escalation is runtime-real and tested. Reasonix's
auto-escalation is advertised in its system prompt (prompt-fragments.ts:24) but
its runtime executor is unverified. This experiment TESTS whether each agent's
escalation actually fires on hard coding tasks.

## Win / tie / lose (declared in advance)
- WIN : dsc-autoroute auto-rescues hard tasks (escalates to pro, passes) AND
        reasonix-default does NOT escalate (stays flash-priced, fails them).
- TIE : reasonix-default also escalates and passes. Report honestly.
- LOSE: dsc-autoroute fails to escalate where reasonix-default does. Report honestly.

## Task inclusion rule (the flash-probe gate, Task 1.3)
Include a task ONLY if: (a) dsc flash-only FAILS it, (b) flash trips a dsc
trigger (>=3 repair errors OR emits <<<NEEDS_PRO>>>), (c) dsc pro-only PASSES it.
Exclude (and list with reason) tasks where flash already passes (no signal) or
pro also fails (no rescue possible).

## Arms
- dsc-autoroute    : -model deepseek-v4-flash --auto-route --escalation-model deepseek-v4-pro
- dsc-flash        : -model deepseek-v4-flash            (ablation: routing OFF)
- reasonix-default : reasonix run <prompt>               (shipped default)
- reasonix-pro     : reasonix run -m deepseek-v4-pro <prompt>  (forced-pro reference)

## Metric
Primary: cost-per-solved-task ($). Secondary: pass-rate. n small (3-5 tasks x
repeats) -> DIRECTIONAL only; state this in every report.

## Tasks (frozen here before running)

### Worked example (committed; format reference only)
- [x] hardbug-race-counter — synthetic worked-example, bug class: concurrency
      data race on a map. NOT a sourced public bug; exists only to make the
      fixture format (`go.mod` + `<pkg>.go` + `<pkg>_test.go` + prompt + task
      yaml) unambiguous. Reproduces under `go test -race ./...`. Do NOT count
      this toward the 3-5 real tasks below.

### TODO — HUMAN-SOURCED real public-bug fixtures (REQUIRED before the experiment runs)
Honesty gate: these MUST be documented, real bugs from public Go projects
(issue / CVE / commit), each reproduced by a failing test. They MUST be sourced
by a human and recorded with a real source URL here AND in the task yaml
`# source:` comment. They MUST NOT be invented, paraphrased from memory, or
hand-tuned beyond the inclusion rule. Until this list is filled with real URLs,
the routing experiment (Tasks 1.3 / 1.4) must NOT run.

- [ ] hardbug-<name1> — source URL: <real issue/CVE/commit URL>; bug class: <class>
- [ ] hardbug-<name2> — source URL: <real issue/CVE/commit URL>; bug class: <class>
- [ ] hardbug-<name3> — source URL: <real issue/CVE/commit URL>; bug class: <class>
- [ ] (optional) hardbug-<name4> — source URL: <real issue/CVE/commit URL>; bug class: <class>
      (2-4 total real fixtures; bug classes per the sourcing rule above:
      concurrency races, parser off-by-one, nil-deref edge, wrong error-path
      logic, multi-file cross-package refactor.)
