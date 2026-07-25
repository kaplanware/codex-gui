# Agent Workspace

The open-source command center for coding agents on Linux.

Agent Workspace is a local control plane for planning, running, reviewing, and
verifying coding-agent work. Codex, Claude Code, Gemini CLI, OpenCode, and
Ollama can be selected directly in Workspace; Mission Control coordinates
parallel provider-independent delivery and review workflows.

> This is an independent open-source client and is not an official OpenAI
> product. Codex, ChatGPT, and OpenAI are trademarks of their respective owner.

## Current status

The local workspace and orchestration foundation is implemented:

- Wails v2 and Vue 3/TypeScript application foundation
- Codex executable discovery and version compatibility checks
- `codex app-server` process lifecycle
- JSONL request/response and notification routing
- `initialize` / `initialized` handshake
- Sanitized Wails connection and activity events
- Startup diagnostics interface
- Codex-managed ChatGPT browser and device-code login
- Persistent local projects and thread metadata in SQLite
- Project directory picker and Git detection
- Thread creation, resumption, and history loading
- Streaming messages, command output, and turn state
- Command/file approval prompts with once/session/reject decisions
- Live unified turn diff viewer
- Provider registry and capability detection
- Agent Mission Control with persistent tasks, runs, and artifacts
- Sequential planner → approval → implementer → reviewer delivery workflows
- Parallel jury runs isolated in per-agent Git worktrees
- Codex browser login plus interactive Claude Code, Gemini CLI, and OpenCode authentication
- Headless Claude Code, Gemini CLI, and OpenCode execution adapters
- Shared `TASK.md` / append-only `MAILBOX.jsonl` coordination with live handoffs
- Smart role routing across installed providers
- Preflight checks, production guard, command risk assessment, and snapshots
- Context Inspector with token estimates
- Verification artifacts and human-reviewed project memory

See the [Agent Workspace roadmap](docs/AGENT_WORKSPACE_ROADMAP.md) and
[development specification](docs/DOCUMENTATION.md).

## Requirements

- Go 1.24 or newer
- Node.js 22 or newer and npm
- Wails CLI v2.13.0
- GTK3 and WebKit2GTK 4.1 development packages
- Git
- Codex CLI 0.145.0 or newer

On Arch Linux or CachyOS:

```bash
sudo pacman -S --needed base-devel git go nodejs npm pkgconf gtk3 webkit2gtk-4.1
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
```

## Development

Install frontend dependencies:

```bash
cd frontend
npm install
cd ..
```

Run the desktop application:

```bash
wails dev -tags webkit2_41
```

Run validation:

```bash
go test -race ./...
go vet ./...
cd frontend
npm run lint
npm run typecheck
npm test
npm run build
```

Build the application:

```bash
wails build -tags webkit2_41
```

## Protocol schema

The stable app-server JSON schema is pinned to Codex CLI 0.145.0. Regenerate it
with the matching CLI:

```bash
scripts/generate-codex-schema.sh
```

The application rejects older Codex versions and warns while allowing newer
versions for best-effort forward compatibility.

## License

[MIT](LICENSE)
