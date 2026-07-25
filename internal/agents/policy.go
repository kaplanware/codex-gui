package agents

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type PreflightCheck struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Blocking bool   `json:"blocking"`
}

type ContextEntry struct {
	Path            string `json:"path"`
	Kind            string `json:"kind"`
	Reason          string `json:"reason"`
	Sensitive       bool   `json:"sensitive"`
	SizeBytes       int64  `json:"sizeBytes"`
	EstimatedTokens int64  `json:"estimatedTokens"`
}

type RiskAssessment struct {
	Level          string   `json:"level"`
	Reasons        []string `json:"reasons"`
	Reversible     bool     `json:"reversible"`
	OutsideProject bool     `json:"outsideProject"`
}

func Preflight(ctx context.Context, root string) []PreflightCheck {
	checks := []PreflightCheck{
		{ID: "directory", Label: "Project directory", Status: "passed", Detail: root, Blocking: true},
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		checks[0].Status, checks[0].Detail = "failed", "Project directory is unavailable"
		return checks
	}
	commandCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "git", "-C", root, "status", "--porcelain").Output()
	git := PreflightCheck{ID: "git", Label: "Git state", Blocking: false}
	if err != nil {
		git.Status, git.Detail = "warning", "Not a Git repository"
	} else if strings.TrimSpace(string(output)) == "" {
		git.Status, git.Detail = "passed", "Working tree is clean"
	} else {
		git.Status, git.Detail = "warning", "Working tree has uncommitted changes"
	}
	checks = append(checks, git)
	branchOutput, branchErr := exec.CommandContext(commandCtx, "git", "-C", root, "branch", "--show-current").Output()
	if branchErr == nil {
		branch := strings.TrimSpace(string(branchOutput))
		status := "passed"
		detail := "Current branch: " + branch
		if branch == "main" || branch == "master" {
			status = "warning"
			detail += " (protected-branch policy recommended)"
		}
		checks = append(checks, PreflightCheck{ID: "branch", Label: "Branch guard", Status: status, Detail: detail})
	}
	for _, marker := range []string{".env.production", "terraform.tfstate", "kubeconfig"} {
		if _, markerErr := os.Stat(filepath.Join(root, marker)); markerErr == nil {
			checks = append(checks, PreflightCheck{
				ID: "production-" + marker, Label: "Production guard", Status: "warning",
				Detail: "Production-sensitive marker detected: " + marker, Blocking: true,
			})
		}
	}
	return checks
}

func InspectContext(root string) ([]ContextEntry, error) {
	candidates := []string{"AGENTS.md", "README.md", "go.mod", "package.json", "pyproject.toml", "Cargo.toml", "composer.json", ".env", ".env.production"}
	entries := make([]ContextEntry, 0)
	for _, candidate := range candidates {
		path := filepath.Join(root, candidate)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		sensitive := strings.HasPrefix(candidate, ".env")
		kind, reason := "included", "Project foundation file"
		if sensitive {
			kind, reason = "excluded", "Sensitive environment configuration"
		}
		entries = append(entries, ContextEntry{
			Path: candidate, Kind: kind, Reason: reason, Sensitive: sensitive,
			SizeBytes: info.Size(), EstimatedTokens: info.Size() / 4,
		})
	}
	return entries, nil
}

func AssessCommand(command, root string) RiskAssessment {
	lower := strings.ToLower(command)
	assessment := RiskAssessment{Level: "low", Reversible: true}
	critical := []string{"rm -rf", "git reset --hard", "git clean -fd", "drop database", "truncate ", "terraform destroy", "kubectl delete", "docker system prune"}
	high := []string{"sudo ", "chmod -r", "chown -r", "migrate", "deploy", "push --force"}
	for _, pattern := range critical {
		if strings.Contains(lower, pattern) {
			assessment.Level = "critical"
			assessment.Reversible = false
			assessment.Reasons = append(assessment.Reasons, "Destructive operation detected: "+pattern)
		}
	}
	if assessment.Level != "critical" {
		for _, pattern := range high {
			if strings.Contains(lower, pattern) {
				assessment.Level = "high"
				assessment.Reasons = append(assessment.Reasons, "Elevated-risk operation detected: "+pattern)
			}
		}
	}
	if strings.Contains(command, "../") || strings.Contains(command, "/etc/") || strings.Contains(command, "/home/") {
		assessment.OutsideProject = true
		assessment.Reasons = append(assessment.Reasons, "Command may address a path outside "+root)
		if assessment.Level == "low" {
			assessment.Level = "medium"
		}
	}
	if len(assessment.Reasons) == 0 {
		assessment.Reasons = []string{"No known destructive pattern detected"}
	}
	return assessment
}
