package codex

import "testing"

func TestParseVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"codex-cli 0.145.0", "0.145.0"},
		{"codex 1.2.3-beta.1", "1.2.3"},
		{"0.145.0", "0.145.0"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			got, err := parseVersion(test.input)
			if err != nil {
				t.Fatalf("parseVersion() error = %v", err)
			}
			if got.raw != test.want {
				t.Fatalf("parseVersion() = %q, want %q", got.raw, test.want)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	t.Parallel()
	old, _ := parseVersion("0.144.9")
	baseline, _ := parseVersion(MinimumVersion)
	newer, _ := parseVersion("0.146.0")
	if compareVersions(old, baseline) >= 0 {
		t.Fatal("older version was not ordered before baseline")
	}
	if compareVersions(baseline, baseline) != 0 {
		t.Fatal("baseline did not compare equal")
	}
	if compareVersions(newer, baseline) <= 0 {
		t.Fatal("newer version was not ordered after baseline")
	}
}
