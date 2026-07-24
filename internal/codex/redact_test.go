package codex

import (
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	t.Parallel()
	input := "Authorization: Bearer top-secret sk-abcdefghijklm eyJabcdefghijk.abcdefghijk.abcdefghijk"
	got := redactSecrets(input)
	for _, secret := range []string{"top-secret", "sk-abcdefghijklm", "eyJabcdefghijk"} {
		if strings.Contains(got, secret) {
			t.Fatalf("redactSecrets() leaked %q in %q", secret, got)
		}
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("redactSecrets() = %q, want redaction marker", got)
	}
}
