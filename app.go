package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
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
				"developer_instructions": nil,
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

func (a *App) SearchProjectFiles(projectID, query string) ([]FileMatch, error) {
	if err := a.requireDatabase(); err != nil {
		return nil, err
	}
	project, err := a.projects.Get(context.Background(), projectID)
	if err != nil {
		return nil, err
	}
	var response struct {
		Files []struct {
			Root     string `json:"root"`
			Path     string `json:"path"`
			FileName string `json:"file_name"`
			Score    int64  `json:"score"`
		} `json:"files"`
	}
	if err := a.call("fuzzyFileSearch", map[string]any{
		"query": query, "roots": []string{project.CanonicalPath}, "cancellationToken": nil,
	}, &response); err != nil {
		return nil, err
	}
	matches := make([]FileMatch, 0, len(response.Files))
	for _, file := range response.Files {
		relative, err := filepath.Rel(project.CanonicalPath, file.Path)
		if err != nil || strings.HasPrefix(relative, "..") {
			continue
		}
		matches = append(matches, FileMatch{
			Name: file.FileName, Path: file.Path, RelativePath: filepath.ToSlash(relative), Score: file.Score,
		})
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
