# Linux Codex Desktop — Development Documentation

> **Status:** Milestones 0–2 usable desktop workspace
> **Target:** Linux-first desktop client for OpenAI Codex  
> **Primary development environment:** CachyOS / Arch Linux  
> **Distribution goal:** Major glibc-based Linux distributions  
> **Initial stack:** Go + Wails v2 + Vue 3 + TypeScript  
> **Codex integration:** `codex app-server` over stdio JSON-RPC  
> **Initial release channel:** GitHub Releases + AppImage + AUR

---

## 1. Project Summary

Linux Codex Desktop is an open-source desktop client that provides a native-feeling graphical interface for Codex on Linux.

The application will not attempt to reimplement the Codex agent, authentication system, sandbox, model communication, or tool execution engine. Instead, it will run the official Codex runtime locally and communicate with:

```bash
codex app-server
```

The app-server exposes authentication, threads, turns, streamed events, approvals, file changes, diffs, account information, and usage limits through a bidirectional JSON-RPC protocol.

The application will initially target Linux, while keeping the codebase portable enough to support Windows and macOS later if useful.

---

## 2. Product Positioning

### Working product description

> An open-source, lightweight Codex workspace built for Linux.

### Primary audience

- Arch Linux and CachyOS users
- Ubuntu, Debian, Fedora, Linux Mint, openSUSE and related users
- Developers who prefer a desktop interface over a terminal-only workflow
- Developers who want multiple Codex threads, visual diffs, approvals and project management
- Linux users currently missing the polished desktop Codex experience

### Initial differentiators

- Linux-first product decisions
- Lower resource usage than an Electron application
- Native host filesystem and process integration
- ChatGPT browser login
- Visual approval workflow
- Unified diff viewer
- Multiple projects and threads
- Git worktree support in later releases
- Open-source and community-friendly packaging

---

## 3. Non-Goals

The first versions will **not**:

- Reimplement Codex internally
- Directly call undocumented OpenAI endpoints
- Read, extract or transmit Codex authentication tokens
- Collect the user's ChatGPT password
- Embed a fake or cloned OpenAI login page
- Build a cloud backend for storing user conversations
- Support every Linux distribution from the first release
- Guarantee compatibility with musl-based distributions such as Alpine Linux
- Provide a custom AI model gateway
- Replace a full IDE
- Attempt feature parity with every official Codex client immediately

The application should remain a local client around the official Codex runtime.

### Current workspace capabilities

- Local projects with collapsible, project-scoped thread trees
- Streaming Codex conversations with back-to-back message queuing
- Model, reasoning-effort and default/plan collaboration controls
- `@` file search, structured file mentions and safe project-local file links
- Command, file, permission, user-input and MCP elicitation workflows
- Primary and secondary account usage-limit visibility
- Local JSON chat export/import
- Unified turn diff and interruption controls

Plan mode and the richer interactive request types use the app-server
experimental API capability. The transport remains local stdio; no experimental
WebSocket transport is enabled.

---

## 4. Technology Decision

## 4.1 Selected stack

| Layer | Technology |
|---|---|
| Desktop framework | Wails v2 |
| Backend | Go |
| Frontend | Vue 3 |
| Frontend language | TypeScript |
| State management | Pinia |
| Styling | Tailwind CSS or plain CSS variables |
| Local persistence | SQLite |
| Codex transport | stdio JSONL |
| Codex protocol | JSON-RPC-like app-server protocol |
| Git operations | Native `git` process initially |
| Linux packaging | AppImage, AUR, `.deb`, `.rpm` |
| CI/CD | GitHub Actions |

## 4.2 Why Go

Go is a suitable backend choice because the application needs to manage:

- Long-running child processes
- Concurrent stdout and stderr readers
- JSON message routing
- Application events
- File watching
- Git commands
- Local SQLite persistence
- Graceful shutdown
- Cross-platform paths
- Browser launching
- Linux desktop integration

Go provides a simple concurrency model, straightforward process control and small native binaries.

## 4.3 Why Wails instead of Electron

Wails allows:

- Go backend bindings
- Vue-based frontend development
- Native application packaging
- Use of the system webview instead of bundling Chromium
- Lower package size and memory usage in typical scenarios
- Direct filesystem and process integration

Electron remains a valid alternative, but it is not required for this project.

## 4.4 Why Wails v2 initially

Wails v2 is selected for the first production implementation because:

- It is the established stable major version.
- Its Linux behavior is better understood.
- It supports Vue and TypeScript.
- It avoids depending on a newer Wails v3 stack during the initial MVP.
- Its Linux builds can use the WebKit2GTK 4.1 build tag where required.

Use:

```bash
wails build -tags webkit2_41
```

for distributions or build environments that require WebKit2GTK 4.1.

The project should periodically reevaluate Wails v3 after its Linux compatibility path and stable release policy are suitable for the supported distribution matrix.

The initial implementation is pinned to Wails v2.13.0 so local and CI builds
use the same toolchain.

---

## 5. High-Level Architecture

```text
┌──────────────────────────────────────────────────────────────┐
│                     Wails Desktop Window                     │
│                                                              │
│  ┌────────────┐  ┌─────────────────────┐  ┌───────────────┐ │
│  │ Projects   │  │ Conversation/Thread │  │ Diff / Files  │ │
│  │ Threads    │  │ Streaming Messages  │  │ Approvals     │ │
│  └────────────┘  └─────────────────────┘  └───────────────┘ │
│                         Vue 3 Frontend                        │
└──────────────────────────────┬───────────────────────────────┘
                               │ Wails bindings/events
┌──────────────────────────────▼───────────────────────────────┐
│                           Go Backend                          │
│                                                              │
│ Process Manager │ RPC Client │ Event Router │ SQLite │ Git   │
└──────────────────────────────┬───────────────────────────────┘
                               │ stdin/stdout JSONL
┌──────────────────────────────▼───────────────────────────────┐
│                     codex app-server                          │
│                                                              │
│ Auth │ Threads │ Turns │ Tools │ Sandbox │ Approvals │ Diff  │
└──────────────────────────────┬───────────────────────────────┘
                               │
                        OpenAI / ChatGPT
```

