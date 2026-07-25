package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Project struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Path            string `json:"path"`
	CanonicalPath   string `json:"canonicalPath"`
	IsGitRepository bool   `json:"isGitRepository"`
	DefaultBranch   string `json:"defaultBranch,omitempty"`
	CreatedAt       string `json:"createdAt"`
	LastOpenedAt    string `json:"lastOpenedAt"`
}

type Thread struct {
	ID         string `json:"id"`
	ProjectID  string `json:"projectId"`
	ProviderID string `json:"providerId"`
	Model      string `json:"model,omitempty"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

type ChatMessage struct {
	ID         string `json:"id"`
	ThreadID   string `json:"threadId"`
	ProviderID string `json:"providerId"`
	Role       string `json:"role"`
	Text       string `json:"text"`
	Status     string `json:"status"`
	CreatedAt  string `json:"createdAt"`
}

type Task struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Title     string `json:"title"`
	Prompt    string `json:"prompt"`
	Mode      string `json:"mode"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type AgentRun struct {
	ID         string `json:"id"`
	TaskID     string `json:"taskId"`
	ProviderID string `json:"providerId"`
	Role       string `json:"role"`
	Position   int    `json:"position"`
	Status     string `json:"status"`
	Progress   int    `json:"progress"`
	ThreadID   string `json:"threadId,omitempty"`
	Worktree   string `json:"worktree,omitempty"`
	Summary    string `json:"summary,omitempty"`
	StartedAt  string `json:"startedAt,omitempty"`
	FinishedAt string `json:"finishedAt,omitempty"`
}

type Artifact struct {
	ID        string `json:"id"`
	TaskID    string `json:"taskId"`
	RunID     string `json:"runId,omitempty"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type ProjectMemory struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

type Store struct {
	db *sql.DB
}

func Open() (*Store, error) {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	dir := filepath.Join(dataHome, "linux-codex-desktop")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create application data directory: %w", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "metadata.db"))
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func OpenPath(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		PRAGMA foreign_keys = ON;
		PRAGMA journal_mode = WAL;
		CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			path TEXT NOT NULL UNIQUE,
			canonical_path TEXT NOT NULL UNIQUE,
			is_git_repository INTEGER NOT NULL DEFAULT 0,
			default_branch TEXT,
			created_at TEXT NOT NULL,
			last_opened_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS threads (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS agent_tasks (
			id TEXT PRIMARY KEY, project_id TEXT NOT NULL, title TEXT NOT NULL,
			prompt TEXT NOT NULL, mode TEXT NOT NULL, status TEXT NOT NULL,
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS agent_runs (
			id TEXT PRIMARY KEY, task_id TEXT NOT NULL, provider_id TEXT NOT NULL,
			role TEXT NOT NULL, position INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL, progress INTEGER NOT NULL DEFAULT 0,
			thread_id TEXT, worktree TEXT, summary TEXT, started_at TEXT, finished_at TEXT,
			FOREIGN KEY (task_id) REFERENCES agent_tasks(id) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS artifacts (
			id TEXT PRIMARY KEY, task_id TEXT NOT NULL, run_id TEXT, type TEXT NOT NULL,
			title TEXT NOT NULL, status TEXT NOT NULL, content TEXT NOT NULL,
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
			FOREIGN KEY (task_id) REFERENCES agent_tasks(id) ON DELETE CASCADE,
			FOREIGN KEY (run_id) REFERENCES agent_runs(id) ON DELETE SET NULL
		);
		CREATE TABLE IF NOT EXISTS project_memory (
			id TEXT PRIMARY KEY, project_id TEXT NOT NULL, kind TEXT NOT NULL,
			title TEXT NOT NULL, content TEXT NOT NULL, status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS chat_messages (
			id TEXT PRIMARY KEY, thread_id TEXT NOT NULL, provider_id TEXT NOT NULL,
			role TEXT NOT NULL, text TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL,
			FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_chat_messages_thread ON chat_messages(thread_id, created_at);
		CREATE INDEX IF NOT EXISTS idx_agent_tasks_project ON agent_tasks(project_id, updated_at DESC);
		CREATE INDEX IF NOT EXISTS idx_agent_runs_task ON agent_runs(task_id);
		CREATE INDEX IF NOT EXISTS idx_artifacts_task ON artifacts(task_id, updated_at DESC);
		INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, datetime('now'));
		INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (2, datetime('now'));
	`)
	if err == nil {
		_, _ = s.db.ExecContext(ctx, `ALTER TABLE threads ADD COLUMN provider_id TEXT NOT NULL DEFAULT 'codex'`)
		_, _ = s.db.ExecContext(ctx, `ALTER TABLE threads ADD COLUMN model TEXT NOT NULL DEFAULT ''`)
	}
	return err
}

func (s *Store) AddChatMessage(ctx context.Context, message ChatMessage) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO chat_messages
		(id, thread_id, provider_id, role, text, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		message.ID, message.ThreadID, message.ProviderID, message.Role, message.Text, message.Status, message.CreatedAt)
	return err
}

