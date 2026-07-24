package codex

import (
	"context"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Emitter func(name string, payload any)

type Manager struct {
	operationMu  sync.Mutex
	mu           sync.RWMutex
	logger       *slog.Logger
	detector     *Detector
	emitter      Emitter
	state        ConnectionState
	installation *CodexInstallation
	lastError    *AppError
	client       *Client
	updatedAt    time.Time
}

func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		logger: logger, detector: NewDetector(), state: StateStopped, updatedAt: time.Now(),
	}
}

func (m *Manager) SetEmitter(emitter Emitter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emitter = emitter
}

func (m *Manager) Detect(ctx context.Context) (CodexInstallation, error) {
	m.transition(StateDetecting, nil)
	installation, err := m.detector.Detect(ctx)
	m.mu.Lock()
	m.installation = &installation
	m.mu.Unlock()
	if err != nil {
		appErr := asAppError(err)
		if appErr.Code == ErrCodexNotFound {
			m.transition(StateMissing, appErr)
		} else {
			m.transition(StateUnsupported, appErr)
		}
		return installation, err
	}
	m.transition(StateStopped, nil)
	return installation, nil
}

func (m *Manager) Start(ctx context.Context) error {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	return m.start(ctx)
}

func (m *Manager) start(ctx context.Context) error {
	m.mu.RLock()
	if m.state == StateStarting || m.state == StateInitializing || m.state == StateReady {
		m.mu.RUnlock()
		return nil
	}
	m.mu.RUnlock()

	installation, err := m.Detect(ctx)
	if err != nil {
		return err
	}
	m.transition(StateStarting, nil)

	var client *Client
	client = NewClient(m.logger, m.emitActivity, func(exitErr error) {
		m.handleExit(clientReference(m, client), exitErr)
	})
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()
	m.transition(StateInitializing, nil)
	err = client.Start(ctx, installation.Path, ClientInfo{
		Name: "linux_codex_desktop", Title: "Linux Codex Desktop", Version: "0.1.0-alpha.1",
	})
	if err != nil {
		appErr := &AppError{
			Code: ErrCodexStartFailed, Summary: "Could not initialize Codex app-server",
			Detail: redactSecrets(err.Error()), Retryable: true,
		}
		m.transition(StateError, appErr)
		return appErr
	}
	m.transition(StateReady, nil)
	return nil
}

// clientReference exists to make the captured pointer explicit for exit identity checks.
func clientReference(_ *Manager, client *Client) *Client { return client }

func (m *Manager) Restart(ctx context.Context) error {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	if err := m.stop(ctx); err != nil && ctx.Err() != nil {
		return err
	}
	return m.start(ctx)
}

func (m *Manager) Stop(ctx context.Context) error {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	return m.stop(ctx)
}

func (m *Manager) stop(ctx context.Context) error {
	m.mu.RLock()
	client := m.client
	state := m.state
	m.mu.RUnlock()
	if client == nil || state == StateStopped {
		return nil
	}
	m.transition(StateStopping, nil)
	err := client.Stop(ctx)
	m.mu.Lock()
	if m.client == client {
		m.client = nil
	}
	m.mu.Unlock()
	m.transition(StateStopped, nil)
	return err
}

func (m *Manager) Status() StartupStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := StartupStatus{
		AppName: "Linux Codex Desktop", AppVersion: "0.1.0-alpha.1",
		Platform: runtime.GOOS + "/" + runtime.GOARCH, State: m.state,
		Installation: m.installation, LastError: m.lastError,
		UpdatedAt: m.updatedAt.UTC().Format(time.RFC3339Nano),
		Git:       detectGit(),
	}
	if m.client != nil {
		status.Stderr = m.client.StderrLines()
	}
	return status
}

func (m *Manager) transition(state ConnectionState, appErr *AppError) {
	m.mu.Lock()
	m.state = state
	m.lastError = appErr
	m.updatedAt = time.Now()
	emitter := m.emitter
	installation := m.installation
	event := ConnectionEvent{
		State: state, Installation: installation, Error: appErr,
		Timestamp: m.updatedAt.UTC().Format(time.RFC3339Nano),
	}
	m.mu.Unlock()
	if emitter != nil {
		emitter("codex:connection", event)
	}
}

func (m *Manager) emitActivity(activity Activity) {
	m.mu.RLock()
	emitter := m.emitter
	m.mu.RUnlock()
	if emitter != nil {
		emitter("codex:activity", activity)
	}
}

func (m *Manager) handleExit(client *Client, err error) {
	m.mu.RLock()
	isCurrent := m.client == client
	state := m.state
	m.mu.RUnlock()
	if !isCurrent || state == StateStopping || state == StateStopped {
		return
	}
	detail := "The process ended unexpectedly."
	if err != nil {
		detail = redactSecrets(err.Error())
	}
	m.transition(StateError, &AppError{
		Code: ErrCodexExited, Summary: "Codex app-server exited", Detail: detail, Retryable: true,
	})
}

func asAppError(err error) *AppError {
	if appErr, ok := err.(*AppError); ok {
		return appErr
	}
	return &AppError{
		Code: ErrCodexStartFailed, Summary: "Codex operation failed",
		Detail: redactSecrets(err.Error()), Retryable: true,
	}
}

func detectGit() DependencyStatus {
	status := DependencyStatus{Name: "Git"}
	path, err := exec.LookPath("git")
	if err != nil {
		return status
	}
	status.Found = true
	status.Path = path
	if output, err := exec.Command(path, "--version").Output(); err == nil {
		status.Version = strings.TrimSpace(strings.TrimPrefix(string(output), "git version "))
	}
	return status
}
