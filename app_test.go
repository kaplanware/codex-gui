package main

import "testing"

func TestValidateLoginURL(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"https://chatgpt.com/auth",
		"https://auth.openai.com/codex/device",
	} {
		if err := validateLoginURL(value); err != nil {
			t.Fatalf("validateLoginURL(%q) error = %v", value, err)
		}
	}
	for _, value := range []string{
		"javascript:alert(1)",
		"file:///tmp/login",
		"https://chatgpt.com.example.org/login",
	} {
		if err := validateLoginURL(value); err == nil {
			t.Fatalf("validateLoginURL(%q) accepted unsafe URL", value)
		}
	}
}