func (s *Store) ListChatMessages(ctx context.Context, threadID string) ([]ChatMessage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, thread_id, provider_id, role, text, status, created_at
		FROM chat_messages WHERE thread_id = ? ORDER BY created_at`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []ChatMessage
	for rows.Next() {
		var message ChatMessage
		if err := rows.Scan(&message.ID, &message.ThreadID, &message.ProviderID, &message.Role,
			&message.Text, &message.Status, &message.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (s *Store) CreateTask(ctx context.Context, task Task) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO agent_tasks
		(id, project_id, title, prompt, mode, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.ProjectID, task.Title, task.Prompt, task.Mode, task.Status, task.CreatedAt, task.UpdatedAt)
	return err
}

func (s *Store) ListTasks(ctx context.Context, projectID string) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, project_id, title, prompt, mode, status, created_at, updated_at
		FROM agent_tasks WHERE (? = '' OR project_id = ?) ORDER BY updated_at DESC`, projectID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.ProjectID, &task.Title, &task.Prompt, &task.Mode, &task.Status, &task.CreatedAt, &task.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *Store) GetTask(ctx context.Context, id string) (Task, error) {
	var task Task
	err := s.db.QueryRowContext(ctx, `SELECT id, project_id, title, prompt, mode, status, created_at, updated_at
		FROM agent_tasks WHERE id = ?`, id).Scan(&task.ID, &task.ProjectID, &task.Title,
		&task.Prompt, &task.Mode, &task.Status, &task.CreatedAt, &task.UpdatedAt)
	return task, err
}

func (s *Store) UpdateTaskStatus(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE agent_tasks SET status = ?, updated_at = ? WHERE id = ?`, status, now(), id)
	return err
}

func (s *Store) CreateRun(ctx context.Context, run AgentRun) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO agent_runs
		(id, task_id, provider_id, role, position, status, progress, thread_id, worktree, summary, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.TaskID, run.ProviderID, run.Role, run.Position, run.Status, run.Progress,
		nullIfEmpty(run.ThreadID), nullIfEmpty(run.Worktree), nullIfEmpty(run.Summary),
		nullIfEmpty(run.StartedAt), nullIfEmpty(run.FinishedAt))
	return err
}

func (s *Store) ListRuns(ctx context.Context, taskID string) ([]AgentRun, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, task_id, provider_id, role, position, status, progress,
		COALESCE(thread_id,''), COALESCE(worktree,''), COALESCE(summary,''),
		COALESCE(started_at,''), COALESCE(finished_at,'')
		FROM agent_runs WHERE (? = '' OR task_id = ?) ORDER BY position ASC`, taskID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []AgentRun
	for rows.Next() {
		var run AgentRun
		if err := rows.Scan(&run.ID, &run.TaskID, &run.ProviderID, &run.Role, &run.Position, &run.Status,
			&run.Progress, &run.ThreadID, &run.Worktree, &run.Summary, &run.StartedAt, &run.FinishedAt); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *Store) UpdateRun(ctx context.Context, id, status string, progress int, summary, threadID string) error {
	finished := ""
	if status == "completed" || status == "failed" || status == "cancelled" {
		finished = now()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE agent_runs SET status = ?, progress = ?, summary = ?,
		thread_id = CASE WHEN ? = '' THEN thread_id ELSE ? END,
		finished_at = CASE WHEN ? = '' THEN finished_at ELSE ? END WHERE id = ?`,
		status, progress, nullIfEmpty(summary), threadID, threadID, finished, finished, id)
	return err
}

func (s *Store) SetRunWorktree(ctx context.Context, id, worktree string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE agent_runs SET worktree = ? WHERE id = ?`, worktree, id)
	return err
}

func (s *Store) GetRunByThread(ctx context.Context, threadID string) (AgentRun, error) {
	var run AgentRun
	err := s.db.QueryRowContext(ctx, `SELECT id, task_id, provider_id, role, position, status,
		progress, COALESCE(thread_id,''), COALESCE(worktree,''), COALESCE(summary,''),
		COALESCE(started_at,''), COALESCE(finished_at,'') FROM agent_runs WHERE thread_id = ?`,
		threadID).Scan(&run.ID, &run.TaskID, &run.ProviderID, &run.Role, &run.Position,
		&run.Status, &run.Progress, &run.ThreadID, &run.Worktree, &run.Summary,
		&run.StartedAt, &run.FinishedAt)
	return run, err
}

func (s *Store) CreateArtifact(ctx context.Context, artifact Artifact) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO artifacts
		(id, task_id, run_id, type, title, status, content, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		artifact.ID, artifact.TaskID, nullIfEmpty(artifact.RunID), artifact.Type, artifact.Title,
		artifact.Status, artifact.Content, artifact.CreatedAt, artifact.UpdatedAt)
	return err
}