---

## 6. Codex App Server Integration

## 6.1 Transport decision

Use the default stdio transport for the MVP:

```bash
codex app-server
```

The server communicates through newline-delimited JSON over stdin and stdout.

Reasons:

- No network port is exposed.
- No WebSocket authentication is required.
- The process lifecycle belongs to the desktop app.
- It is appropriate for a local desktop client.
- It avoids relying on experimental WebSocket behavior.

Do not use the experimental WebSocket transport in the initial production release.

## 6.2 Connection lifecycle

The backend must:

1. Locate the `codex` executable.
2. Verify the installed version.
3. Start `codex app-server`.
4. Attach stdin, stdout and stderr pipes.
5. Start a goroutine that scans stdout line by line.
6. Parse every stdout line as JSON.
7. Route responses by request ID.
8. Route notifications by method name.
9. Send `initialize`.
10. Send the `initialized` notification.
11. Query account status.
12. Expose connection state to the Vue frontend.
13. Restart cleanly after crashes where appropriate.
14. Terminate the process during application shutdown.

## 6.3 Initial handshake

Request:

```json
{
  "method": "initialize",
  "id": 1,
  "params": {
    "clientInfo": {
      "name": "linux_codex_desktop",
      "title": "Linux Codex Desktop",
      "version": "0.1.0"
    }
  }
}
```

After the response:

```json
{
  "method": "initialized",
  "params": {}
}
```

No account, thread or turn operation should be sent before initialization completes.

## 6.4 Schema generation

The app-server protocol can evolve between Codex versions. Generate matching schemas from the installed CLI:

```bash
mkdir -p internal/codex/schema

codex app-server generate-json-schema \
  --out internal/codex/schema
```

Optional TypeScript schema generation:

```bash
codex app-server generate-ts \
  --out frontend/src/codex-schema
```

Do not manually assume that every Codex version exposes exactly the same request and event shapes.

The application records the detected Codex version and rejects versions below
the initial `0.145.0` compatibility baseline. Schemas generated by Codex CLI
`0.145.0` are the first tested bundle; newer versions receive a warning and run
in best-effort mode.

---

## 7. Authentication

## 7.1 Preferred login method

The default login method is the Codex-managed ChatGPT browser flow.

The application must not implement its own OAuth server, store raw OpenAI tokens, ask for the user's ChatGPT password or scrape browser sessions.

Flow:

```text
GUI
  ↓ account/login/start
codex app-server
  ↓ authUrl
System browser
  ↓ OpenAI / ChatGPT authentication
Local callback hosted by app-server
  ↓ account/login/completed
GUI updates account state
```

## 7.2 Read account state

After initialization:

```json
{
  "method": "account/read",
  "id": 10,
  "params": {
    "refreshToken": false
  }
}
```

The UI should display:

- Signed-in state
- Authentication mode
- ChatGPT plan type when available
- Account email or display data when supplied
- Rate limit data when available

## 7.3 Start browser login

Request:

```json
{
  "method": "account/login/start",
  "id": 11,
  "params": {
    "type": "chatgpt",
    "useHostedLoginSuccessPage": true,
    "appBrand": "chatgpt"
  }
}
```

Expected response shape:

```json
{
  "id": 11,
  "result": {
    "type": "chatgpt",
    "loginId": "LOGIN_ID",
    "authUrl": "https://chatgpt.com/..."
  }
}
```

Open `authUrl` in the system's default browser.

In Wails v2, use the runtime browser helper from the Go backend rather than embedding the login page in the application webview.

Wait for:

```json
{
  "method": "account/login/completed",
  "params": {
    "loginId": "LOGIN_ID",
    "success": true,
    "error": null
  }
}
```

Then:

```json
{
  "method": "account/updated",
  "params": {
    "authMode": "chatgpt",
    "planType": "plus"
  }
}
```

Plan type values must be treated as server-provided values rather than a fixed local enum unless generated from the current schema.

## 7.4 Device-code fallback

Provide a fallback when browser callback behavior fails:

```json
{
  "method": "account/login/start",
  "id": 12,
  "params": {
    "type": "chatgptDeviceCode"
  }
}
```

The response provides:

- `verificationUrl`
- `userCode`
- `loginId`

The UI should display the code, provide a copy button and open the verification URL in the browser.

## 7.5 API key login

API key login can be added as a secondary advanced option:

```json
{
  "method": "account/login/start",
  "id": 13,
  "params": {
    "type": "apiKey",
    "apiKey": "USER_PROVIDED_KEY"
  }
}
```

Security requirements:

- Do not log the key.
- Do not store it separately in the application database.
- Clear frontend input state immediately after sending.
- Avoid including it in crash reports.
- Explain that API billing is separate from ChatGPT subscription access.

## 7.6 Logout

```json
{
  "method": "account/logout",
  "id": 14,
  "params": {}
}
```

After logout:

- Clear account UI state.
- Do not delete unrelated local project metadata.
- Return to the login or welcome screen.
- Allow a different account to sign in.

---

## 8. Thread and Turn Model

Codex app-server exposes:

- **Thread:** A persistent conversation
- **Turn:** One user-to-agent interaction
- **Item:** A message, command, file change, reasoning item or other event inside a turn

