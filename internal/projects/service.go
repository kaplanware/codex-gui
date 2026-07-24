package projects

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaplanware/codex-gui/internal/database"
)

type Service struct {
	store *database.Store
}

func NewService(store *database.Store) *Service {
	return &Service{store: store}
}

func (s *Service) Add(ctx context.Context, path string) (database.Project, error) {
	if strings.TrimSpace(path) == "" {
		return database.Project{}, fmt.Errorf("project path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return database.Project{}, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return database.Project{}, err
	}
	if !info.IsDir() {
		return database.Project{}, fmt.Errorf("project path is not a directory")
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return database.Project{}, err
	}
	gitRoot, gitBranch := gitInfo(ctx, canonical)
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	project := database.Project{
		ID: newID(), Name: filepath.Base(canonical), Path: absolute, CanonicalPath: canonical,
		IsGitRepository: gitRoot != "", DefaultBranch: gitBranch,
		CreatedAt: timestamp, LastOpenedAt: timestamp,
	}
	return s.store.AddProject(ctx, project)
}

func (s *Service) List(ctx context.Context) ([]database.Project, error) {
	return s.store.ListProjects(ctx)
}

func (s *Service) Get(ctx context.Context, id string) (database.Project, error) {
	return s.store.GetProject(ctx, id)
}

func (s *Service) Remove(ctx context.Context, id string) error {
	return s.store.RemoveProject(ctx, id)
}

func gitInfo(ctx context.Context, path string) (root, branch string) {
	commandCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "git", "-C", path, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", ""
	}
	root = strings.TrimSpace(string(output))
	output, err = exec.CommandContext(commandCtx, "git", "-C", path, "symbolic-ref", "--short", "HEAD").Output()
	if err == nil {
		branch = strings.TrimSpace(string(output))
	}
	return root, branch
}

func newID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value)
}
