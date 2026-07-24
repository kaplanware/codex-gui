package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxProtocolMessage = 16 * 1024 * 1024
	requestTimeout     = 10 * time.Second
	maxStderrLines     = 100
)

type rpcResponse struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcRequest struct {
	Method string `json:"method"`
	ID     int64  `json:"id"`
	Params any    `json:"params,omitempty"`
}

type rpcNotification struct {
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type wireMessage struct {
	Method string          `json:"method,omitempty"`
	ID     json.RawMessage `json:"id,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

type Client struct {
	logger *slog.Logger
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	writeMu sync.Mutex
	nextID  atomic.Int64

	pendingMu sync.Mutex
	pending   map[int64]chan rpcResponse

	stderrMu    sync.Mutex
	stderrLines []string

	done     chan struct{}
	exitOnce sync.Once
	stopOnce sync.Once

	onActivity func(Activity)
	onMessage  func(method string, id []byte, params []byte)
	onExit     func(error)
}

func NewClient(
	logger *slog.Logger,
	onActivity func(Activity),
	onMessage func(method string, id []byte, params []byte),
	onExit func(error),
) *Client {
	return &Client{
		logger: logger, pending: make(map[int64]chan rpcResponse), done: make(chan struct{}),
		onActivity: onActivity, onMessage: onMessage, onExit: onExit,
	}
}

func (c *Client) Start(ctx context.Context, executable string, info ClientInfo) error {
	c.cmd = exec.Command(executable, "app-server")
	configureProcessGroup(c.cmd)

	var err error
	if c.stdin, err = c.cmd.StdinPipe(); err != nil {
		return err
	}
	if c.stdout, err = c.cmd.StdoutPipe(); err != nil {
		return err
	}
	if c.stderr, err = c.cmd.StderrPipe(); err != nil {
		return err
	}
	if err = c.cmd.Start(); err != nil {
		return err
	}

	go c.readLoop()
	go c.stderrLoop()
	go c.waitLoop()

	handshakeCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var result json.RawMessage
	if err := c.Call(handshakeCtx, "initialize", map[string]any{"clientInfo": info}, &result); err != nil {
		c.stopAfterStartFailure()
		return fmt.Errorf("initialize: %w", err)
	}
	if err := c.Notify("initialized", map[string]any{}); err != nil {
		c.stopAfterStartFailure()
		return fmt.Errorf("initialized: %w", err)
	}
	return nil
}

func (c *Client) stopAfterStartFailure() {
	ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()
	_ = c.Stop(ctx)
}

func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	id := c.nextID.Add(1)
	responseCh := make(chan rpcResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = responseCh
	c.pendingMu.Unlock()
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	if err := c.writeJSON(rpcRequest{Method: method, ID: id, Params: params}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return &AppError{Code: ErrProtocolTimeout, Summary: "Codex request timed out", Detail: method, Retryable: true}
		}
		return ctx.Err()
	case <-c.done:
		return &AppError{Code: ErrCodexExited, Summary: "Codex app-server exited", Retryable: true}
	case response := <-responseCh:
		if response.Error != nil {
			return fmt.Errorf("Codex RPC error %d: %s", response.Error.Code, response.Error.Message)
		}
		if result == nil || len(response.Result) == 0 {
			return nil
		}
		return json.Unmarshal(response.Result, result)
	}
}

func (c *Client) Notify(method string, params any) error {
	return c.writeJSON(rpcNotification{Method: method, Params: params})
}

func (c *Client) Respond(id json.RawMessage, result any) error {
	if len(id) == 0 || !json.Valid(id) {
		return errors.New("invalid server request id")
	}
	var resultValue any = result
	return c.writeJSON(struct {
		ID     json.RawMessage `json:"id"`
		Result any             `json:"result"`
	}{ID: id, Result: resultValue})
}

func (c *Client) writeJSON(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.stdin.Write(payload)
	return err
}

func (c *Client) readLoop() {
	scanner := bufio.NewScanner(c.stdout)
	scanner.Buffer(make([]byte, 64*1024), maxProtocolMessage)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		var message wireMessage
		if err := json.Unmarshal(line, &message); err != nil {
			c.activity(Activity{Kind: "malformed", Message: "Ignored malformed app-server output", Timestamp: timestamp()})
			c.logger.Warn("ignored malformed app-server protocol line", "error", err)
			continue
		}
		if message.Method != "" {
			kind := "notification"
			if len(message.ID) > 0 && string(message.ID) != "null" {
				kind = "server-request"
			}
			c.activity(Activity{Kind: kind, Method: message.Method, Timestamp: timestamp()})
			if c.onMessage != nil {
				c.onMessage(
					message.Method,
					append(json.RawMessage(nil), message.ID...),
					append(json.RawMessage(nil), message.Params...),
				)
			}
			continue
		}
		id, err := parseNumericID(message.ID)
		if err != nil {
			c.activity(Activity{Kind: "malformed", Message: "Ignored response with invalid request ID", Timestamp: timestamp()})
			continue
		}
		c.pendingMu.Lock()
		responseCh := c.pending[id]
		c.pendingMu.Unlock()
		if responseCh != nil {
			responseCh <- rpcResponse{ID: message.ID, Result: message.Result, Error: message.Error}
		}
	}
	if err := scanner.Err(); err != nil {
		c.logger.Warn("app-server stdout reader stopped", "error", err)
	}
}

func parseNumericID(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 {
		return 0, errors.New("missing id")
	}
	return strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
}

func (c *Client) stderrLoop() {
	scanner := bufio.NewScanner(c.stderr)
	scanner.Buffer(make([]byte, 16*1024), 1024*1024)
	for scanner.Scan() {
		line := redactSecrets(scanner.Text())
		c.stderrMu.Lock()
		c.stderrLines = append(c.stderrLines, line)
		if len(c.stderrLines) > maxStderrLines {
			c.stderrLines = c.stderrLines[len(c.stderrLines)-maxStderrLines:]
		}
		c.stderrMu.Unlock()
		c.logger.Warn("codex app-server stderr", "message", line)
	}
}

func (c *Client) waitLoop() {
	err := c.cmd.Wait()
	c.exitOnce.Do(func() {
		close(c.done)
		if c.onExit != nil {
			c.onExit(err)
		}
	})
}

func (c *Client) Stop(ctx context.Context) error {
	var signalErr error
	c.stopOnce.Do(func() {
		if c.stdin != nil {
			_ = c.stdin.Close()
		}
		if c.cmd != nil && c.cmd.Process != nil {
			signalErr = terminateProcessGroup(c.cmd)
		}
	})
	select {
	case <-c.done:
		return signalErr
	case <-ctx.Done():
		if c.cmd != nil && c.cmd.Process != nil {
			_ = killProcessGroup(c.cmd)
		}
		return ctx.Err()
	}
}

func (c *Client) StderrLines() []string {
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	return append([]string(nil), c.stderrLines...)
}

func (c *Client) activity(activity Activity) {
	if c.onActivity != nil {
		c.onActivity(activity)
	}
}

func timestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