The frontend data model should follow the server model instead of reducing everything to plain chat messages.

## 8.1 Start a thread

Example:

```json
{
  "method": "thread/start",
  "id": 20,
  "params": {
    "cwd": "/home/user/projects/example"
  }
}
```

Store:

- Thread ID
- Project ID
- Working directory
- Display title
- Creation timestamp
- Last activity timestamp
- Archived state

Codex remains the source of truth for thread content. SQLite stores local UI metadata and indexing information.

## 8.2 Resume a thread

Use `thread/resume` when reopening an existing Codex thread.

Do not create a replacement local conversation if the server thread is still available.

## 8.3 Start a turn

```json
{
  "method": "turn/start",
  "id": 21,
  "params": {
    "threadId": "THREAD_ID",
    "input": [
      {
        "type": "text",
        "text": "Inspect the repository and explain its architecture."
      }
    ]
  }
}
```

The UI must remain event-driven. The immediate response does not mean the agent has completed.

## 8.4 Turn lifecycle state

Suggested frontend states:

```text
idle
queued
running
waiting_for_approval
waiting_for_user
completed
failed
interrupted
```

Map these states from actual app-server responses and notifications rather than guessing from message text.

## 8.5 Streaming events

The event router should support, at minimum:

- Thread started and updated notifications
- Turn started and completed notifications
- Agent message streaming deltas
- Item started, updated and completed notifications
- Command execution items
- File change items
- Approval requests
- Turn diff updates
- Error notifications
- Account updates
- Rate limit updates

Unknown methods must not crash the application.

Log unknown methods in development mode and preserve forward compatibility.

---

## 9. Approval Workflow

Approval prompts are a core feature, not an optional visual detail.

The backend should route server-initiated requests to the frontend and keep the request pending until the user responds.

The UI should show:

- Requested command or action
- Working directory
- Explanation, when available
- Risk context
- Affected files, when known
- Approve once
- Approve for session, only if supported
- Reject
- Optional user explanation

Rules:

- Never auto-approve commands in the first release.
- Never silently downgrade an approval policy.
- Clearly distinguish read-only inspection from write or execution requests.
- Keep dangerous actions visually explicit.
- Do not infer approval from unrelated UI clicks.
- Time out or cancel requests safely if the app closes.

Later, allow configurable approval profiles only when they map directly to supported Codex policies.

---

## 10. Diff and File Changes

Use the app-server's turn-level diff notifications when available.

A turn diff update may provide an aggregated unified diff. Render it directly instead of attempting to reconstruct a full diff from separate file events.

Initial diff features:

- File list
- Added, modified and deleted indicators
- Unified diff
- Line numbers
- Added and removed line styling
- Collapse unchanged regions
- Copy patch
- Open file in editor
- Refresh from Git
- Display binary-file notice

Later:

- Side-by-side diff
- Per-hunk accept/revert
- Inline comments to Codex
- Commit selected changes
- Worktree comparisons

Do not implement arbitrary file rollback by overwriting content until the behavior is thoroughly tested. Prefer Git-backed revert operations with explicit confirmation.

---

## 11. Project Management

A project represents a local working directory.

Suggested local project fields:

```text
id
name
path
canonical_path
is_git_repository
default_branch
last_opened_at
created_at
settings_json
```

Project actions:

- Add existing directory
- Remove project from the application
- Open in file manager
- Open in terminal
- Open in configured editor
- Start a new Codex thread
- Show existing threads
- Inspect Git status

Removing a project from the application must never delete the project directory.

---

## 12. Local Database

Use SQLite for local application metadata.

Recommended library:

```text
modernc.org/sqlite
```

This avoids requiring a system SQLite library and reduces CGO-related packaging complexity.

Initial tables:

```sql
CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    path TEXT NOT NULL UNIQUE,
    canonical_path TEXT NOT NULL,
    is_git_repository INTEGER NOT NULL DEFAULT 0,
    default_branch TEXT,
    settings_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    last_opened_at TEXT NOT NULL
);

CREATE TABLE threads (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    title TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    archived_at TEXT,
    FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE TABLE app_settings (
    key TEXT PRIMARY KEY,
    value_json TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);
```

Do not duplicate sensitive Codex credentials in SQLite.

Store application data under the XDG data directory:

```text
$XDG_DATA_HOME/linux-codex-desktop/
```

Fallback:

```text
~/.local/share/linux-codex-desktop/
```

Configuration:

```text
$XDG_CONFIG_HOME/linux-codex-desktop/
```

Cache and logs:

```text
$XDG_CACHE_HOME/linux-codex-desktop/
```

---

## 13. Recommended Repository Structure

