package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CODEX_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var message wireMessage
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			os.Exit(2)
		}
		switch message.Method {
		case "initialize":
			io.WriteString(os.Stdout, "not-json\n")
			io.WriteString(os.Stdout, `{"method":"test/progress","params":{"secret":"not-forwarded"}}`+"\n")
			io.WriteString(os.Stdout, `{"id":`+string(message.ID)+`,"result":{"serverInfo":{"name":"fake"}}}`+"\n")
			io.WriteString(os.Stderr, "fake diagnostic\n")
		case "initialized":
		case "echo":
			io.WriteString(os.Stdout, `{"id":`+string(message.ID)+`,"result":{"ok":true}}`+"\n")
		}
	}
	os.Exit(0)
}

func TestClientHandshakeAndRouting(t *testing.T) {
	os.Setenv("GO_WANT_CODEX_HELPER", "1")
	defer os.Unsetenv("GO_WANT_CODEX_HELPER")

	var mu sync.Mutex
	var activities []Activity
	client := NewClient(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		func(activity Activity) {
			mu.Lock()
			activities = append(activities, activity)
			mu.Unlock()
		},
		nil,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Start(ctx, os.Args[0], ClientInfo{Name: "test", Title: "Test", Version: "0.1.0"}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	var result struct {
		OK bool `json:"ok"`
	}
	if err := client.Call(ctx, "echo", map[string]string{"value": "hello"}, &result); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if !result.OK {
		t.Fatal("Call() did not decode response")
	}

	deadline := time.Now().Add(time.Second)
	for len(client.StderrLines()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if lines := client.StderrLines(); len(lines) == 0 || !strings.Contains(lines[0], "fake diagnostic") {
		t.Fatalf("StderrLines() = %v", lines)
	}

	mu.Lock()
	gotActivities := append([]Activity(nil), activities...)
	mu.Unlock()
	if len(gotActivities) < 2 {
		t.Fatalf("activities = %v, want malformed and notification", gotActivities)
	}
	for _, activity := range gotActivities {
		if strings.Contains(activity.Message, "secret") {
			t.Fatal("raw protocol payload leaked into activity")
		}
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	if err := client.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestParseNumericID(t *testing.T) {
	t.Parallel()
	if id, err := parseNumericID(json.RawMessage("42")); err != nil || id != 42 {
		t.Fatalf("parseNumericID() = %d, %v", id, err)
	}
	if _, err := parseNumericID(json.RawMessage(`"server-id"`)); err == nil {
		t.Fatal("parseNumericID() accepted a string response id")
	}
}
