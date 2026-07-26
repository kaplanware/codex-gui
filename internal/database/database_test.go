package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestProjectAndThreadPersistence(t *testing.T) {
	store, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenPath() error = %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	project := Project{
		ID: "p1", Name: "Example", Path: "/tmp/example", CanonicalPath: "/tmp/example",
		IsGitRepository: true, CreatedAt: now(), LastOpenedAt: now(),
	}
	if _, err := store.AddProject(ctx, project); err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	if err := store.UpsertThread(ctx, Thread{
		ID: "t1", ProjectID: "p1", Title: "First thread", Status: "active",
		CreatedAt: now(), UpdatedAt: now(),
	}); err != nil {
		t.Fatalf("UpsertThread() error = %v", err)
	}
	task := Task{
		ID: "task-1", ProjectID: "p1", Title: "Deliver feature", Prompt: "Implement it",
		Mode: "delivery", Status: "queued", CreatedAt: now(), UpdatedAt: now(),
	}
	if err := store.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if err := store.CreateRun(ctx, AgentRun{
		ID: "run-1", TaskID: task.ID, ProviderID: "codex", Role: "planner", Status: "queued",
	}); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if err := store.CreateArtifact(ctx, Artifact{
		ID: "artifact-1", TaskID: task.ID, Type: "plan", Title: "Plan",
		Status: "needs-review", Content: "Step 1", CreatedAt: now(), UpdatedAt: now(),
	}); err != nil {
		t.Fatalf("CreateArtifact() error = %v", err)
	}
	tasks, err := store.ListTasks(ctx, "p1")
	if err != nil || len(tasks) != 1 {
		t.Fatalf("ListTasks() = %v, %v", tasks, err)
	}
	runs, err := store.ListRuns(ctx, task.ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("ListRuns() = %v, %v", runs, err)
	}
	if err := store.UpdateRun(ctx, "run-1", "running", 10, "Started", "thread-1"); err != nil {
		t.Fatalf("UpdateRun() error = %v", err)
	}
	runs, err = store.ListRuns(ctx, task.ID)
	if err != nil || runs[0].StartedAt == "" || runs[0].ThreadID != "thread-1" {
		t.Fatalf("running ListRuns() = %v, %v", runs, err)
	}
	if err := store.ResetRun(ctx, "run-1", "queued", "Retry"); err != nil {
		t.Fatalf("ResetRun() error = %v", err)
	}
	runs, err = store.ListRuns(ctx, task.ID)
	if err != nil || runs[0].Status != "queued" || runs[0].ThreadID != "" ||
		runs[0].StartedAt != "" || runs[0].FinishedAt != "" {
		t.Fatalf("reset ListRuns() = %v, %v", runs, err)
	}
	projects, err := store.ListProjects(ctx)
	if err != nil || len(projects) != 1 {
		t.Fatalf("ListProjects() = %v, %v", projects, err)
	}
	threads, err := store.ListThreads(ctx, "p1")
	if err != nil || len(threads) != 1 || threads[0].Title != "First thread" {
		t.Fatalf("ListThreads() = %v, %v", threads, err)
	}
	if err := store.RemoveProject(ctx, "p1"); err != nil {
		t.Fatalf("RemoveProject() error = %v", err)
	}
	threads, err = store.ListThreads(ctx, "p1")
	if err != nil || len(threads) != 0 {
		t.Fatalf("cascaded ListThreads() = %v, %v", threads, err)
	}
}