```text
linux-codex-desktop/
├── app.go
├── main.go
├── go.mod
├── go.sum
├── wails.json
├── LICENSE
├── README.md
├── DOCUMENTATION.md
├── CONTRIBUTING.md
├── SECURITY.md
├── CHANGELOG.md
├── .gitignore
├── .github/
│   ├── ISSUE_TEMPLATE/
│   ├── workflows/
│   │   ├── test.yml
│   │   ├── release.yml
│   │   └── nightly.yml
│   └── dependabot.yml
├── build/
│   ├── appicon.png
│   ├── linux/
│   │   ├── app.desktop
│   │   ├── app.png
│   │   ├── appimage/
│   │   ├── deb/
│   │   └── rpm/
│   └── bin/
├── cmd/
│   └── diagnostics/
├── internal/
│   ├── app/
│   │   ├── lifecycle.go
│   │   └── errors.go
│   ├── codex/
│   │   ├── client.go
│   │   ├── process.go
│   │   ├── transport.go
│   │   ├── protocol.go
│   │   ├── router.go
│   │   ├── auth.go
│   │   ├── account.go
│   │   ├── threads.go
│   │   ├── turns.go
│   │   ├── approvals.go
│   │   ├── events.go
│   │   ├── version.go
│   │   └── schema/
│   ├── database/
│   │   ├── database.go
│   │   ├── migrations.go
│   │   └── repositories/
│   ├── projects/
│   │   ├── service.go
│   │   ├── paths.go
│   │   └── model.go
│   ├── git/
│   │   ├── client.go
│   │   ├── status.go
│   │   └── diff.go
│   ├── platform/
│   │   ├── browser.go
│   │   ├── editor.go
│   │   ├── filemanager.go
│   │   └── xdg.go
│   └── diagnostics/
│       ├── collector.go
│       └── redact.go
├── frontend/
│   ├── package.json
│   ├── tsconfig.json
│   ├── vite.config.ts
│   └── src/
│       ├── main.ts
│       ├── App.vue
│       ├── assets/
│       ├── components/
│       │   ├── account/
│       │   ├── approvals/
│       │   ├── conversation/
│       │   ├── diff/
│       │   ├── projects/
│       │   └── shared/
│       ├── composables/
│       ├── layouts/
│       ├── router/
│       ├── stores/
│       │   ├── account.ts
│       │   ├── app.ts
│       │   ├── projects.ts
│       │   └── threads.ts
│       ├── types/
│       ├── utils/
│       └── views/
│           ├── WelcomeView.vue
│           ├── LoginView.vue
│           ├── ProjectView.vue
│           ├── SettingsView.vue
│           └── DiagnosticsView.vue
├── packaging/
│   ├── appimage/
│   ├── arch/
│   │   └── PKGBUILD
│   ├── deb/
│   └── rpm/
└── scripts/
    ├── dev.sh
    ├── generate-codex-schema.sh
    ├── package-appimage.sh
    └── release.sh
```

---

## 14. Go RPC Client Design

## 14.1 Core types

```go
package codex

import "encoding/json"

type Request struct {
	Method string `json:"method"`
	ID     int64  `json:"id"`
	Params any    `json:"params,omitempty"`
}

type Notification struct {
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type Response struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}
```

The wire protocol omits the normal `"jsonrpc": "2.0"` field. Do not add it unless the generated schema for the installed version requires it.

## 14.2 Client fields

```go
type Client struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser

	writeMu   sync.Mutex
	nextID    atomic.Int64
	pendingMu sync.Mutex
	pending   map[int64]chan Response

	ctx       context.Context
	cancel    context.CancelFunc

	events    chan Event
	errors    chan error
}
```

## 14.3 Important implementation rules

- Serialize writes with a mutex.
- Use one JSON object per line.
- Set a scanner buffer larger than Go's default scanner limit.
- Do not mix stderr with protocol stdout.
- Never print raw auth payloads to production logs.
- Cancel all pending requests when the process exits.
- Apply request deadlines.
- Protect against malformed JSON.
- Handle unknown notifications.
- Avoid goroutine leaks.
- Use exponential backoff only for safe retryable operations.
- Do not retry turn creation blindly because duplicate turns may be created.

## 14.4 Scanner setup

```go
scanner := bufio.NewScanner(stdout)

buffer := make([]byte, 64*1024)
scanner.Buffer(buffer, 16*1024*1024)
```

The maximum should be revisited after observing large diff and history messages.

## 14.5 Request method

```go
func (c *Client) Call(
	ctx context.Context,
	method string,
	params any,
	result any,
) error {
	id := c.nextID.Add(1)
	responseCh := make(chan Response, 1)

	c.pendingMu.Lock()
	c.pending[id] = responseCh
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	request := Request{
		Method: method,
		ID:     id,
		Params: params,
	}

	if err := c.writeJSON(request); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()

	case response := <-responseCh:
		if response.Error != nil {
			return fmt.Errorf(
				"codex RPC error %d: %s",
				response.Error.Code,
				response.Error.Message,
			)
		}

		if result == nil || len(response.Result) == 0 {
			return nil
		}

		return json.Unmarshal(response.Result, result)
	}
}
```

This is a starting pattern, not final production code.

---

## 15. Wails Backend API

Expose high-level application operations instead of exposing raw JSON-RPC calls to Vue.

Example Wails methods:

```go
func (a *App) GetStartupStatus() StartupStatus
func (a *App) DetectCodex() CodexInstallation
func (a *App) StartCodex() error
func (a *App) GetAccount() (Account, error)
func (a *App) LoginWithChatGPT() (LoginStart, error)
func (a *App) LoginWithDeviceCode() (DeviceCodeLogin, error)
func (a *App) Logout() error
func (a *App) AddProject(path string) (Project, error)
func (a *App) ListProjects() ([]Project, error)
func (a *App) RemoveProject(id string) error
func (a *App) StartThread(projectID string) (Thread, error)
func (a *App) ResumeThread(threadID string) error
func (a *App) SendMessage(threadID string, text string) error
func (a *App) InterruptTurn(threadID string, turnID string) error
func (a *App) RespondToApproval(requestID string, decision ApprovalDecision) error
func (a *App) OpenInEditor(path string) error
func (a *App) OpenInFileManager(path string) error
```

Send streaming data through Wails events:

```text
codex:connection
codex:account-updated
codex:thread-started
codex:thread-updated
codex:turn-started
codex:turn-completed
codex:item-started
codex:item-updated
codex:item-completed
codex:agent-message-delta
codex:diff-updated
codex:approval-requested
codex:error
```

Use stable internal event payloads. Do not expose every raw protocol object directly to the frontend, because this couples the UI to Codex protocol changes.

---

## 16. Frontend Screens