func (s *Store) ListArtifacts(ctx context.Context, taskID string) ([]Artifact, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, task_id, COALESCE(run_id,''), type, title, status,
		content, created_at, updated_at FROM artifacts WHERE task_id = ? ORDER BY updated_at DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var artifacts []Artifact
	for rows.Next() {
		var artifact Artifact
		if err := rows.Scan(&artifact.ID, &artifact.TaskID, &artifact.RunID, &artifact.Type,
			&artifact.Title, &artifact.Status, &artifact.Content, &artifact.CreatedAt, &artifact.UpdatedAt); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, rows.Err()
}

func (s *Store) AddMemory(ctx context.Context, memory ProjectMemory) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO project_memory
		(id, project_id, kind, title, content, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		memory.ID, memory.ProjectID, memory.Kind, memory.Title, memory.Content, memory.Status, memory.CreatedAt)
	return err
}

func (s *Store) ListMemory(ctx context.Context, projectID string) ([]ProjectMemory, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, project_id, kind, title, content, status, created_at
		FROM project_memory WHERE project_id = ? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var memories []ProjectMemory
	for rows.Next() {
		var memory ProjectMemory
		if err := rows.Scan(&memory.ID, &memory.ProjectID, &memory.Kind, &memory.Title,
			&memory.Content, &memory.Status, &memory.CreatedAt); err != nil {
			return nil, err
		}
		memories = append(memories, memory)
	}
	return memories, rows.Err()
}

func (s *Store) UpdateMemoryStatus(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE project_memory SET status = ? WHERE id = ?`, status, id)
	return err
}

func (s *Store) AddProject(ctx context.Context, project Project) (Project, error) {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO projects (
			id, name, path, canonical_path, is_git_repository, default_branch, created_at, last_opened_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, project.ID, project.Name, project.Path, project.CanonicalPath, project.IsGitRepository,
		nullIfEmpty(project.DefaultBranch), project.CreatedAt, project.LastOpenedAt)
	if err != nil {
		return Project{}, err
	}
	return project, nil
}

func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, path, canonical_path, is_git_repository,
			COALESCE(default_branch, ''), created_at, last_opened_at
		FROM projects ORDER BY last_opened_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projects := make([]Project, 0)
	for rows.Next() {
		var project Project
		if err := rows.Scan(
			&project.ID, &project.Name, &project.Path, &project.CanonicalPath,
			&project.IsGitRepository, &project.DefaultBranch, &project.CreatedAt, &project.LastOpenedAt,
		); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func (s *Store) GetProject(ctx context.Context, id string) (Project, error) {
	var project Project
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, path, canonical_path, is_git_repository,
			COALESCE(default_branch, ''), created_at, last_opened_at
		FROM projects WHERE id = ?
	`, id).Scan(
		&project.ID, &project.Name, &project.Path, &project.CanonicalPath,
		&project.IsGitRepository, &project.DefaultBranch, &project.CreatedAt, &project.LastOpenedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, fmt.Errorf("project not found")
	}
	return project, err
}

func (s *Store) RemoveProject(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("project not found")
	}
	return nil
}

func (s *Store) TouchProject(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE projects SET last_opened_at = ? WHERE id = ?`, now(), id)
	return err
}

func (s *Store) UpsertThread(ctx context.Context, thread Thread) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO threads (id, project_id, provider_id, model, title, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_id = excluded.project_id,
			provider_id = excluded.provider_id,
			model = excluded.model,
			title = CASE WHEN excluded.title = '' THEN threads.title ELSE excluded.title END,
			status = excluded.status,
			updated_at = excluded.updated_at
	`, thread.ID, thread.ProjectID, thread.ProviderID, thread.Model, thread.Title, thread.Status, thread.CreatedAt, thread.UpdatedAt)
	return err
}

func (s *Store) ListThreads(ctx context.Context, projectID string) ([]Thread, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, COALESCE(provider_id,'codex'), COALESCE(model,''), title, status, created_at, updated_at
		FROM threads WHERE project_id = ? ORDER BY updated_at DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	threads := make([]Thread, 0)
	for rows.Next() {
		var thread Thread
		if err := rows.Scan(
			&thread.ID, &thread.ProjectID, &thread.ProviderID, &thread.Model, &thread.Title, &thread.Status,
			&thread.CreatedAt, &thread.UpdatedAt,
		); err != nil {
			return nil, err
		}
		threads = append(threads, thread)
	}
	return threads, rows.Err()
}

func (s *Store) GetThread(ctx context.Context, id string) (Thread, error) {
	var thread Thread
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, COALESCE(provider_id,'codex'), COALESCE(model,''), title, status, created_at, updated_at
		FROM threads WHERE id = ?
	`, id).Scan(
		&thread.ID, &thread.ProjectID, &thread.ProviderID, &thread.Model, &thread.Title, &thread.Status,
		&thread.CreatedAt, &thread.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Thread{}, fmt.Errorf("thread not found")
	}
	return thread, err
}

func (s *Store) UpdateThreadTitle(ctx context.Context, id, title string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE threads SET title = ?, updated_at = ? WHERE id = ?
	`, title, now(), id)
	return err
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
