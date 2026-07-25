package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kaplanware/codex-gui/internal/agents"
	"github.com/kaplanware/codex-gui/internal/codex"
	"github.com/kaplanware/codex-gui/internal/database"
	"github.com/kaplanware/codex-gui/internal/projects"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx           context.Context
	manager       *codex.Manager
	store         *database.Store
	projects      *projects.Service
	databaseErr   error
	logger        *slog.Logger
	agentRegistry *agents.Registry
	runMu         sync.Mutex
	turnRuns      map[string]runReference
}

type runReference struct {
	TaskID string
	RunID  string
}

func NewApp() *App {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	store, err := database.Open()
	app := &App{
		manager: codex.NewManager(logger), store: store, databaseErr: err, logger: logger,
		agentRegistry: agents.NewRegistry(), turnRuns: make(map[string]runReference),
	}
	if store != nil {
		app.projects = projects.NewService(store)
	}
	app.manager.SetProtocolHandler(app.handleProtocolMessage)
	return app
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.manager.SetEmitter(func(name string, payload any) {
		runtime.EventsEmit(ctx, name, payload)
	})
	go func() {
		a.agentRegistry.Detect(context.Background())
		if err := a.manager.Start(context.Background()); err != nil {
			slog.Debug("automatic Codex connection failed", "error", err)
		}
	}()
}

type AgentTaskRequest struct {
	ProjectID string            `json:"projectId"`
	Title     string            `json:"title"`
	Prompt    string            `json:"prompt"`
	Workflow  string            `json:"workflow"`
	Roles     []string          `json:"roles"`
	Providers map[string]string `json:"providers"`
}

type AgentTaskBundle struct {
	Task      database.Task       `json:"task"`
	Runs      []database.AgentRun `json:"runs"`
	Artifacts []database.Artifact `json:"artifacts"`
}

func (a *App) shutdown(_ context.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), codex.ShutdownTimeout)
	defer cancel()
	_ = a.manager.Stop(ctx)
	if a.store != nil {
		_ = a.store.Close()
	}
}

func (a *App) GetStartupStatus() codex.StartupStatus {
	return a.manager.Status()
}

func (a *App) DetectCodex() (codex.CodexInstallation, error) {
	return a.manager.Detect(context.Background())
}

func (a *App) StartCodex() error {
	return a.manager.Start(context.Background())
}

func (a *App) RestartCodex() error {
	return a.manager.Restart(context.Background())
}

type AccountView struct {
	SignedIn           bool   `json:"signedIn"`
	Type               string `json:"type,omitempty"`
	Email              string `json:"email,omitempty"`
	PlanType           string `json:"planType,omitempty"`
	RequiresOpenAIAuth bool   `json:"requiresOpenaiAuth"`
}

type LoginStart struct {
	Type            string `json:"type"`
	LoginID         string `json:"loginId"`
	AuthURL         string `json:"authUrl,omitempty"`
	VerificationURL string `json:"verificationUrl,omitempty"`
	UserCode        string `json:"userCode,omitempty"`
}

type ThreadView struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	Items     []any  `json:"items,omitempty"`
}

type TurnStart struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	Status   string `json:"status"`
}

type ModelView struct {
	ID                        string   `json:"id"`
	DisplayName               string   `json:"displayName"`
	Description               string   `json:"description"`
	IsDefault                 bool     `json:"isDefault"`
	DefaultReasoningEffort    string   `json:"defaultReasoningEffort"`
	SupportedReasoningEfforts []string `json:"supportedReasoningEfforts"`
}

type RateLimitWindow struct {
	UsedPercent        float64 `json:"usedPercent"`
	RemainingPercent   float64 `json:"remainingPercent"`
	ResetsAt           int64   `json:"resetsAt,omitempty"`
	WindowDurationMins int64   `json:"windowDurationMins,omitempty"`
}

type RateLimitsView struct {
	LimitName string           `json:"limitName,omitempty"`
	Primary   *RateLimitWindow `json:"primary,omitempty"`
	Secondary *RateLimitWindow `json:"secondary,omitempty"`
}

type FileMatch struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	RelativePath string `json:"relativePath"`
	Score        int64  `json:"score"`
}

type MentionInput struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type TurnOptions struct {
	Model    string         `json:"model,omitempty"`
	Effort   string         `json:"effort,omitempty"`
	Mode     string         `json:"mode,omitempty"`
	Mentions []MentionInput `json:"mentions,omitempty"`
}

type ChatArchive struct {
	Version    int    `json:"version"`
	ExportedAt string `json:"exportedAt"`
	Title      string `json:"title"`
	Items      []any  `json:"items"`
}

type ApprovalDecision struct {
	RequestID string `json:"requestId"`
	Kind      string `json:"kind"`
	Decision  string `json:"decision"`
}

func (a *App) GetAccount() (AccountView, error) {
	var response struct {
		Account            json.RawMessage `json:"account"`
		RequiresOpenAIAuth bool            `json:"requiresOpenaiAuth"`
	}
	if err := a.call("account/read", map[string]any{"refreshToken": false}, &response); err != nil {
		return AccountView{}, err
	}
	view := AccountView{RequiresOpenAIAuth: response.RequiresOpenAIAuth}
	if len(response.Account) == 0 || string(response.Account) == "null" {
		return view, nil
	}
	var account struct {
		Type     string `json:"type"`
		Email    string `json:"email"`
		PlanType string `json:"planType"`
	}
	if err := json.Unmarshal(response.Account, &account); err != nil {
		return AccountView{}, err
	}
	view.SignedIn = true
	view.Type = account.Type
	view.Email = account.Email
	view.PlanType = account.PlanType
	return view, nil
}

