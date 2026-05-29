package agent

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/llmtest"
	"github.com/amemiya02/deepseekcode/internal/prompt"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// readonlyEchoTool is a read-only tool (storm-breaker suppresses repeated
// identical read-only calls), used to prove escalation does not let a
// discarded flash turn pollute the committed turn's suppression history.
type readonlyEchoTool struct{ calls *int32 }

func (readonlyEchoTool) Name() string        { return "rdonly" }
func (readonlyEchoTool) Description() string { return "read-only echo" }
func (readonlyEchoTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"p":{"type":"string"}}}`)
}
func (readonlyEchoTool) IsReadOnly() bool { return true }
func (e readonlyEchoTool) Execute(_ context.Context, args json.RawMessage) (tools.Result, error) {
	atomic.AddInt32(e.calls, 1)
	return tools.Result{Content: "rd:" + string(args)}, nil
}

// T2.3 — model-driven escalation. The mechanism (marker detection + auto-escalate
// on repair errors + pro re-issue + policy.escalated event) adds NO wire bytes;
// only the opt-in contract (EnableEscalation) moves the fingerprint, and the
// re-issue itself changes only req.Model, so the prefix bytes are unchanged.

const proModel = config.ModelPro // "deepseek-v4-pro"

// --- pure-function unit tests (deterministic, no mock) ---

func TestNeedsProMarker(t *testing.T) {
	cases := []struct {
		text       string
		wantOK     bool
		wantReason string
	}{
		{"<<<NEEDS_PRO>>>", true, ""},
		{"<<<NEEDS_PRO: needs deeper reasoning>>>", true, "needs deeper reasoning"},
		{"  \n<<<NEEDS_PRO: x>>>\nmore", true, "x"},
		{"<<<NEEDS_PRO>>>\nthen prose", true, ""},     // prose on the NEXT line is fine
		{"I think <<<NEEDS_PRO>>>", false, ""},        // not at line start
		{"<<<NEEDS_PRO>>> and then prose", false, ""}, // trailing content on the marker line → not clean
		{"<<<NEEDS_PRO: r>>> trailing", false, ""},    // ditto, with a reason
		{"NEEDS_PRO", false, ""},
		{"<<<NEEDS_PRO without close", false, ""},
		{"", false, ""},
		{"regular answer", false, ""},
	}
	for _, c := range cases {
		reason, ok := needsProMarker(c.text)
		if ok != c.wantOK {
			t.Errorf("needsProMarker(%q) ok=%v, want %v", c.text, ok, c.wantOK)
		}
		if ok && reason != c.wantReason {
			t.Errorf("needsProMarker(%q) reason=%q, want %q", c.text, reason, c.wantReason)
		}
	}
}

func TestEscalationTrigger(t *testing.T) {
	a := &Agent{}
	// Marker wins over the repair threshold.
	if tr, _ := a.escalationTrigger("<<<NEEDS_PRO: x>>>", 99); tr != "marker" {
		t.Errorf("marker text should trigger marker, got %q", tr)
	}
	// Threshold fires at exactly escalationRepairThreshold.
	if tr, _ := a.escalationTrigger("", escalationRepairThreshold); tr != "repair_errors" {
		t.Errorf("%d repair errors should trigger repair_errors, got %q", escalationRepairThreshold, tr)
	}
	// Below threshold, no marker → no escalation.
	if tr, _ := a.escalationTrigger("", escalationRepairThreshold-1); tr != "" {
		t.Errorf("below threshold with no marker should not trigger, got %q", tr)
	}
	if tr, _ := a.escalationTrigger("normal answer", 0); tr != "" {
		t.Errorf("normal answer should not trigger, got %q", tr)
	}
}

func TestEscalationEnabledSelfSkip(t *testing.T) {
	a := &Agent{Model: "deepseek-v4-flash"}
	if a.escalationEnabled() {
		t.Error("escalation must be off when EscalationModel is empty")
	}
	a.EscalationModel = proModel
	if !a.escalationEnabled() {
		t.Error("escalation should be on when target differs from Model")
	}
	a.Model = proModel
	if a.escalationEnabled() {
		t.Error("escalation must self-skip when already on the target model")
	}
}

// --- contract / fingerprint tests ---

func TestEscalationContractIsInFingerprintInput(t *testing.T) {
	tools := []llm.Tool{}
	base := "You are a coding agent."
	fpBase := llm.StaticPrefix{System: base, Tools: tools}.Fingerprint().CombinedSHA256
	withContract := base + escalationContract(proModel)
	fpContract := llm.StaticPrefix{System: withContract, Tools: tools}.Fingerprint().CombinedSHA256
	if fpBase == fpContract {
		t.Error("adding the escalation contract must move the fingerprint (it is part of the prefix input)")
	}
	if !strings.Contains(escalationContract(proModel), proModel) {
		t.Error("the contract must name the escalation model (the only allowed interpolant)")
	}
}

func TestEnableEscalationInjectsContractBeforeBoundary(t *testing.T) {
	a := &Agent{Model: "deepseek-v4-flash"}
	a.System = "STATIC PART" + prompt.DynamicContextBoundary + "DYNAMIC PART"
	a.EnableEscalation(proModel)

	if a.EscalationModel != proModel {
		t.Fatalf("EscalationModel = %q, want %q", a.EscalationModel, proModel)
	}
	contract := escalationContract(proModel)
	ci := strings.Index(a.System, strings.TrimSpace(contract))
	bi := strings.Index(a.System, prompt.DynamicContextBoundary)
	if ci < 0 {
		t.Fatal("contract not present in system prompt after EnableEscalation")
	}
	if ci >= bi {
		t.Errorf("contract must be inserted before the dynamic boundary (contract@%d, boundary@%d)", ci, bi)
	}
	// The static prefix (what feeds the fingerprint) must now include the contract.
	if !strings.Contains(staticPrefixOf(a.System), strings.TrimSpace(contract)) {
		t.Error("contract must land in the static (fingerprinted) prefix")
	}
}

func TestEnableEscalationNoopCases(t *testing.T) {
	a := &Agent{Model: "deepseek-v4-flash", System: "sys"}
	a.EnableEscalation("") // empty → no-op
	if a.EscalationModel != "" || a.System != "sys" {
		t.Error("EnableEscalation(\"\") must be a no-op")
	}
	a.EnableEscalation("deepseek-v4-flash") // same as Model → no-op
	if a.EscalationModel != "" || a.System != "sys" {
		t.Error("EnableEscalation(sameModel) must be a no-op")
	}
}

// --- end-to-end mechanism tests (real loop against the mock) ---

// TestEscalateOnMarkerReissuesOnPro: flash emits the marker; the turn is
// re-issued on pro; only the pro answer is committed (the marker turn is
// discarded), and the second request carries the pro model.
func TestEscalateOnMarkerReissuesOnPro(t *testing.T) {
	srv := llmtest.NewServer(
		llmtest.Turn{Text: "<<<NEEDS_PRO: this needs deeper reasoning>>>"},
		llmtest.Turn{Text: "pro's considered answer"},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.EscalationModel = proModel
	esc := captureEscalations(a)

	reason, err := a.Run(context.Background(), "hard question")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}
	if srv.Count() != 2 {
		t.Fatalf("served %d requests, want 2 (flash marker + pro re-issue)", srv.Count())
	}
	if m := requestModel(t, srv.Requests()[0]); m != "deepseek-v4-flash" {
		t.Errorf("request 1 model = %q, want flash", m)
	}
	if m := requestModel(t, srv.Requests()[1]); m != proModel {
		t.Errorf("request 2 model = %q, want %q (the escalation)", m, proModel)
	}

	events := esc()
	if len(events) != 1 || events[0].Trigger != "marker" {
		t.Fatalf("want exactly one EventEscalated{marker}, got %+v", events)
	}

	// Exactly one assistant turn, holding the pro answer — NOT the marker.
	var assistants []llm.Message
	for _, m := range a.Messages {
		if m.Role == "assistant" {
			assistants = append(assistants, m)
		}
	}
	if len(assistants) != 1 {
		t.Fatalf("recorded %d assistant turns, want 1 (the discarded marker turn must not be persisted)", len(assistants))
	}
	got := assistantText(assistants[0])
	if !strings.Contains(got, "pro's considered answer") {
		t.Errorf("committed turn = %q, want the pro answer", got)
	}
	if strings.Contains(got, "NEEDS_PRO") {
		t.Error("the flash marker leaked into the committed transcript")
	}
}

// TestAutoEscalateOnRepairErrors: a flash turn whose tool calls all carry
// unrepairable args (3× KindArgsInvalid) crosses the threshold and re-issues
// on pro without any marker.
func TestAutoEscalateOnRepairErrors(t *testing.T) {
	srv := llmtest.NewServer(
		llmtest.Turn{ToolCalls: []llmtest.ToolCall{
			{ID: "c1", Name: "echo", Args: "@@@"}, // unrepairable → KindArgsInvalid
			{ID: "c2", Name: "echo", Args: "@@@"},
			{ID: "c3", Name: "echo", Args: "@@@"},
		}},
		llmtest.Turn{Text: "pro fixed it"},
	)
	defer srv.Close()

	var calls int32
	a := newMockLoopAgent(t, srv)
	a.Tools.Register(loopEchoTool{calls: &calls})
	a.EscalationModel = proModel
	esc := captureEscalations(a)

	reason, err := a.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}
	if srv.Count() != 2 {
		t.Fatalf("served %d requests, want 2 (flash + pro re-issue)", srv.Count())
	}
	if m := requestModel(t, srv.Requests()[1]); m != proModel {
		t.Errorf("re-issue model = %q, want %q", m, proModel)
	}
	events := esc()
	if len(events) != 1 || events[0].Trigger != "repair_errors" {
		t.Fatalf("want exactly one EventEscalated{repair_errors}, got %+v", events)
	}
}

// TestEscalationSelfSkipWhenAlreadyPro: when the main loop already runs on the
// target model, the marker is left as the answer (no re-issue).
func TestEscalationSelfSkipWhenAlreadyPro(t *testing.T) {
	srv := llmtest.NewServer(llmtest.Turn{Text: "<<<NEEDS_PRO>>> already deep"})
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.Model = proModel
	a.EscalationModel = proModel // same → escalation self-skips
	esc := captureEscalations(a)

	if _, err := a.Run(context.Background(), "x"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if srv.Count() != 1 {
		t.Fatalf("served %d requests, want 1 (no escalation when already on target)", srv.Count())
	}
	if len(esc()) != 0 {
		t.Error("no escalation event expected when already on the target model")
	}
}

// TestEscalationDisabledByDefault: with no EscalationModel, the marker is inert.
func TestEscalationDisabledByDefault(t *testing.T) {
	srv := llmtest.NewServer(llmtest.Turn{Text: "<<<NEEDS_PRO>>> but escalation off"})
	defer srv.Close()

	a := newMockLoopAgent(t, srv) // EscalationModel == ""
	esc := captureEscalations(a)

	if _, err := a.Run(context.Background(), "x"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if srv.Count() != 1 {
		t.Fatalf("served %d requests, want 1 (escalation off by default)", srv.Count())
	}
	if len(esc()) != 0 {
		t.Error("no escalation event expected when escalation is disabled")
	}
}

// TestEscalationChargesBothFlashAndPro pins the budget fix: the flash turn
// really streamed (and was billed), so its cost is charged in addition to the
// pro re-issue — SpentCNY ends at flash+pro, not pro alone.
func TestEscalationChargesBothFlashAndPro(t *testing.T) {
	srv := llmtest.NewServer(
		llmtest.Turn{Text: "<<<NEEDS_PRO>>>"},
		llmtest.Turn{Text: "pro answer"},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.EscalationModel = proModel

	if _, err := a.Run(context.Background(), "x"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The mock's default usage frame, charged once per model.
	u := llm.Usage{PromptTokens: 1000, CompletionTokens: 50, PromptCacheHitTokens: 900, PromptCacheMissTokens: 100}
	want := llm.Cost("deepseek-v4-flash", u) + llm.Cost(proModel, u)
	if want <= 0 {
		t.Fatal("test precondition: flash+pro cost should be > 0")
	}
	if got := a.BudgetState.SpentCNY; math.Abs(got-want) > 1e-9 {
		t.Errorf("SpentCNY = %v, want flash+pro = %v (the discarded flash turn must still be charged)", got, want)
	}
	// Sanity: charging only pro would be strictly less.
	if proOnly := llm.Cost(proModel, u); math.Abs(a.BudgetState.SpentCNY-proOnly) < 1e-9 {
		t.Error("SpentCNY equals pro-only — the flash turn's spend was dropped")
	}
}

// TestEscalationDoesNotPolluteStormHistory pins the storm-breaker fix. The flash
// turn emits 5 identical read-only calls: 2 are kept (polluting suppression
// history) and 3 are storm-suppressed, which trips the repair-error auto-escalate
// threshold. The escalated pro turn issues ONE identical read-only call; with the
// history rewound it must be KEPT and execute. Without the fix the flash turn's
// kept calls would suppress it (0 executions).
func TestEscalationDoesNotPolluteStormHistory(t *testing.T) {
	srv := llmtest.NewServer(
		llmtest.Turn{ToolCalls: []llmtest.ToolCall{
			{ID: "f1", Name: "rdonly", Args: `{"p":"a"}`},
			{ID: "f2", Name: "rdonly", Args: `{"p":"a"}`},
			{ID: "f3", Name: "rdonly", Args: `{"p":"a"}`},
			{ID: "f4", Name: "rdonly", Args: `{"p":"a"}`},
			{ID: "f5", Name: "rdonly", Args: `{"p":"a"}`}, // 2 kept + 3 suppressed → repairErrors=3
		}},
		llmtest.Turn{ToolCalls: []llmtest.ToolCall{{ID: "p1", Name: "rdonly", Args: `{"p":"a"}`}}}, // pro re-issue
		llmtest.Turn{Text: "done"}, // pro's call ran → loop continues → finish
	)
	defer srv.Close()

	var calls int32
	a := newMockLoopAgent(t, srv)
	a.Tools.Register(readonlyEchoTool{calls: &calls})
	a.EscalationModel = proModel
	esc := captureEscalations(a)

	reason, err := a.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}
	if events := esc(); len(events) != 1 || events[0].Trigger != "repair_errors" {
		t.Fatalf("want one EventEscalated{repair_errors}, got %+v", events)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("rdonly executed %d times, want 1 — the pro re-issue's call must not be suppressed by the discarded flash turn's identical calls", n)
	}
	if srv.Count() != 3 {
		t.Fatalf("served %d requests, want 3 (flash + pro-with-call + pro-done)", srv.Count())
	}
}

// --- helpers ---

func captureEscalations(a *Agent) func() []EventEscalated {
	sub := a.Bus().Subscribe(64)
	var (
		mu   sync.Mutex
		seen []EventEscalated
	)
	go func() {
		for env := range sub.C {
			if e, ok := env.Event.(EventEscalated); ok {
				mu.Lock()
				seen = append(seen, e)
				mu.Unlock()
			}
		}
	}()
	return func() []EventEscalated {
		mu.Lock()
		defer mu.Unlock()
		return append([]EventEscalated(nil), seen...)
	}
}

func requestModel(t *testing.T, body []byte) string {
	t.Helper()
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	return req.Model
}

func assistantText(m llm.Message) string {
	var out string
	for _, b := range m.Blocks {
		if tb, ok := b.(llm.TextBlock); ok {
			out += tb.Text
		}
	}
	return out
}