## 16.1 Startup diagnostics

Display:

- Codex found or missing
- Codex version
- Git found or missing
- Database status
- App-server state
- Supported Codex version range
- WebKit/runtime warning if detectable

## 16.2 Welcome/login screen

Actions:

- Continue with ChatGPT
- Use device code
- Use API key
- Open installation instructions when Codex is missing
- Retry detection

## 16.3 Main workspace

Suggested layout:

```text
┌─────────────────────────────────────────────────────────────┐
│ Project selector │ Current thread │ Account │ Settings      │
├──────────────┬────────────────────────────┬─────────────────┤
│ Projects     │ Conversation               │ Changes         │
│              │                            │                 │
│ Threads      │ Agent output               │ File list       │
│              │ Commands                   │ Diff            │
│ New thread   │ Approval cards             │ Git status      │
├──────────────┴────────────────────────────┴─────────────────┤
│ Prompt input                         Send │ Stop            │
└─────────────────────────────────────────────────────────────┘
```

## 16.4 Settings

Initial settings:

- Theme: system, light, dark
- Editor command
- Terminal command
- File manager command
- Codex executable path override
- Default approval policy
- Default sandbox mode, only from supported values
- Diagnostics logging level
- Check for updates
- Telemetry: disabled by default unless explicitly designed later

---

## 17. Linux Development Setup on CachyOS

## 17.1 System packages

Start with:

```bash
sudo pacman -Syu
```

Install development tools:

```bash
sudo pacman -S --needed \
  base-devel \
  git \
  go \
  nodejs \
  npm \
  pkgconf \
  gtk3 \
  webkit2gtk-4.1
```

Package names can change in rolling distributions. Confirm available WebKitGTK packages with:

```bash
pacman -Ss webkit2gtk
```

Verify:

```bash
go version
node --version
npm --version
git --version
```

## 17.2 Install Wails v2

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
```

Ensure Go's binary directory is in `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Persist it in the shell configuration used by CachyOS, for example:

```bash
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
source ~/.zshrc
```

Check dependencies:

```bash
wails doctor
```

## 17.3 Install Codex

Follow the current official Codex installation instructions.

After installation, verify:

```bash
codex --version
codex app-server --help
```

The application should use the executable discovered from:

```bash
command -v codex
```

Allow users to override the executable path in settings.

## 17.4 Create the project

```bash
wails init \
  -n linux-codex-desktop \
  -t vue-ts
```

Then:

```bash
cd linux-codex-desktop
git init
```

Development mode:

```bash
wails dev
```

Production test build:

```bash
wails build -tags webkit2_41
```

---

## 18. First Implementation Milestones

## Milestone 0 — Repository foundation

Deliverables:

- Wails Vue TypeScript project
- Git repository
- License
- README
- Documentation
- Basic GitHub Actions test workflow
- Formatting and linting
- Application icon placeholder

Acceptance criteria:

```bash
wails dev
```

opens a working window on CachyOS.

## Milestone 1 — Codex process connection

Deliverables:

- Codex executable detection
- Version parsing
- App-server child process
- stdin/stdout JSONL transport
- Request/response routing
- Initialization handshake
- Graceful shutdown
- Diagnostics screen

Acceptance criteria:

- Application starts app-server.
- Initialization completes.
- App-server stderr is captured separately.
- Closing the application terminates the child process.
- A malformed protocol line does not crash the GUI.

## Milestone 2 — Account login

Deliverables:

- `account/read`
- ChatGPT browser login
- Browser launching
- Login completion event
- Device-code fallback
- Logout
- Account state UI

Acceptance criteria:

- User can sign in through the browser.
- GUI detects successful login without manual token entry.
- Restarting the application reuses the Codex-managed login.
- User can log out.

## Milestone 3 — Projects

Deliverables:

- SQLite initialization
- Add directory dialog
- Project persistence
- Git repository detection
- Project list
- Open in file manager
- Remove from app

Acceptance criteria:

- Projects survive application restart.
- Removing a project does not modify its files.
- Duplicate canonical paths are rejected.

## Milestone 4 — Threads and messages

Deliverables:

- Start thread
- Resume thread
- Start turn
- Stream agent output
- Basic turn statuses
- Stop/interruption control
- Thread list and local titles

Acceptance criteria:

- User selects a project.
- User starts a thread.
- User sends a prompt.
- Agent output appears incrementally.
- Thread can be reopened after switching projects.

## Milestone 5 — Commands and approvals

Deliverables:

- Command item rendering
- Approval request routing
- Approval dialog/card
- Reject and approve actions
- Pending request cleanup

Acceptance criteria:

- No action requiring approval is silently approved.
- User can inspect the requested command.
- The server receives the selected decision.
- Closing a pending approval does not leave the backend blocked indefinitely.

## Milestone 6 — Diff viewer

Deliverables:

- Turn-level unified diff
- File list
- Syntax-aware or reliable plain diff rendering
- Open file in editor
- Git status refresh

Acceptance criteria:

- Changed files appear during a turn.
- Aggregated diff updates without duplicate lines.
- Large diffs remain usable.
- Unknown/binary diffs fail gracefully.

## Milestone 7 — Alpha packaging

Deliverables:

- AppImage
- AUR `-bin` package
- SHA256 checksums
- GitHub release workflow
- Installation documentation
- Supported distribution matrix

Acceptance criteria:

- AppImage runs on CachyOS.
- AppImage runs on at least Ubuntu 24.04 and Debian 12 test systems.
- AUR package installs and launches.
- Build artifacts are reproducibly named.

---

## 19. MVP Scope

Version `0.1.0-alpha` should contain only:

