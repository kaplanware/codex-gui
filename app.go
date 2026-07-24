package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaplanware/codex-gui/internal/codex"
	"github.com/kaplanware/codex-gui/internal/database"
	"github.com/kaplanware/codex-gui/internal/projects"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx         context.Context
	manager     *codex.Manager
	store       *database.Store
	projects    *projects.Service
	databaseErr error
	logger      *slog.Logger
}

func NewApp() *App {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	store, err := database.Open()
	app := &App{manager: codex.NewManager(logger), store: store, databaseErr: err, logger: logger}
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
		if err := a.manager.Start(context.Background()); err != nil {
			slog.Debug("automatic Codex connection failed", "error", err)
		}
	}()
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
	var response struct {
		Thread struct {
			ID        string `json:"id"`
			CreatedAt int64  `json:"createdAt"`
			UpdatedAt int64  `json:"updatedAt"`
		} `json:"thread"`
	}
	err = a.call("thread/start", map[string]any{
		"cwd": project.CanonicalPath, "approvalPolicy": "on-request", "sandbox": "workspace-write",
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
	if err := a.call("thread/resume", map[string]any{
		"threadId": threadID, "cwd": project.CanonicalPath,
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
	err = a.call("turn/start", map[string]any{
		"threadId": threadID,
		"input": []map[string]any{{
			"type": "text", "text": text, "text_elements": []any{},
		}},
	}, &response)
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
	case "command":
		return a.manager.Respond(decision.RequestID, map[string]any{"decision": decision.Decision})
	case "file":
		return a.manager.Respond(decision.RequestID, map[string]any{"decision": decision.Decision})
	default:
		return errors.New("unsupported approval kind")
	}
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
	case "account/login/completed":
		emit("codex:login-completed", payload)
	case "thread/started", "thread/name/updated":
		emit("codex:thread-updated", payload)
	case "turn/started":
		emit("codex:turn-started", payload)
	case "turn/completed":
		emit("codex:turn-completed", payload)
	case "item/started", "item/updated", "item/completed":
		emit("codex:item", map[string]any{"event": method, "payload": payload})
	case "item/agentMessage/delta":
		emit("codex:agent-message-delta", payload)
	case "item/commandExecution/outputDelta":
		emit("codex:command-output-delta", payload)
	case "turn/diff/updated":
		emit("codex:diff-updated", payload)
	case "item/commandExecution/requestApproval":
		emit("codex:approval-requested", approvalPayload("command", id, payload))
	case "item/fileChange/requestApproval":
		emit("codex:approval-requested", approvalPayload("file", id, payload))
	}
}

func approvalPayload(kind string, id []byte, payload any) map[string]any {
	return map[string]any{
		"kind": kind, "requestId": string(id), "details": payload,
	}
}
