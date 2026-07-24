package codex

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

func TestManagerSerializesConcurrentStarts(t *testing.T) {
	os.Setenv("GO_WANT_CODEX_HELPER", "1")
	defer os.Unsetenv("GO_WANT_CODEX_HELPER")

	manager := NewManager(slog.New(slog.NewTextHandler(io.Discard, nil)))
	manager.detector = &Detector{runner: fakeRunner{
		path: os.Args[0], output: "codex-cli " + MinimumVersion,
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wait sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs <- manager.Start(ctx)
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
	}
	if got := manager.Status().State; got != StateReady {
		t.Fatalf("Status().State = %s, want %s", got, StateReady)
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	if err := manager.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if got := manager.Status().State; got != StateStopped {
		t.Fatalf("Status().State = %s, want %s", got, StateStopped)
	}
}