1. Codex executable detection
2. Codex app-server startup
3. ChatGPT browser login
4. Project directory selection
5. Thread creation and resumption
6. Streaming agent messages
7. Command and tool item display
8. Approval responses
9. Unified diff viewer
10. Local project/thread metadata
11. Basic diagnostics
12. AppImage and AUR release

Do not delay the alpha for:

- Parallel agents
- GitHub integration
- SSH remote mode
- Worktrees
- Plugin marketplace
- Custom themes
- Full IDE editing
- Cloud sync
- Collaboration
- Built-in terminal emulator
- Flatpak
- ARM64

---

## 20. Post-MVP Roadmap

## Version 0.2

- Better thread history
- Search projects and threads
- Account rate-limit display
- Markdown improvements
- Side-by-side diff
- Configurable editor integration
- Better error recovery
- `.deb` package
- `.rpm` package

## Version 0.3

- Git worktrees
- Parallel task dashboard
- Per-project Codex settings
- Thread fork support
- Git commit workflow
- Inline diff comments
- Drag-and-drop images/files if supported by protocol

## Version 0.4

- GitHub issues and pull requests
- Remote SSH workspaces
- Optional Unix socket app-server transport
- Session recovery
- Notification center
- Desktop notifications

## Version 1.0

- Stable protocol compatibility policy
- Automatic update checks
- Signed release artifacts
- Complete diagnostics bundle with redaction
- Mature approval UX
- Tested distribution matrix
- Accessibility review
- Internationalization
- Contributor documentation

---

## 21. Linux Distribution Strategy

The product is developed on Arch/CachyOS but should not contain Arch-specific business logic.

Avoid patterns such as:

```go
if distro == "arch" {
    // normal application behavior
}
```

Distribution-specific logic should be limited to:

- Installation help
- Dependency diagnostics
- Package creation
- Desktop integration edge cases

## 21.1 Initial supported systems

Officially tested:

- CachyOS / current Arch Linux
- Ubuntu 24.04 LTS
- Debian 12
- Fedora: current and previous stable release
- Linux Mint 22 or later
- openSUSE Tumbleweed

Best effort:

- EndeavourOS
- Manjaro
- Ubuntu 22.04 LTS
- openSUSE Leap
- RHEL-compatible desktop systems
- NixOS
- Gentoo

Not initially supported:

- 32-bit systems
- End-of-life distributions
- Alpine Linux and other musl-first systems
- Headless systems without a desktop webview environment
- Unsupported WebKitGTK environments

This matrix must be updated according to real CI and user test results.

## 21.2 Architectures

Initial:

```text
x86_64
```

Planned:

```text
aarch64 / ARM64
```

Avoid claiming ARM64 support until release artifacts are built and tested on actual ARM64 Linux systems.

---

## 22. Packaging Strategy

Release order:

```text
AppImage
  ↓
AUR package
  ↓
Debian package
  ↓
RPM package
  ↓
Flatpak feasibility
```

## 22.1 AppImage

AppImage is the main universal artifact for early releases.

Example:

```text
linux-codex-desktop-0.1.0-x86_64.AppImage
```

Important compatibility rule:

> Build release binaries on a base system no newer than the oldest system the artifact intends to support.

Do not build the universal AppImage only on current Arch Linux. An Arch-built binary may link against a newer glibc version and fail on older Ubuntu or Debian systems.

Recommended approach:

```text
Development: CachyOS
Release build: controlled Ubuntu/Debian container or VM
Testing: CachyOS + Ubuntu + Debian + Fedora
```

The build base must still provide the Wails and WebKitGTK development stack required by the chosen build tags.

## 22.2 AUR

Initial package name:

```text
linux-codex-desktop-bin
```

The AUR package should:

- Download a tagged release artifact
- Verify SHA256 checksum
- Install the binary
- Install `.desktop` entry
- Install icons
- Declare required runtime dependencies
- Avoid building the full frontend on the user's machine

A source-building package can be added later:

```text
linux-codex-desktop
```

## 22.3 Debian package

Artifact:

```text
linux-codex-desktop_0.1.0_amd64.deb
```

Test on:

- Ubuntu 24.04
- Debian 12
- Linux Mint 22

## 22.4 RPM package

Artifact:

```text
linux-codex-desktop-0.1.0-1.x86_64.rpm
```

Test primarily on Fedora before claiming broader RPM compatibility.

## 22.5 Flatpak

Do not prioritize Flatpak for the MVP.

The application needs broad access to:

- Local project directories
- Host `git`
- Host `codex`
- Shell/tool processes
- Editors
- Terminals
- File manager integration

Flatpak sandbox permissions can make these workflows more complex and potentially confusing. Reevaluate after the native package workflow is stable.

---

## 23. Release Artifacts

A tagged release should eventually include:

```text
linux-codex-desktop-0.1.0-x86_64.AppImage
linux-codex-desktop_0.1.0_amd64.deb
linux-codex-desktop-0.1.0-1.x86_64.rpm
linux-codex-desktop-0.1.0-linux-x86_64.tar.gz
SHA256SUMS
SHA256SUMS.sig
source-code.tar.gz
```

Initial alpha may contain only:

```text
AppImage
tar.gz
SHA256SUMS
```

Use semantic versioning:

```text
0.1.0-alpha.1
0.1.0-alpha.2
0.1.0-beta.1
0.1.0
```

---

## 24. GitHub Actions Plan

Workflows:

```text
test.yml
release.yml
nightly.yml
```

## 24.1 Test workflow

Run on pull requests:

- `go test ./...`
- `go vet ./...`
- Frontend type checking
- Frontend linting
- Frontend unit tests
- Build verification
- Generated binding verification

## 24.2 Release workflow

Trigger:

