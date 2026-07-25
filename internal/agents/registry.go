package agents

import (
	"context"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type Capability string

const (
	CapabilityChat       Capability = "chat"
	CapabilityPlan       Capability = "plan"
	CapabilityImplement  Capability = "implement"
	CapabilityReview     Capability = "review"
	CapabilityBrowser    Capability = "browser"
	CapabilityLocal      Capability = "local"
	CapabilityStructured Capability = "structured-events"
)

type ProviderStatus struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Kind         string       `json:"kind"`
	Installed    bool         `json:"installed"`
	Runnable     bool         `json:"runnable"`
	Executable   string       `json:"executable,omitempty"`
	Version      string       `json:"version,omitempty"`
	Status       string       `json:"status"`
	Capabilities []Capability `json:"capabilities"`
}

type Definition struct {
	ID           string
	Name         string
	Kind         string
	Executables  []string
	VersionArgs  []string
	Capabilities []Capability
}

type Registry struct {
	mu          sync.RWMutex
	definitions []Definition
	statuses    []ProviderStatus
}

func NewRegistry() *Registry {
	return &Registry{definitions: []Definition{
		{ID: "codex", Name: "Codex", Kind: "cloud-cli", Executables: []string{"codex"}, VersionArgs: []string{"--version"}, Capabilities: []Capability{CapabilityChat, CapabilityPlan, CapabilityImplement, CapabilityReview, CapabilityStructured}},
		{ID: "claude", Name: "Claude Code", Kind: "cloud-cli", Executables: []string{"claude"}, VersionArgs: []string{"--version"}, Capabilities: []Capability{CapabilityChat, CapabilityPlan, CapabilityImplement, CapabilityReview}},
		{ID: "antigravity", Name: "Antigravity", Kind: "cloud-cli", Executables: []string{"antigravity"}, VersionArgs: []string{"--version"}, Capabilities: []Capability{CapabilityPlan, CapabilityImplement, CapabilityReview, CapabilityBrowser}},
		{ID: "gemini", Name: "Gemini CLI", Kind: "cloud-cli", Executables: []string{"gemini"}, VersionArgs: []string{"--version"}, Capabilities: []Capability{CapabilityChat, CapabilityPlan, CapabilityImplement, CapabilityReview}},
		{ID: "opencode", Name: "OpenCode", Kind: "cloud-cli", Executables: []string{"opencode"}, VersionArgs: []string{"--version"}, Capabilities: []Capability{CapabilityChat, CapabilityPlan, CapabilityImplement}},
		{ID: "ollama", Name: "Ollama", Kind: "local", Executables: []string{"ollama"}, VersionArgs: []string{"--version"}, Capabilities: []Capability{CapabilityChat, CapabilityReview, CapabilityLocal}},
	}}
}

func (r *Registry) Detect(ctx context.Context) []ProviderStatus {
	statuses := make([]ProviderStatus, 0, len(r.definitions))
	for _, definition := range r.definitions {
		status := ProviderStatus{
			ID: definition.ID, Name: definition.Name, Kind: definition.Kind,
			Status: "not-installed", Capabilities: definition.Capabilities,
		}
		for _, executable := range definition.Executables {
			path, err := exec.LookPath(executable)
			if err != nil {
				continue
			}
			status.Installed = true
			status.Executable = path
			status.Runnable = definition.ID == "codex"
			if status.Runnable {
				status.Status = "available"
			} else {
				status.Status = "adapter-pending"
			}
			commandCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			output, err := exec.CommandContext(commandCtx, path, definition.VersionArgs...).CombinedOutput()
			cancel()
			if err == nil {
				status.Version = firstLine(string(output))
			}
			break
		}
		statuses = append(statuses, status)
	}
	sort.SliceStable(statuses, func(i, j int) bool {
		if statuses[i].Installed != statuses[j].Installed {
			return statuses[i].Installed
		}
		return statuses[i].Name < statuses[j].Name
	})
	r.mu.Lock()
	r.statuses = statuses
	r.mu.Unlock()
	return append([]ProviderStatus(nil), statuses...)
}

func (r *Registry) Statuses() []ProviderStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]ProviderStatus(nil), r.statuses...)
}

func (r *Registry) Platform() string { return runtime.GOOS + "/" + runtime.GOARCH }

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.IndexByte(value, '\n'); index >= 0 {
		return value[:index]
	}
	return value
}
