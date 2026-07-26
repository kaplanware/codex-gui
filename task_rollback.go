package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaplanware/codex-gui/internal/database"
)

type RollbackTaskImpact struct {
	TaskID      string `json:"taskId"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Branch      string `json:"branch,omitempty"`
	Workspace   string `json:"workspace"`
	ChangeCount int    `json:"changeCount"`
	CanRollback bool   `json:"canRollback"`
	BlockReason string `json:"blockReason,omitempty"`
}

type RollbackPreview struct {
	TaskID      string               `json:"taskId"`
	CanRollback bool                 `json:"canRollback"`
	Warning     string               `json:"warning"`
	Tasks       []RollbackTaskImpact `json:"tasks"`
}

func (a *App) PreviewTaskRollback(taskID string) (RollbackPreview, error) {
	root, err := a.store.GetTask(context.Background(), taskID)
	if err != nil {
		return RollbackPreview{}, err
	}
	tasks, err := a.store.ListTasks(context.Background(), root.ProjectID)
	if err != nil {
		return RollbackPreview{}, err
	}
	selected := dependentTaskChain(taskID, tasks)
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].CreatedAt > selected[j].CreatedAt
	})
	preview := RollbackPreview{
		TaskID:  taskID,
		Warning: "Rollback permanently discards the selected task's changes and every dependent task created after it. Tasks are reverted newest-first.",
	}
	preview.CanRollback = len(selected) > 0
	sharedWorkspaceWarning := false
	for _, task := range selected {
		impact := RollbackTaskImpact{
			TaskID: task.ID, Title: task.Title, Status: task.Status,
			Branch: task.TaskBranch, Workspace: task.WorkspaceMode,
		}
		if changes, changeErr := a.GetTaskChanges(task.ID); changeErr == nil {
			impact.ChangeCount = len(changes.Changes)
		}
		impact.CanRollback, impact.BlockReason = a.canRollbackTask(task)
		sharedWorkspaceWarning = sharedWorkspaceWarning || task.Worktree == ""
		if !impact.CanRollback {
			preview.CanRollback = false
		}
		preview.Tasks = append(preview.Tasks, impact)
	}
	if sharedWorkspaceWarning {
		preview.Warning += " At least one task used the shared project directory, so its rollback restores the complete captured baseline and can also discard unrelated edits made after that snapshot."
	}
	return preview, nil
}

func dependentTaskChain(rootID string, tasks []database.Task) []database.Task {
	included := map[string]bool{rootID: true}
	changed := true
	for changed {
		changed = false
		for _, task := range tasks {
			if task.ParentTaskID != "" && included[task.ParentTaskID] && !included[task.ID] {
				included[task.ID] = true
				changed = true
			}
		}
	}
	result := make([]database.Task, 0, len(included))
	for _, task := range tasks {
		if included[task.ID] {
			result = append(result, task)
		}
	}
	return result
}

func (a *App) canRollbackTask(task database.Task) (bool, string) {
	if task.RollbackStatus == "completed" {
		return false, "Task is already rolled back"
	}
	if task.Worktree != "" && task.BaseGitSHA != "" {
		if info, err := os.Stat(task.Worktree); err == nil && info.IsDir() {
			return true, ""
		}
		return false, "Task worktree is unavailable"
	}
	artifacts, err := a.store.ListArtifacts(context.Background(), task.ID)
	if err != nil {
		return false, err.Error()
	}
	if _, ok := snapshotFromArtifacts(artifacts); !ok {
		return false, "No exact pre-task baseline is available"
	}
	if err := a.validateFileRollback(task, artifacts); err != nil {
		return false, err.Error()
	}
	return true, ""
}

func (a *App) validateFileRollback(task database.Task, artifacts []database.Artifact) error {
	baseline, ok := snapshotFromArtifacts(artifacts)
	if !ok {
		return errors.New("exact baseline is unavailable")
	}
	current, err := captureTaskFiles(baseline.Project)
	if err != nil {
		return err
	}
	for path, before := range baseline.Files {
		after, exists := current[path]
		if exists && after.Hash == before.Hash {
			continue
		}
		if !before.Text {
			return fmt.Errorf("binary or large file cannot be restored safely: %s", path)
		}
	}
	return nil
}

func (a *App) ExecuteTaskRollback(taskID, confirmation string) (RollbackPreview, error) {
	root, err := a.store.GetTask(context.Background(), taskID)
	if err != nil {
		return RollbackPreview{}, err
	}
	if strings.TrimSpace(confirmation) != root.Title {
		return RollbackPreview{}, errors.New("confirmation must exactly match the task title")
	}
	preview, err := a.PreviewTaskRollback(taskID)
	if err != nil {
		return RollbackPreview{}, err
	}
	if !preview.CanRollback {
		return preview, errors.New("rollback is blocked; review the task impact list")
	}
	tasksByID := make(map[string]database.Task, len(preview.Tasks))
	tasks, _ := a.store.ListTasks(context.Background(), root.ProjectID)
	for _, task := range tasks {
		tasksByID[task.ID] = task
	}
	for _, impact := range preview.Tasks {
		task := tasksByID[impact.TaskID]
		a.cancelTaskProcesses(task.ID)
		if err := a.rollbackTask(task); err != nil {
			_ = a.store.UpdateTaskRollbackStatus(context.Background(), task.ID, "rollback-failed", err.Error())
			return preview, fmt.Errorf("rollback %s: %w", task.Title, err)
		}
		_ = a.store.UpdateTaskRollbackStatus(context.Background(), task.ID, "rolled-back", "completed")
		timestamp := time.Now().UTC().Format(time.RFC3339Nano)
		content, _ := json.MarshalIndent(impact, "", "  ")
		_ = a.store.CreateArtifact(context.Background(), database.Artifact{
			ID: newLocalID("rollback"), TaskID: task.ID, Type: "rollback",
			Title: "Rollback completed", Status: "verified", Content: string(content),
			CreatedAt: timestamp, UpdatedAt: timestamp,
		})
		a.stopCoordinationWatcher(task.ID)
	}
	a.emitAgentTasksChanged()
	return a.PreviewTaskRollback(taskID)
}

func (a *App) rollbackTask(task database.Task) error {
	if task.Worktree != "" && task.BaseGitSHA != "" {
		if output, err := commandOutput(context.Background(), task.Worktree, 30*time.Second,
			"git", "reset", "--hard", task.BaseGitSHA); err != nil {
			return fmt.Errorf("reset isolated branch: %w: %s", err, output)
		}
		if output, err := commandOutput(context.Background(), task.Worktree, 30*time.Second,
			"git", "clean", "-fd"); err != nil {
			return fmt.Errorf("clean isolated branch: %w: %s", err, output)
		}
		return nil
	}
	artifacts, err := a.store.ListArtifacts(context.Background(), task.ID)
	if err != nil {
		return err
	}
	baseline, ok := snapshotFromArtifacts(artifacts)
	if !ok {
		return errors.New("exact baseline is unavailable")
	}
	current, err := captureTaskFiles(baseline.Project)
	if err != nil {
		return err
	}
	for path, before := range baseline.Files {
		after, exists := current[path]
		if exists && after.Hash == before.Hash {
			continue
		}
		if !before.Text {
			return fmt.Errorf("cannot safely restore binary or large file: %s", path)
		}
	}
	for path := range current {
		if _, existed := baseline.Files[path]; existed {
			continue
		}
		if err := os.Remove(filepath.Join(baseline.Project, filepath.FromSlash(path))); err != nil &&
			!errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for path, before := range baseline.Files {
		after, exists := current[path]
		if exists && after.Hash == before.Hash {
			continue
		}
		target := filepath.Join(baseline.Project, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		mode := os.FileMode(before.Mode)
		if mode == 0 {
			mode = 0o600
		}
		if err := os.WriteFile(target, []byte(before.Content), mode); err != nil {
			return err
		}
	}
	return nil
}
