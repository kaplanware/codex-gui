package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureAndCompareTaskFilesWithoutGit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "existing.txt"), []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".agent-workspace"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agent-workspace", "state.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	baseline, err := captureTaskFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, included := baseline[".agent-workspace/state.json"]; included {
		t.Fatal("coordination state must not be tracked as a project change")
	}
	if err := os.WriteFile(filepath.Join(root, "existing.txt"), []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "added.txt"), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	current, err := captureTaskFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	changes := compareTaskFiles(baseline, current, "Project workspace", "")
	if len(changes) != 2 {
		t.Fatalf("changes = %#v, want 2", changes)
	}
	if changes[0].Kind != "added" || changes[1].Kind != "modified" {
		t.Fatalf("unexpected change kinds: %#v", changes)
	}
	if changes[1].Diff == "" {
		t.Fatal("modified text file should include an inline diff")
	}
}
