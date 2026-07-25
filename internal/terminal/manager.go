package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
)

type Event struct {
	SessionID  string `json:"sessionId"`
	ProviderID string `json:"providerId"`
	Data       string `json:"data,omitempty"`
	ExitCode   *int   `json:"exitCode,omitempty"`
	Error      string `json:"error,omitempty"`
}

type SessionInfo struct {
	ID         string `json:"id"`
	ProviderID string `json:"providerId"`
	Status     string `json:"status"`
	StartedAt  string `json:"startedAt"`
}

type session struct {
	info    SessionInfo
	command *exec.Cmd
	pty     *os.File
}

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*session
	emit     func(Event)
}

func NewManager(emit func(Event)) *Manager {
	return &Manager{sessions: make(map[string]*session), emit: emit}
}

func (m *Manager) Start(
	ctx context.Context,
	providerID, executable string,
	args []string,
	cwd string,
	cols, rows uint16,
) (SessionInfo, error) {
	if executable == "" {
		return SessionInfo{}, errors.New("terminal executable is required")
	}
	if cols == 0 {
		cols = 110
	}
	if rows == 0 {
		rows = 32
	}
	id := fmt.Sprintf("console-%d", time.Now().UnixNano())
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = cwd
	command.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	file, err := pty.StartWithSize(command, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return SessionInfo{}, err
	}
	info := SessionInfo{
		ID: id, ProviderID: providerID, Status: "running",
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	entry := &session{info: info, command: command, pty: file}
	m.mu.Lock()
	m.sessions[id] = entry
	m.mu.Unlock()
	go m.read(entry)
	go m.wait(entry)
	return info, nil
}

func (m *Manager) Write(sessionID, data string) error {
	entry, err := m.get(sessionID)
	if err != nil {
		return err
	}
	_, err = io.WriteString(entry.pty, data)
	return err
}

func (m *Manager) Resize(sessionID string, cols, rows uint16) error {
	entry, err := m.get(sessionID)
	if err != nil {
		return err
	}
	if cols == 0 || rows == 0 {
		return nil
	}
	return pty.Setsize(entry.pty, &pty.Winsize{Cols: cols, Rows: rows})
}

func (m *Manager) Stop(sessionID string) error {
	entry, err := m.get(sessionID)
	if err != nil {
		return err
	}
	if entry.command.Process != nil {
		_ = entry.command.Process.Signal(os.Interrupt)
	}
	return entry.pty.Close()
}

func (m *Manager) StopAll() {
	m.mu.RLock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	for _, id := range ids {
		_ = m.Stop(id)
	}
}

func (m *Manager) get(sessionID string) (*session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry := m.sessions[sessionID]
	if entry == nil {
		return nil, errors.New("terminal session not found")
	}
	return entry, nil
}

func (m *Manager) read(entry *session) {
	buffer := make([]byte, 16*1024)
	for {
		count, err := entry.pty.Read(buffer)
		if count > 0 && m.emit != nil {
			m.emit(Event{
				SessionID: entry.info.ID, ProviderID: entry.info.ProviderID,
				Data: string(buffer[:count]),
			})
		}
		if err != nil {
			return
		}
	}
}

func (m *Manager) wait(entry *session) {
	err := entry.command.Wait()
	exitCode := -1
	if entry.command.ProcessState != nil {
		exitCode = entry.command.ProcessState.ExitCode()
	}
	_ = entry.pty.Close()
	m.mu.Lock()
	delete(m.sessions, entry.info.ID)
	m.mu.Unlock()
	event := Event{SessionID: entry.info.ID, ProviderID: entry.info.ProviderID, ExitCode: &exitCode}
	if err != nil && exitCode != 0 {
		event.Error = err.Error()
	}
	if m.emit != nil {
		m.emit(event)
	}
}
