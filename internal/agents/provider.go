package agents

import (
	"context"
	"errors"
)

var ErrCapabilityUnavailable = errors.New("provider capability is unavailable")

type SessionOptions struct {
	ProjectPath string
	Model       string
	Effort      string
	Mode        string
	Role        string
}

type Session struct {
	ID         string
	ProviderID string
	Status     string
}

type Input struct {
	Text     string
	Mentions []string
}

type Event struct {
	ProviderID string
	SessionID  string
	Type       string
	Payload    any
}

// Provider is the stable boundary used by orchestration. Provider-specific
// transports are responsible for translating their native protocol into Event.
type Provider interface {
	ID() string
	Name() string
	Detect(context.Context) ProviderStatus
	Authenticate(context.Context) error
	StartSession(context.Context, SessionOptions) (Session, error)
	Send(context.Context, string, Input) error
	Interrupt(context.Context, string) error
	Events() <-chan Event
}
