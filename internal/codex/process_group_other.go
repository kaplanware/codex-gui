//go:build !linux

package codex

import (
	"os"
	"os/exec"
)

func configureProcessGroup(_ *exec.Cmd) {}

func terminateProcessGroup(cmd *exec.Cmd) error {
	return cmd.Process.Signal(os.Interrupt)
}

func killProcessGroup(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
