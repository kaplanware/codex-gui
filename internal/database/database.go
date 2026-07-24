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
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
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
		INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, datetime('now'));
	`)
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
		INSERT INTO threads (id, project_id, title, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_id = excluded.project_id,
			title = CASE WHEN excluded.title = '' THEN threads.title ELSE excluded.title END,
			status = excluded.status,
			updated_at = excluded.updated_at
	`, thread.ID, thread.ProjectID, thread.Title, thread.Status, thread.CreatedAt, thread.UpdatedAt)
	return err
}

func (s *Store) ListThreads(ctx context.Context, projectID string) ([]Thread, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, title, status, created_at, updated_at
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
			&thread.ID, &thread.ProjectID, &thread.Title, &thread.Status,
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
		SELECT id, project_id, title, status, created_at, updated_at
		FROM threads WHERE id = ?
	`, id).Scan(
		&thread.ID, &thread.ProjectID, &thread.Title, &thread.Status,
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
