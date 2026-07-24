package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/kaplanware/codex-gui/internal/codex"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx     context.Context
	manager *codex.Manager
}

func NewApp() *App {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return &App{manager: codex.NewManager(logger)}
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
