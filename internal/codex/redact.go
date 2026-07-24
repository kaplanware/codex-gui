package codex

import "regexp"

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)[^\s]+`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{10,}\b`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`),
}

func redactSecrets(value string) string {
	redacted := value
	for index, pattern := range secretPatterns {
		if index == 0 {
			redacted = pattern.ReplaceAllString(redacted, "${1}[REDACTED]")
			continue
		}
		redacted = pattern.ReplaceAllString(redacted, "[REDACTED]")
	}
	return redacted
}
