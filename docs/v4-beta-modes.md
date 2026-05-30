# DeepSeek V4 beta capability modes

**Goal:** Document Chat Prefix Completion and FIM Completion as known DeepSeek V4 capabilities that are intentionally not implemented in this tranche.

**Files involved:**
- Modify: `docs/MODEL_COMPATIBILITY.md`
- Modify: `docs/PROVIDERS.md`
- Modify: `docs/PARITY.md`
- Read-only reference: `internal/llm/provider_deepseek.go`

**Interface contract:**
```markdown
## Deferred V4 beta modes

- Chat Prefix Completion ...
- FIM Completion ...
- Current runtime status ...
- Future implementation notes ...
```

**Implementation steps:**
1. Add a `Deferred V4 beta modes` section to `docs/MODEL_COMPATIBILITY.md`.
2. State that Chat Prefix Completion requires the DeepSeek beta base URL and a final assistant message with `prefix: true`.
3. State that FIM Completion uses the DeepSeek beta completions endpoint and is intended for code completion/fill-in-middle flows.
4. State that provider capabilities may list these modes, but the agent loop does not route normal ReAct turns through them yet.
5. Add a short provider note to `docs/PROVIDERS.md`.
6. Add a parity note to `docs/PARITY.md` saying no runtime golden exists yet because implementation is deferred.
7. Include links to the official DeepSeek Chat Prefix and FIM docs.

**Edge cases to handle:**
- Avoid implying the beta modes are available through normal `dsc -p` or TUI chat.
- Avoid promising a delivery date.
- Keep README unchanged in this task because this is contributor-facing, not user-facing runtime functionality.

**Out of scope (do not do):**
- Do not implement Chat Prefix Completion.
- Do not implement FIM Completion.
- Do not add beta base URL config.
- Do not change provider capabilities.

**Acceptance criteria (each must be objectively checkable):**
- [ ] `docs/MODEL_COMPATIBILITY.md` contains `Deferred V4 beta modes`.
- [ ] `docs/MODEL_COMPATIBILITY.md` links to both official DeepSeek beta docs.
- [ ] `docs/PROVIDERS.md` states the beta modes are capability metadata, not current runtime routing.
- [ ] `docs/PARITY.md` states runtime parity tests are deferred until implementation.
- [ ] `go test ./...` exits 0, or the implementation report records any pre-existing unrelated failure.

**Example I/O:**
```markdown
Provider capability: ChatPrefixCompletion=true
Runtime status: documented capability only; normal ReAct turns still use /v1/chat/completions.
```


