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
- [ ] hardbug-<name1> — source URL, bug class
- [ ] ... (3-5 total)