```yaml
on:
  push:
    tags:
      - "v*"
```

Stages:

1. Checkout
2. Install Go
3. Install Node
4. Install Linux build dependencies
5. Install Wails v2
6. Build frontend/backend
7. Package artifacts
8. Calculate checksums
9. Sign artifacts when configured
10. Create GitHub Release
11. Upload artifacts

Do not publish an AUR update until the GitHub release artifact exists and its checksum is final.

---

## 25. Security Model

## 25.1 Trust boundaries

```text
Untrusted:
- Project repository contents
- Agent-generated commands
- Agent-generated file edits
- MCP/plugin output
- External command output

Trusted with caution:
- Local application database
- User-selected settings
- Codex app-server protocol

Highly sensitive:
- Authentication state
- API keys
- Tokens
- Project source code
- Environment variables
- Shell command arguments
```

## 25.2 Mandatory rules

- Never log API keys or tokens.
- Never send project content to a separate developer-owned backend.
- Never expose app-server over a public interface by default.
- Keep stdio transport local for MVP.
- Never enable automatic command approvals by default.
- Clearly display destructive operations.
- Redact environment secrets from diagnostics.
- Avoid shell string concatenation.
- Use argv-based process execution.
- Canonicalize project paths.
- Prevent accidental path deletion.
- Treat repository-provided instructions as untrusted content processed by Codex, not by the GUI itself.
- Avoid loading arbitrary remote content in the application webview.
- Open authentication links only when returned by the local app-server and validate their scheme.

## 25.3 URL validation

Before opening a login URL:

- Require `https`, except the app-server's own localhost callback is handled by app-server.
- Prefer expected OpenAI/ChatGPT hosts.
- Reject `javascript:`, `data:` and file URLs.
- Do not render the authentication page inside the main webview.

## 25.4 Diagnostics redaction

A support bundle may include:

- Application version
- OS and desktop information
- Codex version
- Wails runtime details
- Sanitized logs
- App-server exit status
- Enabled feature flags

It must not include:

- API keys
- Access or refresh tokens
- Full environment variables
- Full user prompts without explicit consent
- Source files
- Private Git remote credentials
- Browser cookies

---

## 26. Error Handling

Create typed error categories:

```text
CodexNotFound
CodexUnsupportedVersion
CodexStartFailed
CodexExited
ProtocolMalformed
ProtocolTimeout
AuthenticationFailed
ProjectNotFound
ProjectPermissionDenied
DatabaseFailure
GitNotFound
GitCommandFailed
UnsupportedPlatform
```

Every visible error should include:

- Human-readable summary
- Suggested next action
- Optional expandable technical details
- Copy diagnostics action

Do not expose raw stack traces as the only user message.

---

## 27. Logging

Use structured logging in Go.

Suggested levels:

```text
error
warn
info
debug
trace
```

Production default:

```text
info
```

Protocol payload logging:

- Disabled by default
- Redacted
- Enabled only in explicit diagnostics mode
- Never logs authentication secrets

Log files should rotate or have a maximum size.

Suggested location:

```text
$XDG_STATE_HOME/linux-codex-desktop/logs/
```

Fallback:

```text
~/.local/state/linux-codex-desktop/logs/
```

---

## 28. Testing Strategy

## 28.1 Unit tests

Test:

- JSON response classification
- Request ID routing
- Notification routing
- Version parsing
- Path canonicalization
- XDG paths
- Database migrations
- URL validation
- Secret redaction
- Git output parsing

## 28.2 Fake app-server

Build a small test process that:

- Reads JSONL from stdin
- Responds to `initialize`
- Emits account events
- Emits streaming message deltas
- Emits approval requests
- Emits diff updates
- Simulates malformed messages
- Simulates process crash
- Simulates delayed responses

This allows deterministic tests without making real model requests.

## 28.3 Integration tests

Test against a real installed Codex version in a controlled repository.

Do not run billable or account-dependent integration tests on every public pull request.

## 28.4 Distribution tests

Minimum release smoke test:

- Launch application
- Detect Codex
- Start app-server
- Open login page
- Add project
- Start thread
- Exit cleanly

Test environments:

- CachyOS or Arch VM
- Ubuntu 24.04 VM/container with desktop runner
- Debian 12 VM
- Fedora VM

---

## 29. Naming and Branding

Use a neutral working name until branding and trademark review are complete.

Avoid implying that the project is an official OpenAI application.

README disclaimer:

> This is an independent open-source client and is not an official OpenAI product. Codex, ChatGPT and OpenAI are trademarks of their respective owner.

Before public release:

- Review OpenAI brand guidelines.
- Avoid official app icon imitation.
- Avoid confusing repository names.
- Avoid using OpenAI logos as the product icon.
- Clearly disclose the dependency on the official Codex runtime.
- Review the Codex repository license and redistribution terms before bundling its binary.

For the first release, prefer requiring an existing Codex installation instead of bundling the Codex executable.

---

## 30. Codex Runtime Distribution Policy

## Initial policy

The application does not bundle Codex.

Startup detection:

```bash
command -v codex
codex --version
```

When missing, show installation guidance and a retry button.

Benefits:

- Simpler licensing position
- Users can update Codex independently
- Smaller application package
- Lower security maintenance burden
- No duplicated runtime download

## Later possibility

Bundling or downloading a pinned Codex runtime can improve onboarding, but it requires:

- License review
- Signature/checksum verification
- Secure update design
- Multiple architecture builds
- Clear runtime version ownership
- Compatibility testing
- Supply-chain policy

Do not add automatic runtime downloading casually.

---

## 31. Compatibility Policy

The desktop client and Codex runtime versions can evolve independently.

Maintain:

