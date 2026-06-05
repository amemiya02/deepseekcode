package llm

import "testing"

func TestSelectThinkingHighEN(t *testing.T) {
	if !SelectThinking(false, "help me debug this crash", false) {
		t.Error("expected true for 'debug'")
	}
	if !SelectThinking(false, "an ERROR occurred", false) {
		t.Error("expected true for 'ERROR' (case-insensitive)")
	}
}

func TestSelectThinkingHighZH(t *testing.T) {
	if !SelectThinking(false, "帮我调试这个崩溃", false) {
		t.Error("expected true for zh '调试'")
	}
	if !SelectThinking(false, "程序报错了", false) {
		t.Error("expected true for zh '报错'")
	}
}

func TestSelectThinkingHighJA(t *testing.T) {
	if !SelectThinking(false, "デバッグしてください", false) {
		t.Error("expected true for ja 'デバッグ'")
	}
}

func TestSelectThinkingLowEN(t *testing.T) {
	if SelectThinking(false, "search for the file", true) {
		t.Error("expected false for 'search'")
	}
	if SelectThinking(false, "please lookup the docs", true) {
		t.Error("expected false for 'lookup'")
	}
}

func TestSelectThinkingLowZH(t *testing.T) {
	if SelectThinking(false, "搜索一下这个文件", true) {
		t.Error("expected false for zh '搜索'")
	}
}

func TestSelectThinkingEmpty(t *testing.T) {
	if SelectThinking(false, "", false) {
		t.Error("empty msg, defaultOn=false → false")
	}
	if !SelectThinking(false, "", true) {
		t.Error("empty msg, defaultOn=true → true")
	}
}

func TestSelectThinkingBugNotTrigger(t *testing.T) {
	// "bug" alone should NOT trigger high (only "debug"/"error" + CJK words)
	if SelectThinking(false, "find a bug", false) {
		t.Error("'bug' alone should not trigger high")
	}
}

func TestSelectThinkingHighOverridesLow(t *testing.T) {
	// Contains both high ("调试") and low ("搜索") → high wins
	if !SelectThinking(false, "调试搜索功能", false) {
		t.Error("high+low coexist → high wins → true")
	}
}

func TestSelectThinkingSubagent(t *testing.T) {
	// isSubagent short-circuits to false regardless
	if SelectThinking(true, "debug everything", true) {
		t.Error("isSubagent → false always")
	}
}

func TestSelectThinkingDefault(t *testing.T) {
	if SelectThinking(false, "write a function", false) {
		t.Error("no hit, defaultOn=false → false")
	}
	if !SelectThinking(false, "write a function", true) {
		t.Error("no hit, defaultOn=true → true")
	}
}

func TestIsTrivialMessage(t *testing.T) {
	// Short greetings/acks (no task) are trivial → must not force thinking.
	for _, m := range []string{"你好", "hi", "hey", "thanks", "谢谢", "好的", "ok", "  hi  ", ""} {
		if !IsTrivialMessage(m) {
			t.Errorf("IsTrivialMessage(%q) = false, want true", m)
		}
	}
	// Substantive messages, or short ones carrying a high-effort keyword, are not.
	for _, m := range []string{
		"implement the review panel",
		"refactor the gateway hub",
		"please fix the login flow now",
		"报错",         // short but high-effort keyword → not trivial
		"debug this", // high-effort keyword
	} {
		if IsTrivialMessage(m) {
			t.Errorf("IsTrivialMessage(%q) = true, want false", m)
		}
	}
}
