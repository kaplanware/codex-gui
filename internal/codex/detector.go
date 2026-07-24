package codex

import (
	"context"
	"os/exec"
	"strings"
)

type commandRunner interface {
	LookPath(file string) (string, error)
	Output(ctx context.Context, path string, args ...string) ([]byte, error)
}

type systemRunner struct{}

func (systemRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (systemRunner) Output(ctx context.Context, path string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, path, args...).CombinedOutput()
}

type Detector struct {
	runner commandRunner
}

func NewDetector() *Detector {
	return &Detector{runner: systemRunner{}}
}

func (d *Detector) Detect(ctx context.Context) (CodexInstallation, error) {
	result := CodexInstallation{MinimumVersion: MinimumVersion}
	path, err := d.runner.LookPath("codex")
	if err != nil {
		return result, &AppError{
			Code: ErrCodexNotFound, Summary: "Codex CLI was not found",
			Detail: "Install Codex or add it to PATH, then retry.", Retryable: true,
		}
	}
	result.Found = true
	result.Path = path

	output, err := d.runner.Output(ctx, path, "--version")
	result.RawVersion = strings.TrimSpace(string(output))
	if err != nil {
		return result, &AppError{
			Code: ErrCodexStartFailed, Summary: "Could not read the Codex version",
			Detail: result.RawVersion, Retryable: true,
		}
	}

	current, err := parseVersion(result.RawVersion)
	if err != nil {
		return result, &AppError{
			Code: ErrCodexUnsupportedVersion, Summary: "Codex returned an unrecognized version",
			Detail: err.Error(), Retryable: true,
		}
	}
	minimum, _ := parseVersion(MinimumVersion)
	result.Version = current.raw
	comparison := compareVersions(current, minimum)
	result.Supported = comparison >= 0
	result.NewerThanTested = comparison > 0
	if !result.Supported {
		return result, &AppError{
			Code: ErrCodexUnsupportedVersion, Summary: "Codex CLI is too old",
			Detail: "Version " + MinimumVersion + " or newer is required.", Retryable: true,
		}
	}
	return result, nil
}