```text
Minimum supported Codex version
Maximum tested Codex version
Detected Codex version
Generated schema version
```

Startup behavior:

- Supported version: continue normally.
- Newer untested version: show a warning, allow best-effort use.
- Older incompatible version: block app-server operations and show upgrade instructions.
- Schema generation mismatch: show diagnostics and avoid undefined behavior.

Feature detection is preferable to hardcoded version checks when the app-server exposes capabilities.

---

## 32. Immediate Development Checklist

Complete these in order.

### Day 1

```text
[ ] Create GitHub repository
[ ] Choose working project name
[ ] Install Go, Node, Wails and WebKitGTK dependencies
[ ] Install/verify Codex CLI
[ ] Generate Wails Vue TypeScript project
[ ] Run wails dev
[ ] Add README, LICENSE and DOCUMENTATION.md
[ ] Add Go and frontend formatting
[ ] Commit initial scaffold
```

### Day 2

```text
[ ] Implement codex executable detection
[ ] Implement version command
[ ] Spawn codex app-server
[ ] Separate stdout and stderr
[ ] Implement JSONL writer
[ ] Implement response router
[ ] Send initialize and initialized
[ ] Show connection state in GUI
```

### Day 3

```text
[ ] Implement account/read
[ ] Implement browser login request
[ ] Open authUrl in system browser
[ ] Handle account/login/completed
[ ] Handle account/updated
[ ] Add logout
[ ] Add device-code fallback
```

### Day 4

```text
[ ] Add SQLite
[ ] Add project model and migrations
[ ] Add directory picker
[ ] Persist projects
[ ] Detect Git repositories
[ ] Build project sidebar
```

### Day 5

```text
[ ] Implement thread/start
[ ] Implement turn/start
[ ] Route message events
[ ] Render streaming output
[ ] Add stop button
[ ] Persist thread metadata
```

### Day 6–7

```text
[ ] Render command items
[ ] Implement approval requests
[ ] Render turn diff updates
[ ] Improve errors and diagnostics
[ ] Create first AppImage experiment
[ ] Publish private/pre-release alpha
```

---

## 33. First Commands to Run

```bash
sudo pacman -Syu

sudo pacman -S --needed \
  base-devel \
  git \
  go \
  nodejs \
  npm \
  pkgconf \
  gtk3 \
  webkit2gtk-4.1

go install github.com/wailsapp/wails/v2/cmd/wails@latest

export PATH="$PATH:$(go env GOPATH)/bin"

wails doctor

codex --version
codex app-server --help

wails init \
  -n linux-codex-desktop \
  -t vue-ts

cd linux-codex-desktop

git init

wails dev
```

First commit:

```bash
git add .
git commit -m "chore: initialize Linux Codex desktop project"
```

---

## 34. Definition of Done for the First Public Alpha

The alpha is ready when a new Linux user can:

1. Download the AppImage or install the AUR package.
2. Launch the application.
3. See whether Codex is installed.
4. Sign in through the system browser.
5. Add a local project.
6. Start a Codex thread.
7. Send a prompt.
8. See streamed agent output.
9. Review command approval requests.
10. Review changed files and a unified diff.
11. Close and reopen the app without losing project metadata.
12. Understand that the app is independent and not an official OpenAI product.

The alpha does not need to be perfect. It must be transparent, recoverable, secure by default and useful.

---

## 35. Key Technical References

These references should be checked regularly because the protocol and packaging ecosystem can evolve.

### OpenAI Codex App Server

- https://developers.openai.com/codex/app-server
- https://github.com/openai/codex/tree/main/codex-rs/app-server
- https://github.com/openai/codex

Important current characteristics:

- App-server powers rich Codex clients.
- It supports authentication, thread history, approvals and streamed events.
- The default local transport is stdio JSONL.
- The protocol uses JSON-RPC-style messages without the normal `jsonrpc` header on the wire.
- The client must perform `initialize` and `initialized`.
- Browser and device-code ChatGPT login flows are exposed through account RPC methods.
- Schemas can be generated from the installed CLI version.

### Wails

- https://wails.io/
- https://wails.io/docs/gettingstarted/installation/
- https://wails.io/docs/gettingstarted/building/
- https://wails.io/docs/guides/linux-distro-support/

### AppImage

- https://appimage.org/
- https://docs.appimage.org/
- https://docs.appimage.org/reference/best-practices.html

Build universal artifacts on a sufficiently old supported base system to avoid requiring a glibc version newer than the target distribution provides.

---

## 36. Final Architecture Decision Record

### Decision

Build the first Linux Codex desktop client with:

```text
Go
Wails v2
Vue 3
TypeScript
SQLite
codex app-server
stdio JSONL
AppImage + AUR
```

### Rationale

- Go is suitable for local process orchestration and concurrency.
- Wails provides a lightweight desktop shell around a web frontend.
- Vue matches the developer's existing frontend experience.
- Codex app-server is the intended deep-integration interface.
- Browser login allows the user to authenticate through the Codex-managed ChatGPT flow.
- Stdio avoids exposing a network service.
- AppImage and AUR provide the fastest route to Linux users.
- Cross-distribution compatibility is maintained by avoiding Arch-specific application logic and building release artifacts on an appropriately old base system.

### Revisit when

- Wails v3 has a stable and suitable Linux compatibility story.
- Codex introduces a dedicated stable SDK for Go.
- Codex runtime bundling becomes legally and operationally appropriate.
- Flatpak host integration can support the required developer workflows cleanly.
- ARM64 demand justifies a supported release pipeline.

---

**Start with Milestone 0 and Milestone 1. Do not build the entire product architecture before proving that Go can reliably initialize `codex app-server`, route responses and stream events into the Vue interface.**
