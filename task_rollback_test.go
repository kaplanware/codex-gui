package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaplanware/codex-gui/internal/database"
	"github.com/kaplanware/codex-gui/internal/projects"
)

func TestDependentTaskChain(t *testing.T) {
	tasks := []database.Task{
		{ID: "one"},
		{ID: "two", ParentTaskID: "one"},
		{ID: "three", ParentTaskID: "two"},
		{ID: "other"},
	}
	chain := dependentTaskChain("two", tasks)
	if len(chain) != 2 {
		t.Fatalf("dependentTaskChain() = %#v, want task two and three", chain)
	}
}

func TestFileRollbackRestoresBaseline(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := captureTaskFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := database.OpenPath(filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	project := database.Project{
		ID: "project", Name: "project", Path: root, CanonicalPath: root,
		CreatedAt: now, LastOpenedAt: now,
	}
	if _, err := store.AddProject(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	task := database.Task{
		ID: "task", ProjectID: project.ID, Title: "Change app", Prompt: "Change it",
		Mode: "delivery", Status: "completed", WorkspaceMode: "current",
		ApprovalMode: "manual", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(taskSnapshotPayload{
		Project: root, CapturedAt: time.Now().UTC(), Baseline: true, Files: files,
	})
	if err := store.CreateArtifact(context.Background(), database.Artifact{
		ID: "snapshot", TaskID: task.ID, Type: "snapshot", Title: "Baseline",
		Status: "available", Content: string(payload), CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	added := filepath.Join(root, "added.txt")
	if err := os.WriteFile(added, []byte("remove me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := &App{store: store, projects: projects.NewService(store)}
	if err := app.rollbackTask(task); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(path)
	if string(content) != "before\n" {
		t.Fatalf("restored content = %q", content)
	}
	if _, err := os.Stat(added); !os.IsNotExist(err) {
		t.Fatalf("added file still exists: %v", err)
	}
}

func TestCreateTaskWorkspaceUsesLinkedWorktree(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		command := exec.Command("git", args...)
		command.Dir = repository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	runGit("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "README.md")
	runGit("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "base")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	app := &App{}
	worktree, branch, err := app.createTaskWorkspace(database.Project{
		ID: "project", CanonicalPath: repository, IsGitRepository: true,
	}, database.Task{
		ID: "task-123456789", Title: "Add feature", WorkspaceMode: "new-branch",
		BaseBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if branch == "" {
		t.Fatal("generated branch is empty")
	}
	if _, err := os.Stat(filepath.Join(worktree, "README.md")); err != nil {
		t.Fatal(err)
	}
	current := exec.Command("git", "branch", "--show-current")
	current.Dir = repository
	output, err := current.Output()
	if err != nil || string(output) != "main\n" {
		t.Fatalf("main worktree branch changed: %q, %v", output, err)
	}
}

func TestParseCheckedOutBranches(t *testing.T) {
	output := `worktree /repo
HEAD 1111111111111111111111111111111111111111
branch refs/heads/main

worktree /tmp/task-worktree
HEAD 2222222222222222222222222222222222222222
branch refs/heads/agent/feature
`
	branches := parseCheckedOutBranches(output)
	if branches["main"] != "/repo" {
		t.Fatalf("main worktree = %q, want /repo", branches["main"])
	}
	if branches["agent/feature"] != "/tmp/task-worktree" {
		t.Fatalf("task worktree = %q, want /tmp/task-worktree", branches["agent/feature"])
	}
}