func (a *App) GetRateLimits() (RateLimitsView, error) {
	var response struct {
		RateLimits struct {
			LimitName string `json:"limitName"`
			Primary   *struct {
				UsedPercent        float64 `json:"usedPercent"`
				ResetsAt           *int64  `json:"resetsAt"`
				WindowDurationMins *int64  `json:"windowDurationMins"`
			} `json:"primary"`
			Secondary *struct {
				UsedPercent        float64 `json:"usedPercent"`
				ResetsAt           *int64  `json:"resetsAt"`
				WindowDurationMins *int64  `json:"windowDurationMins"`
			} `json:"secondary"`
		} `json:"rateLimits"`
	}
	if err := a.call("account/rateLimits/read", nil, &response); err != nil {
		return RateLimitsView{}, err
	}
	view := RateLimitsView{LimitName: response.RateLimits.LimitName}
	if response.RateLimits.Primary != nil {
		view.Primary = rateLimitWindow(
			response.RateLimits.Primary.UsedPercent,
			response.RateLimits.Primary.ResetsAt,
			response.RateLimits.Primary.WindowDurationMins,
		)
	}
	if response.RateLimits.Secondary != nil {
		view.Secondary = rateLimitWindow(
			response.RateLimits.Secondary.UsedPercent,
			response.RateLimits.Secondary.ResetsAt,
			response.RateLimits.Secondary.WindowDurationMins,
		)
	}
	return view, nil
}

func (a *App) ListModels() ([]ModelView, error) {
	var response struct {
		Data []struct {
			ID                        string `json:"id"`
			DisplayName               string `json:"displayName"`
			Description               string `json:"description"`
			IsDefault                 bool   `json:"isDefault"`
			DefaultReasoningEffort    string `json:"defaultReasoningEffort"`
			SupportedReasoningEfforts []struct {
				ReasoningEffort string `json:"reasoningEffort"`
			} `json:"supportedReasoningEfforts"`
		} `json:"data"`
	}
	if err := a.call("model/list", map[string]any{"limit": 100, "includeHidden": false}, &response); err != nil {
		return nil, err
	}
	models := make([]ModelView, 0, len(response.Data))
	for _, model := range response.Data {
		view := ModelView{
			ID: model.ID, DisplayName: model.DisplayName, Description: model.Description,
			IsDefault: model.IsDefault, DefaultReasoningEffort: model.DefaultReasoningEffort,
		}
		for _, effort := range model.SupportedReasoningEfforts {
			view.SupportedReasoningEfforts = append(view.SupportedReasoningEfforts, effort.ReasoningEffort)
		}
		models = append(models, view)
	}
	return models, nil
}

func (a *App) ListAgentProviders() []agents.ProviderStatus {
	return a.agentRegistry.Detect(context.Background())
}

func (a *App) ListAgentTasks(projectID string) ([]AgentTaskBundle, error) {
	if err := a.requireDatabase(); err != nil {
		return nil, err
	}
	tasks, err := a.store.ListTasks(context.Background(), projectID)
	if err != nil {
		return nil, err
	}
	bundles := make([]AgentTaskBundle, 0, len(tasks))
	for _, task := range tasks {
		runs, runErr := a.store.ListRuns(context.Background(), task.ID)
		if runErr != nil {
			return nil, runErr
		}
		artifacts, artifactErr := a.store.ListArtifacts(context.Background(), task.ID)
		if artifactErr != nil {
			return nil, artifactErr
		}
		bundles = append(bundles, AgentTaskBundle{Task: task, Runs: runs, Artifacts: artifacts})
	}
	return bundles, nil
}

func (a *App) CreateAgentTask(request AgentTaskRequest) (AgentTaskBundle, error) {
	if err := a.requireDatabase(); err != nil {
		return AgentTaskBundle{}, err
	}
	request.Title = strings.TrimSpace(request.Title)
	request.Prompt = strings.TrimSpace(request.Prompt)
	if request.ProjectID == "" || request.Prompt == "" {
		return AgentTaskBundle{}, errors.New("project and task prompt are required")
	}
	if _, err := a.projects.Get(context.Background(), request.ProjectID); err != nil {
		return AgentTaskBundle{}, err
	}
	if request.Title == "" {
		request.Title = request.Prompt
		if len([]rune(request.Title)) > 72 {
			request.Title = string([]rune(request.Title)[:72]) + "…"
		}
	}
	if request.Workflow == "" {
		request.Workflow = "delivery"
	}
	if len(request.Roles) == 0 {
		request.Roles = []string{"planner", "implementer", "reviewer"}
	}
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	task := database.Task{
		ID: newLocalID("task"), ProjectID: request.ProjectID, Title: request.Title,
		Prompt: request.Prompt, Mode: request.Workflow, Status: "queued",
		CreatedAt: timestamp, UpdatedAt: timestamp,
	}
	if err := a.store.CreateTask(context.Background(), task); err != nil {
		return AgentTaskBundle{}, err
	}
	providers := a.agentRegistry.Statuses()
	if len(providers) == 0 {
		providers = a.agentRegistry.Detect(context.Background())
	}
	runs := make([]database.AgentRun, 0, len(request.Roles))
	for index, role := range request.Roles {
		providerID := request.Providers[role]
		if providerID == "" || providerID == "auto" {
			providerID = routeProvider(role, providers)
		}
		status := "queued"
		if !providerAvailable(providerID, providers) {
			status = "unavailable"
		}
		run := database.AgentRun{
			ID: newLocalID("run"), TaskID: task.ID, ProviderID: providerID,
			Role: role, Position: index, Status: status, Progress: 0,
		}
		if request.Workflow == "delivery" && index > 0 && status == "queued" {
			run.Status = "blocked"
		}
		if err := a.store.CreateRun(context.Background(), run); err != nil {
			return AgentTaskBundle{}, err
		}
		runs = append(runs, run)
	}
	preflight, _ := json.MarshalIndent(a.PreflightProject(request.ProjectID), "", "  ")
	artifact := database.Artifact{
		ID: newLocalID("artifact"), TaskID: task.ID, Type: "preflight",
		Title: "Preflight report", Status: "generated", Content: string(preflight),
		CreatedAt: timestamp, UpdatedAt: timestamp,
	}
	_ = a.store.CreateArtifact(context.Background(), artifact)
	artifacts := []database.Artifact{artifact}
	if snapshot, snapshotErr := a.CreateTaskSnapshot(task.ID); snapshotErr == nil {
		artifacts = append(artifacts, snapshot)
	}
	go a.startEligibleRuns(task.ID)
	return AgentTaskBundle{Task: task, Runs: runs, Artifacts: artifacts}, nil
}

