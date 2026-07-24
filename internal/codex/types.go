package codex

import "time"

const (
	MinimumVersion  = "0.145.0"
	ShutdownTimeout = 3 * time.Second
)

type ConnectionState string

const (
	StateDetecting    ConnectionState = "detecting"
	StateMissing      ConnectionState = "missing"
	StateUnsupported  ConnectionState = "unsupported"
	StateStarting     ConnectionState = "starting"
	StateInitializing ConnectionState = "initializing"
	StateReady        ConnectionState = "ready"
	StateStopping     ConnectionState = "stopping"
	StateStopped      ConnectionState = "stopped"
	StateError        ConnectionState = "error"
)

type ErrorCode string

const (
	ErrCodexNotFound           ErrorCode = "CodexNotFound"
	ErrCodexUnsupportedVersion ErrorCode = "CodexUnsupportedVersion"
	ErrCodexStartFailed        ErrorCode = "CodexStartFailed"
	ErrCodexExited             ErrorCode = "CodexExited"
	ErrProtocolMalformed       ErrorCode = "ProtocolMalformed"
	ErrProtocolTimeout         ErrorCode = "ProtocolTimeout"
)

type AppError struct {
	Code      ErrorCode `json:"code"`
	Summary   string    `json:"summary"`
	Detail    string    `json:"detail,omitempty"`
	Retryable bool      `json:"retryable"`
}

func (e *AppError) Error() string {
	if e.Detail == "" {
		return e.Summary
	}
	return e.Summary + ": " + e.Detail
}

type CodexInstallation struct {
	Found           bool   `json:"found"`
	Path            string `json:"path,omitempty"`
	Version         string `json:"version,omitempty"`
	RawVersion      string `json:"rawVersion,omitempty"`
	MinimumVersion  string `json:"minimumVersion"`
	Supported       bool   `json:"supported"`
	NewerThanTested bool   `json:"newerThanTested"`
}

type DependencyStatus struct {
	Name    string `json:"name"`
	Found   bool   `json:"found"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
}

type ConnectionEvent struct {
	State        ConnectionState    `json:"state"`
	Installation *CodexInstallation `json:"installation,omitempty"`
	Error        *AppError          `json:"error,omitempty"`
	Timestamp    string             `json:"timestamp"`
}

type Activity struct {
	Method    string `json:"method,omitempty"`
	Kind      string `json:"kind"`
	Message   string `json:"message,omitempty"`
	Timestamp string `json:"timestamp"`
}

type StartupStatus struct {
	AppName      string             `json:"appName"`
	AppVersion   string             `json:"appVersion"`
	Platform     string             `json:"platform"`
	State        ConnectionState    `json:"state"`
	Installation *CodexInstallation `json:"installation,omitempty"`
	Git          DependencyStatus   `json:"git"`
	LastError    *AppError          `json:"lastError,omitempty"`
	Stderr       []string           `json:"stderr"`
	UpdatedAt    string             `json:"updatedAt"`
}
