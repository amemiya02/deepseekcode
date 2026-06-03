// Package i18n provides a lightweight message catalog for deepseekcode.
// Locale is resolved once at init time (or on ReloadLocale) from:
//  1. DEEPSEEKCODE_LANG  (e.g. "zh-CN")
//  2. LANGUAGE           (colon-separated, first wins)
//  3. LANG               (strips encoding suffix, e.g. "zh_CN.UTF-8" → "zh-CN")
//
// T(key, args...) returns the translated string for the active locale,
// falling back to English. If args are provided they are interpolated via
// fmt.Sprintf. Unknown keys return the key itself so nothing silently
// disappears.
package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// messages holds the full catalog keyed by locale tag then message key.
// Only "en" (English) and "zh-CN" (Simplified Chinese) are required for
// this milestone; add more locales by inserting a new inner map.
var messages = map[string]map[string]string{
	"en": {
		// Welcome banner (internal/tui/welcome.go)
		"welcome.narrow":      "⏺ deepseekcode",
		"welcome.narrow.hint": "%s · /help for commands",
		"welcome.tagline":     "Terminal coding agent for DeepSeek",
		"welcome.hint.send":   "⏎ send · ⇧⏎ newline · /help · ^D quit",

		// Status HUD (internal/tui/status_hud.go)
		"status.thinking":   "thinking…",
		"status.streaming":  "streaming",
		"status.idle":       "ready",
		"status.error":      "error",
		"status.compacting": "compacting context…",

		// Permission prompts (internal/tui/permission.go)
		"permission.prompt":  "Allow tool %s on %s? [y/N/always]",
		"permission.granted": "Permission granted.",
		"permission.denied":  "Permission denied.",
		"permission.always":  "Always allowed for this session.",

		// Question / confirm prompts (internal/tui/question.go)
		"question.confirm":    "Continue? [y/N]",
		"question.input.hint": "Type your answer and press ⏎",

		// App-level hints (internal/tui/app.go)
		"app.help.hint": "/help for commands",

		// Sentinel key present only in English — used by TestT_Fallback_MissingKey
		// to verify that T() falls back to English when a key is absent from the
		// active locale.  Never add this key to any other locale map.
		"_test.en_only_sentinel": "english-only-sentinel",
	},

	"zh-CN": {
		// Welcome banner
		"welcome.narrow":      "⏺ deepseekcode",
		"welcome.narrow.hint": "%s · 输入 /help 查看命令",
		"welcome.tagline":     "DeepSeek 终端编程代理",
		"welcome.hint.send":   "⏎ 发送 · ⇧⏎ 换行 · /help · ^D 退出",

		// Status HUD
		"status.thinking":   "思考中…",
		"status.streaming":  "输出中",
		"status.idle":       "就绪",
		"status.error":      "错误",
		"status.compacting": "压缩上下文…",

		// Permission prompts
		"permission.prompt":  "允许工具 %s 操作 %s？[y/N/always]",
		"permission.granted": "已授权。",
		"permission.denied":  "已拒绝。",
		"permission.always":  "本次会话始终允许。",

		// Question / confirm prompts
		"question.confirm":    "继续？[y/N]",
		"question.input.hint": "输入答案并按 ⏎",

		// App-level hints
		"app.help.hint": "输入 /help 查看命令",
	},
}

var (
	mu     sync.RWMutex
	locale string // resolved locale tag, e.g. "zh-CN" or "en"
)

func init() { ReloadLocale() }

// ReloadLocale re-reads the environment and updates the active locale.
// It is exported for tests that manipulate env vars between sub-tests.
func ReloadLocale() {
	mu.Lock()
	defer mu.Unlock()
	locale = resolveLocale()
}

// resolveLocale applies the priority chain and normalises the tag.
func resolveLocale() string {
	// 1. Explicit override.
	if v := os.Getenv("DEEPSEEKCODE_LANG"); v != "" {
		return normalise(v)
	}
	// 2. LANGUAGE (colon-separated list, first wins).
	if v := os.Getenv("LANGUAGE"); v != "" {
		parts := strings.SplitN(v, ":", 2)
		if tag := normalise(parts[0]); tag != "" {
			return tag
		}
	}
	// 3. LANG (strip encoding suffix).
	if v := os.Getenv("LANG"); v != "" {
		// "zh_CN.UTF-8" → "zh-CN"
		tag := normalise(strings.SplitN(v, ".", 2)[0])
		if tag != "" {
			return tag
		}
	}
	return "en"
}

// normalise converts POSIX-style locale tags ("zh_CN") to BCP-47 ("zh-CN")
// and returns "" for "C", "POSIX", or empty values.
func normalise(tag string) string {
	tag = strings.TrimSpace(tag)
	switch tag {
	case "", "C", "POSIX":
		return ""
	}
	// Replace underscore with hyphen: zh_CN → zh-CN.
	return strings.ReplaceAll(tag, "_", "-")
}

// T returns the translated string for key in the active locale.
// If args are provided they are interpolated via fmt.Sprintf.
// Falls back to English if the locale has no translation for key.
// Returns the key itself if neither locale has it (prevents silent data loss).
func T(key string, args ...any) string {
	mu.RLock()
	loc := locale
	mu.RUnlock()

	msg := lookup(loc, key)
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}

// lookup returns the message for key, falling back to "en", then to key.
func lookup(loc, key string) string {
	if cat, ok := messages[loc]; ok {
		if msg, ok := cat[key]; ok {
			return msg
		}
	}
	// English fallback.
	if cat, ok := messages["en"]; ok {
		if msg, ok := cat[key]; ok {
			return msg
		}
	}
	// Last resort: return the key so the caller always gets a non-empty string.
	return key
}