func (a *App) PreflightProject(projectID string) []agents.PreflightCheck {
	project, err := a.projects.Get(context.Background(), projectID)
	if err != nil {
		return []agents.PreflightCheck{{ID: "project", Label: "Project", Status: "failed", Detail: err.Error(), Blocking: true}}
	}
	return agents.Preflight(context.Background(), project.CanonicalPath)
}

func (a *App) InspectProjectContext(projectID string) ([]agents.ContextEntry, error) {
	project, err := a.projects.Get(context.Background(), projectID)
	if err != nil {
		return nil, err
	}
	return agents.InspectContext(project.CanonicalPath)
}

func (a *App) AssessCommandRisk(projectID, command string) (agents.RiskAssessment, error) {
	project, err := a.projects.Get(context.Background(), projectID)
	if err != nil {
		return agents.RiskAssessment{}, err
	}
	return agents.AssessCommand(command, project.CanonicalPath), nil
}

func (a *App) ListProjectMemory(projectID string) ([]database.ProjectMemory, error) {
	return a.store.ListMemory(context.Background(), projectID)
}

func (a *App) ProposeProjectMemory(projectID, kind, title, content string) (database.ProjectMemory, error) {
	if _, err := a.projects.Get(context.Background(), projectID); err != nil {
		return database.ProjectMemory{}, err
	}
	memory := database.ProjectMemory{
		ID: newLocalID("memory"), ProjectID: projectID, Kind: kind, Title: title,
		Content: content, Status: "proposed", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	return memory, a.store.AddMemory(context.Background(), memory)
}

func (a *App) ReviewProjectMemory(id, decision string) error {
	if decision != "approved" && decision != "rejected" {
		return errors.New("memory decision must be approved or rejected")
	}
	return a.store.UpdateMemoryStatus(context.Background(), id, decision)
}

func (a *App) ApproveAgentTaskStage(taskID string) error {
	runs, err := a.store.ListRuns(context.Background(), taskID)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.Status == "blocked" {
			if err := a.store.UpdateRun(context.Background(), run.ID, "queued", 0, "Human approval granted", ""); err != nil {
				return err
			}
			_ = a.store.UpdateTaskStatus(context.Background(), taskID, "running")
			go a.startEligibleRuns(taskID)
			a.emitAgentTasksChanged()
			return nil
		}
	}
	return errors.New("task has no stage waiting for approval")
}

func (a *App) CreateTaskSnapshot(taskID string) (database.Artifact, error) {
	task, err := a.store.GetTask(context.Background(), taskID)
	if err != nil {
		return database.Artifact{}, err
	}
	project, err := a.projects.Get(context.Background(), task.ProjectID)
	if err != nil {
		return database.Artifact{}, err
	}
	head, _ := commandOutput(context.Background(), project.CanonicalPath, 5*time.Second, "git", "rev-parse", "HEAD")
	status, _ := commandOutput(context.Background(), project.CanonicalPath, 5*time.Second, "git", "status", "--porcelain=v1")
	diff, _ := commandOutput(context.Background(), project.CanonicalPath, 10*time.Second, "git", "diff", "--binary")
	content, _ := json.MarshalIndent(map[string]any{
		"gitSha": strings.TrimSpace(head), "dirtyState": strings.Split(strings.TrimSpace(status), "\n"),
		"diff": diff, "project": project.CanonicalPath, "capturedAt": time.Now().UTC(),
	}, "", "  ")
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	artifact := database.Artifact{
		ID: newLocalID("snapshot"), TaskID: taskID, Type: "snapshot", Title: "Pre-run snapshot",
		Status: "available", Content: string(content), CreatedAt: timestamp, UpdatedAt: timestamp,
	}
	return artifact, a.store.CreateArtifact(context.Background(), artifact)
}

func (a *App) CreateTaskWorktree(taskID string) (database.Artifact, error) {
	task, err := a.store.GetTask(context.Background(), taskID)
	if err != nil {
		return database.Artifact{}, err
	}
	project, err := a.projects.Get(context.Background(), task.ProjectID)
	if err != nil {
		return database.Artifact{}, err
	}
	if !project.IsGitRepository {
		return database.Artifact{}, errors.New("isolated worktrees require a Git repository")
	}
	root, err := agentWorktreeRoot(project.ID)
	if err != nil {
		return database.Artifact{}, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return database.Artifact{}, err
	}
	target := filepath.Join(root, task.ID)
	branch := "agent-workspace/" + task.ID
	output, err := commandOutput(context.Background(), project.CanonicalPath, 30*time.Second,
		"git", "worktree", "add", "-b", branch, target, "HEAD")
	if err != nil {
		return database.Artifact{}, fmt.Errorf("create worktree: %w: %s", err, output)
	}
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	artifact := database.Artifact{
		ID: newLocalID("worktree"), TaskID: taskID, Type: "worktree", Title: "Disposable worktree",
		Status: "available", Content: target, CreatedAt: timestamp, UpdatedAt: timestamp,
	}
	return artifact, a.store.CreateArtifact(context.Background(), artifact)
}

