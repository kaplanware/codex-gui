package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaplanware/codex-gui/internal/database"
)

const snapshotContentLimit = 512 * 1024

type taskFileSnapshot struct {
	Hash       string `json:"hash"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modifiedAt"`
	Mode       uint32 `json:"mode"`
	Content    string `json:"content,omitempty"`
	Text       bool   `json:"text"`
}

type taskSnapshotPayload struct {
	GitSHA     string                      `json:"gitSha,omitempty"`
	DirtyState []string                    `json:"dirtyState,omitempty"`
	Diff       string                      `json:"diff,omitempty"`
	Project    string                      `json:"project"`
	CapturedAt time.Time                   `json:"capturedAt"`
	Baseline   bool                        `json:"baseline"`
	Files      map[string]taskFileSnapshot `json:"files,omitempty"`
}

type TaskFileChange struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Workspace string `json:"workspace"`
	RunID     string `json:"runId,omitempty"`
	Size      int64  `json:"size"`
	Diff      string `json:"diff,omitempty"`
	Inferred  bool   `json:"inferred"`
}

type TaskChanges struct {
	TaskID     string           `json:"taskId"`
	BaselineAt string           `json:"baselineAt"`
	CapturedAt string           `json:"capturedAt"`
	Source     string           `json:"source"`
	Changes    []TaskFileChange `json:"changes"`
}

func captureTaskFiles(root string) (map[string]taskFileSnapshot, error) {
	files := make(map[string]taskFileSnapshot)
	contentBudget := int64(16 * 1024 * 1024)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if relative != "." && ignoredTaskDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || ignoredTaskFile(relative) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		hasher := sha256.New()
		_, copyErr := io.Copy(hasher, file)
		_ = file.Close()
		if copyErr != nil {
			return nil
		}
		snapshot := taskFileSnapshot{
			Hash: hex.EncodeToString(hasher.Sum(nil)), Size: info.Size(),
			ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano), Mode: uint32(info.Mode().Perm()),
		}
		if info.Size() <= snapshotContentLimit && info.Size() <= contentBudget {
			if content, readErr := os.ReadFile(path); readErr == nil && !strings.ContainsRune(string(content), '\x00') {
				snapshot.Text = true
				snapshot.Content = string(content)
				contentBudget -= info.Size()
			}
		}
		files[filepath.ToSlash(relative)] = snapshot
		return nil
	})
	return files, err
}

func ignoredTaskDirectory(name string) bool {
	switch name {
	case ".git", ".agent-workspace", "node_modules", "vendor", "dist", "build", ".next", ".nuxt", "coverage":
		return true
	default:
		return false
	}
}

func ignoredTaskFile(relative string) bool {
	name := filepath.Base(relative)
	return name == ".env" || strings.HasPrefix(name, ".env.") ||
		strings.HasSuffix(name, ".pem") || strings.HasSuffix(name, ".key")
}

func compareTaskFiles(
	baseline, current map[string]taskFileSnapshot,
	workspace, runID string,
) []TaskFileChange {
	paths := make(map[string]bool, len(baseline)+len(current))
	for path := range baseline {
		paths[path] = true
	}
	for path := range current {
		paths[path] = true
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	changes := make([]TaskFileChange, 0)
	for _, path := range ordered {
		before, existed := baseline[path]
		after, exists := current[path]
		if existed && exists && before.Hash == after.Hash {
			continue
		}
		change := TaskFileChange{Path: path, Workspace: workspace, RunID: runID}
		switch {
		case !existed:
			change.Kind, change.Size = "added", after.Size
			change.Diff = renderTaskDiff(path, "", after.Content, false, true, false, after.Text)
		case !exists:
			change.Kind, change.Size = "deleted", before.Size
			change.Diff = renderTaskDiff(path, before.Content, "", true, false, before.Text, false)
		default:
			change.Kind, change.Size = "modified", after.Size
			change.Diff = renderTaskDiff(path, before.Content, after.Content, true, true, before.Text, after.Text)
		}
		changes = append(changes, change)
	}
	return changes
}

func renderTaskDiff(
	path, before, after string,
	beforeExists, afterExists, beforeText, afterText bool,
) string {
	if (beforeExists && !beforeText) || (afterExists && !afterText) {
		return "Binary or large file changed; inline preview is unavailable."
	}
	var output strings.Builder
	fmt.Fprintf(&output, "--- a/%s\n+++ b/%s\n", path, path)
	if before != "" {
		for _, line := range strings.Split(strings.TrimSuffix(before, "\n"), "\n") {
			output.WriteString("-")
			output.WriteString(line)
			output.WriteByte('\n')
		}
	}
	if after != "" {
		for _, line := range strings.Split(strings.TrimSuffix(after, "\n"), "\n") {
			output.WriteString("+")
			output.WriteString(line)
			output.WriteByte('\n')
		}
	}
	return truncate(output.String(), 120000)
}

func snapshotFromArtifacts(artifacts []database.Artifact) (taskSnapshotPayload, bool) {
	for index := len(artifacts) - 1; index >= 0; index-- {
		if artifacts[index].Type != "snapshot" {
			continue
		}
		var payload taskSnapshotPayload
		if json.Unmarshal([]byte(artifacts[index].Content), &payload) == nil &&
			payload.Baseline && payload.Files != nil {
			return payload, true
		}
	}
	return taskSnapshotPayload{}, false
}
