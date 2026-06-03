// export_test.go — test-only helpers for the i18n package.
// This file is compiled exclusively during `go test`; it is never included in
// production binaries.

package i18n

// RegisterTestMessages merges entries into the named locale's catalog.
// Returns a cleanup function that removes the registered keys so tests are
// isolated from one another.  Callers should defer the returned func or invoke
// it from t.Cleanup.
func RegisterTestMessages(locale string, entries map[string]string) func() {
	mu.Lock()
	defer mu.Unlock()

	if messages[locale] == nil {
		messages[locale] = make(map[string]string)
	}
	for k, v := range entries {
		messages[locale][k] = v
	}

	return func() {
		mu.Lock()
		defer mu.Unlock()
		for k := range entries {
			delete(messages[locale], k)
		}
	}
}
