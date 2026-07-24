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