func agentWorktreeRoot(projectID string) (string, error) {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "linux-codex-desktop", "worktrees", projectID), nil
}

func (a *App) RunTaskVerification(taskID string) (database.Artifact, error) {
	task, err := a.store.GetTask(context.Background(), taskID)
	if err != nil {
		return database.Artifact{}, err
	}
	project, err := a.projects.Get(context.Background(), task.ProjectID)
	if err != nil {
		return database.Artifact{}, err
	}
	type result struct {
		Name    string `json:"name"`
		Command string `json:"command"`
		Passed  bool   `json:"passed"`
		Output  string `json:"output"`
	}
	checks := [][]string{{"git", "diff", "--check"}}
	if _, statErr := os.Stat(filepath.Join(project.CanonicalPath, "go.mod")); statErr == nil {
		checks = append(checks, []string{"go", "test", "./..."})
	}
	if _, statErr := os.Stat(filepath.Join(project.CanonicalPath, "frontend", "package.json")); statErr == nil {
		checks = append(checks, []string{"npm", "test", "--", "--run"})
	}
	results := make([]result, 0, len(checks))
	allPassed := true
	for _, check := range checks {
		cwd := project.CanonicalPath
		if check[0] == "npm" {
			cwd = filepath.Join(cwd, "frontend")
		}
		output, commandErr := commandOutput(context.Background(), cwd, 2*time.Minute, check[0], check[1:]...)
		passed := commandErr == nil
		allPassed = allPassed && passed
		results = append(results, result{
			Name: check[0], Command: strings.Join(check, " "), Passed: passed, Output: truncate(output, 12000),
		})
	}
	content, _ := json.MarshalIndent(results, "", "  ")
	status := "failed"
	if allPassed {
		status = "verified"
	}
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	artifact := database.Artifact{
		ID: newLocalID("verification"), TaskID: taskID, Type: "verification",
		Title: "Verification report", Status: status, Content: string(content),
		CreatedAt: timestamp, UpdatedAt: timestamp,
	}
	return artifact, a.store.CreateArtifact(context.Background(), artifact)
}

func commandOutput(parent context.Context, cwd string, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = cwd
	output, err := command.CombinedOutput()
	return string(output), err
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n…truncated"
}

func (a *App) LoginWithChatGPT() (LoginStart, error) {
	var response LoginStart
	err := a.call("account/login/start", map[string]any{
		"type": "chatgpt", "useHostedLoginSuccessPage": true, "appBrand": "chatgpt",
	}, &response)
	if err != nil {
		return LoginStart{}, err
	}
	if err := validateLoginURL(response.AuthURL); err != nil {
		return LoginStart{}, err
	}
	runtime.BrowserOpenURL(a.ctx, response.AuthURL)
	return response, nil
}

func (a *App) LoginWithDeviceCode() (LoginStart, error) {
	var response LoginStart
	if err := a.call("account/login/start", map[string]any{"type": "chatgptDeviceCode"}, &response); err != nil {
		return LoginStart{}, err
	}
	if err := validateLoginURL(response.VerificationURL); err != nil {
		return LoginStart{}, err
	}
	runtime.BrowserOpenURL(a.ctx, response.VerificationURL)
	return response, nil
}

func (a *App) Logout() error {
	return a.call("account/logout", nil, nil)
}

func (a *App) SelectProject() (database.Project, error) {
	if err := a.requireDatabase(); err != nil {
		return database.Project{}, err
	}
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Add a project directory",
	})
	if err != nil {
		return database.Project{}, err
	}
	if path == "" {
		return database.Project{}, errors.New("project selection was cancelled")
	}
	return a.projects.Add(context.Background(), path)
}

func (a *App) AddProject(path string) (database.Project, error) {
	if err := a.requireDatabase(); err != nil {
		return database.Project{}, err
	}
	return a.projects.Add(context.Background(), path)
}

func (a *App) ListProjects() ([]database.Project, error) {
	if err := a.requireDatabase(); err != nil {
		return nil, err
	}
	return a.projects.List(context.Background())
}

func (a *App) RemoveProject(id string) error {
	if err := a.requireDatabase(); err != nil {
		return err
	}
	return a.projects.Remove(context.Background(), id)
}

func (a *App) ListThreads(projectID string) ([]database.Thread, error) {
	if err := a.requireDatabase(); err != nil {
		return nil, err
	}
	return a.store.ListThreads(context.Background(), projectID)
}

func (a *App) StartThread(projectID string) (ThreadView, error) {
	if err := a.requireDatabase(); err != nil {
		return ThreadView{}, err
	}
	project, err := a.projects.Get(context.Background(), projectID)
	if err != nil {
		return ThreadView{}, err
	}
	return a.startThreadAt(projectID, project.CanonicalPath)
}

