# Linux Codex Desktop

An open-source, lightweight Codex workspace built for Linux.

Linux Codex Desktop is a native desktop host for the official Codex runtime. It
uses Go and Wails for process and desktop integration, Vue 3 for the interface,
and communicates with `codex app-server` over local stdio JSONL.

> This is an independent open-source client and is not an official OpenAI
> product. Codex, ChatGPT, and OpenAI are trademarks of their respective owner.

## Current status

Milestones 0 and 1 are implemented:

- Wails v2 and Vue 3/TypeScript application foundation
- Codex executable discovery and version compatibility checks
- `codex app-server` process lifecycle
- JSONL request/response and notification routing
- `initialize` / `initialized` handshake
- Sanitized Wails connection and activity events
- Startup diagnostics interface

Authentication, projects, threads, approvals, diffs, SQLite persistence, and
release packaging are planned but not part of this initial slice. See
[the development specification](docs/DOCUMENTATION.md).

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
