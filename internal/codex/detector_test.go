package codex

import (
	"context"
	"errors"
	"testing"
)

type fakeRunner struct {
	path   string
	output string
	err    error
}

func (f fakeRunner) LookPath(string) (string, error) {
	if f.path == "" {
		return "", errors.New("missing")
	}
	return f.path, nil
}

func (f fakeRunner) Output(context.Context, string, ...string) ([]byte, error) {
	return []byte(f.output), f.err
}

func TestDetectorCompatibility(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		runner        fakeRunner
		wantCode      ErrorCode
		wantSupported bool
		wantNewer     bool
	}{
		{name: "missing", runner: fakeRunner{}, wantCode: ErrCodexNotFound},
		{name: "older", runner: fakeRunner{path: "/bin/codex", output: "codex-cli 0.144.0"}, wantCode: ErrCodexUnsupportedVersion},
		{name: "baseline", runner: fakeRunner{path: "/bin/codex", output: "codex-cli 0.145.0"}, wantSupported: true},
		{name: "newer", runner: fakeRunner{path: "/bin/codex", output: "codex-cli 0.146.0"}, wantSupported: true, wantNewer: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			detector := &Detector{runner: test.runner}
			got, err := detector.Detect(context.Background())
			if got.Supported != test.wantSupported || got.NewerThanTested != test.wantNewer {
				t.Fatalf("Detect() = %+v", got)
			}
			if test.wantCode == "" {
				if err != nil {
					t.Fatalf("Detect() unexpected error = %v", err)
				}
				return
			}
			appErr, ok := err.(*AppError)
			if !ok || appErr.Code != test.wantCode {
				t.Fatalf("Detect() error = %#v, want code %s", err, test.wantCode)
			}
		})
	}
}