func (a *App) startThreadAt(projectID, cwd string) (ThreadView, error) {
	var response struct {
		Thread struct {
			ID        string `json:"id"`
			CreatedAt int64  `json:"createdAt"`
			UpdatedAt int64  `json:"updatedAt"`
		} `json:"thread"`
	}
	err := a.call("thread/start", map[string]any{
		"cwd": cwd, "approvalPolicy": "on-request", "sandbox": "workspace-write",
	}, &response)
	if err != nil {
		return ThreadView{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	thread := database.Thread{
		ID: response.Thread.ID, ProjectID: projectID, Title: "New thread",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}
	if err := a.store.UpsertThread(context.Background(), thread); err != nil {
		return ThreadView{}, err
	}
	_ = a.store.TouchProject(context.Background(), projectID)
	return threadView(thread), nil
}

func (a *App) ResumeThread(threadID string) (ThreadView, error) {
	if err := a.requireDatabase(); err != nil {
		return ThreadView{}, err
	}
	thread, err := a.store.GetThread(context.Background(), threadID)
	if err != nil {
		return ThreadView{}, err
	}
	project, err := a.projects.Get(context.Background(), thread.ProjectID)
	if err != nil {
		return ThreadView{}, err
	}
	var response struct {
		Thread struct {
			Turns []struct {
				Items []any `json:"items"`
			} `json:"turns"`
		} `json:"thread"`
	}
	cwd := project.CanonicalPath
	if run, runErr := a.store.GetRunByThread(context.Background(), threadID); runErr == nil && run.Worktree != "" {
		cwd = run.Worktree
	}
	if err := a.call("thread/resume", map[string]any{
		"threadId": threadID, "cwd": cwd,
	}, &response); err != nil {
		return ThreadView{}, err
	}
	view := threadView(thread)
	for _, turn := range response.Thread.Turns {
		view.Items = append(view.Items, turn.Items...)
	}
	return view, nil
}

func (a *App) SendMessage(threadID, text string) (TurnStart, error) {
	return a.SendMessageWithOptions(threadID, text, TurnOptions{})
}

func (a *App) SendMessageWithOptions(threadID, text string, options TurnOptions) (TurnStart, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return TurnStart{}, errors.New("message cannot be empty")
	}
	thread, err := a.store.GetThread(context.Background(), threadID)
	if err != nil {
		return TurnStart{}, err
	}
	var response struct {
		Turn struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"turn"`
	}
	input := []map[string]any{{
		"type": "text", "text": text, "text_elements": []any{},
	}}
	for _, mention := range options.Mentions {
		if mention.Path == "" {
			continue
		}
		input = append(input, map[string]any{
			"type": "mention", "name": mention.Name, "path": mention.Path,
		})
	}
	params := map[string]any{
		"threadId": threadID,
		"input":    input,
	}
	if options.Model != "" {
		params["model"] = options.Model
	}
	if options.Effort != "" {
		params["effort"] = options.Effort
	}
	if options.Mode == "plan" || options.Mode == "default" {
		model := options.Model
		if model == "" {
			model = "default"
		}
		params["collaborationMode"] = map[string]any{
			"mode": options.Mode,
			"settings": map[string]any{
				"model": model, "reasoning_effort": nullIfEmptyString(options.Effort),
				"developer_instructions": planDeveloperInstructions(options.Mode),
			},
		}
	}
	err = a.call("turn/start", params, &response)
	if err != nil {
		return TurnStart{}, err
	}
	if thread.Title == "" || thread.Title == "New thread" {
		title := text
		if len([]rune(title)) > 64 {
			title = string([]rune(title)[:64]) + "…"
		}
		_ = a.store.UpdateThreadTitle(context.Background(), threadID, title)
	}
	return TurnStart{ThreadID: threadID, TurnID: response.Turn.ID, Status: response.Turn.Status}, nil
}

func planDeveloperInstructions(mode string) any {
	if mode != "plan" {
		return nil
	}
	return `You are in a strict planning phase. Inspect the repository as needed, but do not modify files, run destructive commands, or implement the request. Produce a decision-ready implementation plan with: objective and assumptions; current-state findings; affected files and components; ordered implementation steps; risks and mitigations; verification plan; rollback plan; and explicit questions only when genuinely blocking. End with a concise approval checklist.`
}

func (a *App) SearchProjectFiles(projectID, query string) ([]FileMatch, error) {
	if err := a.requireDatabase(); err != nil {
		return nil, err
	}
	project, err := a.projects.Get(context.Background(), projectID)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	matches := make([]FileMatch, 0, 64)
	skip := map[string]bool{
		".git": true, "node_modules": true, "vendor": true, "dist": true,
		"build": true, ".idea": true, ".vscode": true,
	}
	err = filepath.WalkDir(project.CanonicalPath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if path != project.CanonicalPath && skip[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		relative, relativeErr := filepath.Rel(project.CanonicalPath, path)
		if relativeErr != nil {
			return nil
		}
		relative = filepath.ToSlash(relative)
		lower := strings.ToLower(relative)
		index := strings.Index(lower, needle)
		if needle != "" && index < 0 {
			return nil
		}
		score := int64(1000 - index*5 - len(relative))
		if strings.HasPrefix(strings.ToLower(entry.Name()), needle) {
			score += 500
		}
		matches = append(matches, FileMatch{
			Name: entry.Name(), Path: path, RelativePath: relative, Score: score,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
	if len(matches) > 20 {
		matches = matches[:20]
	}
	return matches, nil
}

func (a *App) OpenFile(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	runtime.BrowserOpenURL(a.ctx, (&url.URL{Scheme: "file", Path: absolute}).String())
	return nil
}

func (a *App) OpenProjectFile(projectID, relativePath string) error {
	project, err := a.projects.Get(context.Background(), projectID)
	if err != nil {
		return err
	}
	root := filepath.Clean(project.CanonicalPath)
	target := filepath.Clean(filepath.Join(root, filepath.FromSlash(relativePath)))
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("file is outside the selected project")
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("path is not a file")
	}
	runtime.BrowserOpenURL(a.ctx, (&url.URL{Scheme: "file", Path: target}).String())
	return nil
}

func (a *App) ExportThread(threadID string) (string, error) {
	thread, err := a.store.GetThread(context.Background(), threadID)
	if err != nil {
		return "", err
	}
	var response struct {
		Thread struct {
			Turns []struct {
				Items []any `json:"items"`
			} `json:"turns"`
		} `json:"thread"`
	}
	if err := a.call("thread/read", map[string]any{
		"threadId": threadID, "includeTurns": true,
	}, &response); err != nil {
		return "", err
	}
	archive := ChatArchive{
		Version: 1, ExportedAt: time.Now().UTC().Format(time.RFC3339Nano), Title: thread.Title,
	}
	for _, turn := range response.Thread.Turns {
		archive.Items = append(archive.Items, turn.Items...)
	}
	data, err := json.MarshalIndent(archive, "", "  ")
	if err != nil {
		return "", err
	}
	defaultName := sanitizeFilename(thread.Title)
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title: "Export Codex chat", DefaultFilename: defaultName + ".codex-chat.json",
		Filters: []runtime.FileFilter{{DisplayName: "Codex Chat Archive", Pattern: "*.codex-chat.json"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (a *App) ImportThread(projectID string) (ThreadView, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Import Codex chat",
		Filters: []runtime.FileFilter{{DisplayName: "Codex Chat Archive", Pattern: "*.codex-chat.json"}},
	})
	if err != nil || path == "" {
		return ThreadView{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return ThreadView{}, err
	}
	if info.Size() > 20*1024*1024 {
		return ThreadView{}, errors.New("chat archive exceeds the 20 MiB import limit")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ThreadView{}, err
	}
	var archive ChatArchive
	if err := json.Unmarshal(data, &archive); err != nil {
		return ThreadView{}, fmt.Errorf("invalid chat archive: %w", err)
	}
	if archive.Version != 1 {
		return ThreadView{}, fmt.Errorf("unsupported chat archive version %d", archive.Version)
	}
	view, err := a.StartThread(projectID)
	if err != nil {
		return ThreadView{}, err
	}
	responseItems := make([]any, 0, len(archive.Items))
	for _, rawItem := range archive.Items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		switch item["type"] {
		case "userMessage":
			text := extractUserMessageText(item)
			if text != "" {
				responseItems = append(responseItems, map[string]any{
					"type": "message", "role": "user",
					"content": []any{map[string]any{"type": "input_text", "text": text}},
				})
			}
		case "agentMessage":
			text, _ := item["text"].(string)
			if text != "" {
				responseItems = append(responseItems, map[string]any{
					"type": "message", "role": "assistant",
					"content": []any{map[string]any{"type": "output_text", "text": text}},
				})
			}
		}
	}
	if len(responseItems) > 0 {
		if err := a.call("thread/inject_items", map[string]any{
			"threadId": view.ID, "items": responseItems,
		}, nil); err != nil {
			return ThreadView{}, err
		}
	}
	title := strings.TrimSpace(archive.Title)
	if title == "" {
		title = "Imported chat"
	} else {
		title = "Imported: " + title
	}
	_ = a.store.UpdateThreadTitle(context.Background(), view.ID, title)
	view.Title = title
	view.Items = archive.Items
	return view, nil
}

func (a *App) InterruptTurn(threadID, turnID string) error {
	return a.call("turn/interrupt", map[string]any{"threadId": threadID, "turnId": turnID}, nil)
}

func (a *App) RespondToApproval(decision ApprovalDecision) error {
	switch decision.Decision {
	case "accept", "acceptForSession", "decline", "cancel":
	default:
		return errors.New("unsupported approval decision")
	}
	switch decision.Kind {
	case "command", "file":
		return a.manager.Respond(decision.RequestID, map[string]any{"decision": decision.Decision})
	case "permissions":
		if decision.Decision == "accept" || decision.Decision == "acceptForSession" {
			return errors.New("permission approvals require the requested permission payload")
		}
		return a.manager.Respond(decision.RequestID, map[string]any{
			"permissions": map[string]any{}, "scope": "turn",
		})
	default:
		return errors.New("unsupported approval kind")
	}
}

func (a *App) RespondToPermission(
	requestID string,
	permissions map[string]any,
	scope string,
) error {
	if scope != "turn" && scope != "session" {
		return errors.New("invalid permission scope")
	}
	return a.manager.Respond(requestID, map[string]any{
		"permissions": permissions, "scope": scope,
	})
}

func (a *App) RespondToUserInput(requestID string, answers map[string][]string) error {
	result := make(map[string]any, len(answers))
	for id, values := range answers {
		result[id] = map[string]any{"answers": values}
	}
	return a.manager.Respond(requestID, map[string]any{"answers": result})
}

func (a *App) RespondToElicitation(requestID, action string, content map[string]any) error {
	if action != "accept" && action != "decline" && action != "cancel" {
		return errors.New("invalid elicitation action")
	}
	var responseContent any
	if action == "accept" {
		responseContent = content
	}
	return a.manager.Respond(requestID, map[string]any{
		"action": action, "content": responseContent, "_meta": nil,
	})
}

func (a *App) OpenInFileManager(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	runtime.BrowserOpenURL(a.ctx, (&url.URL{Scheme: "file", Path: absolute}).String())
	return nil
}

func (a *App) call(method string, params, result any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return a.manager.Call(ctx, method, params, result)
}

func (a *App) startEligibleRuns(taskID string) {
	task, err := a.store.GetTask(context.Background(), taskID)
	if err != nil {
		return
	}
	runs, err := a.store.ListRuns(context.Background(), taskID)
	if err != nil {
		return
	}
	started := false
	for _, run := range runs {
		if run.Status != "queued" {
			continue
		}
		started = true
		if run.ProviderID != "codex" {
			_ = a.store.UpdateRun(context.Background(), run.ID, "unavailable", 0, "Provider execution adapter is not available yet.", "")
			continue
		}
		if err := a.startCodexAgentRun(task, run); err != nil {
			_ = a.store.UpdateRun(context.Background(), run.ID, "failed", 0, err.Error(), "")
		}
		if task.Mode == "delivery" {
			break
		}
	}
	if started {
		_ = a.store.UpdateTaskStatus(context.Background(), taskID, "running")
		a.emitAgentTasksChanged()
	}
}

func (a *App) startCodexAgentRun(task database.Task, run database.AgentRun) error {
	project, err := a.projects.Get(context.Background(), task.ProjectID)
	if err != nil {
		return err
	}
	cwd := project.CanonicalPath
	if task.Mode == "parallel" && project.IsGitRepository {
		cwd, err = a.createRunWorktree(project, run)
		if err != nil {
			return err
		}
		if err := a.store.SetRunWorktree(context.Background(), run.ID, cwd); err != nil {
			return err
		}
	}
	thread, err := a.startThreadAt(task.ProjectID, cwd)
	if err != nil {
		return err
	}
	prompt := rolePrompt(run.Role, task.Prompt)
	mode := "default"
	if run.Role == "planner" {
		mode = "plan"
	}
	turn, err := a.SendMessageWithOptions(thread.ID, prompt, TurnOptions{Mode: mode})
	if err != nil {
		return err
	}
	a.runMu.Lock()
	a.turnRuns[turn.TurnID] = runReference{TaskID: task.ID, RunID: run.ID}
	a.runMu.Unlock()
	if err := a.store.UpdateRun(context.Background(), run.ID, "running", 10, "Codex session started", thread.ID); err != nil {
		return err
	}
	a.emitAgentTasksChanged()
	return nil
}

func (a *App) createRunWorktree(project database.Project, run database.AgentRun) (string, error) {
	root, err := agentWorktreeRoot(project.ID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	target := filepath.Join(root, run.ID)
	branch := "agent-workspace/" + run.ID
	output, err := commandOutput(context.Background(), project.CanonicalPath, 30*time.Second,
		"git", "worktree", "add", "-b", branch, target, "HEAD")
	if err != nil {
		return "", fmt.Errorf("create isolated run worktree: %w: %s", err, output)
	}
	return target, nil
}

func (a *App) completeAgentRun(turnID string) {
	a.runMu.Lock()
	reference, ok := a.turnRuns[turnID]
	if ok {
		delete(a.turnRuns, turnID)
	}
	a.runMu.Unlock()
	if !ok {
		return
	}
	_ = a.store.UpdateRun(context.Background(), reference.RunID, "completed", 100, "Run completed", "")
	task, err := a.store.GetTask(context.Background(), reference.TaskID)
	if err != nil {
		return
	}
	runs, err := a.store.ListRuns(context.Background(), reference.TaskID)
	if err != nil {
		return
	}
	if task.Mode == "delivery" {
		for _, run := range runs {
			if run.ID == reference.RunID && run.Role == "planner" {
				_ = a.store.UpdateTaskStatus(context.Background(), reference.TaskID, "waiting-approval")
				timestamp := time.Now().UTC().Format(time.RFC3339Nano)
				_ = a.store.CreateArtifact(context.Background(), database.Artifact{
					ID: newLocalID("plan"), TaskID: reference.TaskID, RunID: reference.RunID,
					Type: "plan", Title: "Implementation plan", Status: "needs-review",
					Content:   "Open the planner thread to review the complete plan, then approve the next workflow stage.",
					CreatedAt: timestamp, UpdatedAt: timestamp,
				})
				a.emitAgentTasksChanged()
				return
			}
		}
		for _, run := range runs {
			if run.Status == "blocked" {
				_ = a.store.UpdateRun(context.Background(), run.ID, "queued", 0, "Previous workflow stage completed", "")
				go a.startEligibleRuns(reference.TaskID)
				a.emitAgentTasksChanged()
				return
			}
		}
	}
	allTerminal := true
	for _, run := range runs {
		if run.Status == "queued" || run.Status == "blocked" || run.Status == "running" {
			allTerminal = false
		}
	}
	if allTerminal {
		_ = a.store.UpdateTaskStatus(context.Background(), reference.TaskID, "completed")
		timestamp := time.Now().UTC().Format(time.RFC3339Nano)
		_ = a.store.CreateArtifact(context.Background(), database.Artifact{
			ID: newLocalID("artifact"), TaskID: reference.TaskID, Type: "verification",
			Title: "Workflow completion", Status: "needs-review",
			Content:   "All configured agent runs reached a terminal state. Review each provider thread and verification evidence before merge.",
			CreatedAt: timestamp, UpdatedAt: timestamp,
		})
	}
	a.emitAgentTasksChanged()
}

func (a *App) emitAgentTasksChanged() {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "agents:tasks-updated", nil)
	}
}

func rolePrompt(role, prompt string) string {
	switch role {
	case "planner":
		return "Act as the planning agent for this task. Analyze only and produce a decision-ready plan; do not implement.\n\nTask:\n" + prompt
	case "reviewer":
		return "Act as an independent reviewer. Inspect the current repository state and task outcome. Report correctness, architecture, security, regressions, and required revisions. Do not modify files.\n\nOriginal task:\n" + prompt
	case "security":
		return "Act as an independent security reviewer. Inspect the implementation for trust-boundary, injection, permission, secret, and destructive-operation risks. Do not modify files.\n\nOriginal task:\n" + prompt
	case "browser-qa":
		return "Act as browser QA. Inspect the requested user flow and produce a verification checklist and findings. Use available browser tooling when configured.\n\nOriginal task:\n" + prompt
	default:
		return "Act as the implementation agent. Implement the task safely, preserve unrelated changes, and run relevant verification.\n\nTask:\n" + prompt
	}
}

func routeProvider(role string, providers []agents.ProviderStatus) string {
	preferred := []string{"codex", "claude", "gemini", "opencode", "ollama"}
	if role == "browser-qa" {
		preferred = []string{"antigravity", "codex", "claude"}
	} else if role == "reviewer" || role == "planner" {
		preferred = []string{"claude", "codex", "gemini", "ollama"}
	}
	for _, id := range preferred {
		if providerAvailable(id, providers) {
			return id
		}
	}
	return preferred[0]
}

func providerAvailable(id string, providers []agents.ProviderStatus) bool {
	for _, provider := range providers {
		if provider.ID == id {
			return provider.Runnable
		}
	}
	return false
}

func newLocalID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func (a *App) requireDatabase() error {
	if a.databaseErr != nil {
		return a.databaseErr
	}
	if a.store == nil || a.projects == nil {
		return errors.New("local database is unavailable")
	}
	return nil
}

func threadView(thread database.Thread) ThreadView {
	return ThreadView{
		ID: thread.ID, ProjectID: thread.ProjectID, Title: thread.Title,
		Status: thread.Status, CreatedAt: thread.CreatedAt, UpdatedAt: thread.UpdatedAt,
	}
}

func validateLoginURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" {
		return errors.New("app-server returned an invalid login URL")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "chatgpt.com" && !strings.HasSuffix(host, ".chatgpt.com") &&
		host != "openai.com" && !strings.HasSuffix(host, ".openai.com") {
		return errors.New("app-server returned an unexpected login host")
	}
	return nil
}

func (a *App) handleProtocolMessage(method string, id, params []byte) {
	if a.ctx == nil {
		return
	}
	emit := func(event string, payload any) {
		runtime.EventsEmit(a.ctx, event, payload)
	}
	var payload any
	if len(params) > 0 {
		if err := json.Unmarshal(params, &payload); err != nil {
			a.logger.Warn("could not decode Codex event", "method", method, "error", err)
			return
		}
	}
	switch method {
	case "account/updated":
		emit("codex:account-updated", payload)
	case "account/rateLimits/updated":
		emit("codex:rate-limits-updated", payload)
	case "account/login/completed":
		emit("codex:login-completed", payload)
	case "thread/started", "thread/name/updated":
		emit("codex:thread-updated", payload)
	case "turn/started":
		emit("codex:turn-started", payload)
	case "turn/completed":
		emit("codex:turn-completed", payload)
		if turnID := nestedString(payload, "turn", "id"); turnID != "" {
			go a.completeAgentRun(turnID)
		}
	case "item/started", "item/updated", "item/completed":
		emit("codex:item", map[string]any{"event": method, "payload": payload})
	case "item/agentMessage/delta":
		emit("codex:agent-message-delta", payload)
	case "item/commandExecution/outputDelta":
		emit("codex:command-output-delta", payload)
	case "turn/diff/updated":
		emit("codex:diff-updated", payload)
	case "item/commandExecution/requestApproval":
		approval := approvalPayload("command", id, payload)
		if details, ok := approval["details"].(map[string]any); ok {
			command, _ := details["command"].(string)
			cwd, _ := details["cwd"].(string)
			details["risk"] = agents.AssessCommand(command, cwd)
		}
		emit("codex:approval-requested", approval)
	case "item/fileChange/requestApproval":
		emit("codex:approval-requested", approvalPayload("file", id, payload))
	case "item/permissions/requestApproval":
		emit("codex:approval-requested", approvalPayload("permissions", id, payload))
	case "item/tool/requestUserInput":
		emit("codex:user-input-requested", map[string]any{
			"requestId": string(id), "details": payload,
		})
	case "mcpServer/elicitation/request":
		emit("codex:elicitation-requested", map[string]any{
			"requestId": string(id), "details": payload,
		})
	}
}

func nestedString(value any, keys ...string) string {
	current := value
	for _, key := range keys {
		object, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = object[key]
	}
	text, _ := current.(string)
	return text
}

func approvalPayload(kind string, id []byte, payload any) map[string]any {
	return map[string]any{
		"kind": kind, "requestId": string(id), "details": payload,
	}
}

func rateLimitWindow(used float64, resetsAt, duration *int64) *RateLimitWindow {
	window := &RateLimitWindow{
		UsedPercent: used, RemainingPercent: max(0, 100-used),
	}
	if resetsAt != nil {
		window.ResetsAt = *resetsAt
	}
	if duration != nil {
		window.WindowDurationMins = *duration
	}
	return window
}

func nullIfEmptyString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func extractUserMessageText(item map[string]any) string {
	content, _ := item["content"].([]any)
	var parts []string
	for _, entry := range content {
		value, ok := entry.(map[string]any)
		if !ok || value["type"] != "text" {
			continue
		}
		if text, ok := value["text"].(string); ok {
			parts = append(parts, text)
		}
	}
	if text, ok := item["text"].(string); ok && text != "" {
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n")
}

func sanitizeFilename(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "codex-chat"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "\x00", "")
	value = replacer.Replace(value)
	if len([]rune(value)) > 80 {
		value = string([]rune(value)[:80])
	}
	return value
}
