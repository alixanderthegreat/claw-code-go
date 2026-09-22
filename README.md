# claw-code-go

<p align="center">
  <img src="assets/claw-code-go.png" alt="claw-code-go" width="360" />
</p>

<p align="center">
  <strong>A Go port of Claude Code — Anthropic's agentic CLI coding assistant</strong><br/>
  Fast. Extensible. Built for terminals.
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat-square&logo=go" />
  <img src="https://img.shields.io/badge/Claude-claude--sonnet--4-blueviolet?style=flat-square&logo=anthropic" />
  <img src="https://img.shields.io/badge/MCP-supported-green?style=flat-square" />
  <img src="https://img.shields.io/badge/Multi--provider-Anthropic%20%7C%20OpenAI-orange?style=flat-square" />
  <img src="https://img.shields.io/badge/TUI-Bubble%20Tea-pink?style=flat-square" />
</p>

---

<p align="center">
  <img src="assets/screenshot.png" alt="claw-code-go terminal screenshot" width="600" />
</p>

---

## What is this?

`claw-code-go` is a full Go reimplementation of [Claude Code](https://docs.anthropic.com/en/docs/claude-code) — Anthropic's agentic coding assistant. It runs in your terminal, understands your codebase, calls tools, writes and edits files, searches the web, and works autonomously until the job is done.

**Why Go?**
- Single static binary — no Node.js runtime required
- Faster startup, lower memory footprint
- Easier to embed, cross-compile, and distribute

---

## Features

### 🤖 Agentic Loop
Full tool-use conversation loop: Claude reasons, calls tools, reads the results, and keeps going until the task is complete or it hits an `end_turn`.

### 🎨 Bubble Tea TUI
Rich terminal UI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) — streaming output with a spinner, styled message history, syntax-aware theming, and a clean session view.

### 🛠 Tool Suite

| Tool | Description |
|------|-------------|
| `bash` | Execute shell commands (30s timeout, sandboxed) |
| `read_file` | Read files from disk |
| `write_file` | Write/create files |
| `file_edit` | Surgical text replacements in existing files |
| `glob` | Find files matching a glob pattern |
| `grep` | Search across files with regex |
| `web_fetch` | Fetch and parse a URL |
| `web_search` | Search the web (Brave API) |
| `todo_write` | Track and persist task lists |

### 🔐 OAuth + Multi-Provider Auth
Native OAuth flow for Anthropic accounts. Credentials are stored securely and refreshed automatically. OpenAI is scaffolded alongside Anthropic — swap providers via config.

### 🌐 MCP — Model Context Protocol
Full MCP integration: connect external tool servers over stdio or SSE, register their tools, and let Claude call them seamlessly alongside built-in tools.

### 🔒 Permissions & Safety
Fine-grained permission system controls what Claude can do — bash execution, file writes, network access — with configurable modes (`default`, `accept-edits`, `bypass`, `plan`) and rule-based overrides. `bypass` mode still honors an explicit deny rule rather than truly allowing everything unconditionally — see [Server Mode](#server-mode-dispatch-surface) for why that matters.

### 📦 Context Assembly
On every turn, claw-code-go automatically injects:
- Git repo state (branch, diff summary, recent commits)
- Memory directory files (`.claw-code/memory/`)
- System info (OS, hostname, working directory)

### 💾 Session Persistence & History
Sessions are saved as JSON files in `~/.claw-code/sessions/`. Resume any previous session by ID. The TUI includes a built-in session history browser.

### 📊 Token Usage & Cost Tracking
Every session tracks input/output tokens per turn. Estimated USD cost is shown at the end of each session for all known models (Claude 3/4 + GPT-4o family).

### ⚡ Context Compaction
Automatic conversation compaction keeps context windows manageable on long sessions — summarising older turns without losing important state.

### 🧠 Memory Directory
Drop markdown files into `.claw-code/memory/` and they're injected into every conversation automatically — persistent instructions, project context, preferences.

---

## Prerequisites

- **Go 1.24+**
- `ANTHROPIC_API_KEY` environment variable, or run the TUI once and use `/login` (see [Login](#login))

---

## Install

```sh
git clone https://github.com/alixanderthegreat/claw-code-go
cd claw-code-go
go build -o claw-code-go ./cmd/claw-code-go
```

Or install directly:

```sh
go install github.com/alixanderthegreat/claw-code-go/cmd/claw-code-go@latest
```

---

## Usage

### Interactive REPL (default)

```sh
./claw-code-go
```

### One-shot mode

```sh
export ANTHROPIC_API_KEY=sk-ant-...
./claw-code-go --prompt "Refactor the auth package to use interfaces"
```

### Login

There's no `--login` CLI flag - authenticate from inside the TUI itself:

```sh
./claw-code-go
# then, in the TUI:
/login
```

`/login` walks you through picking a provider - Anthropic (OAuth via browser, or an API key entered by hand) or OpenAI (API key only) - and stores the result for next time. If you start the TUI with no credentials on hand, it tells you this directly rather than failing silently.

For a local/custom OpenAI-compatible endpoint instead (self-hosted models, a gateway, etc.), skip `/login` entirely and set `base_url`/`api_key` in `~/.config/claw-code-go/config.json` - see [Local/Custom Models](#localcustom-models) below.

### Local/Custom Models

Point claw-code-go at any OpenAI-compatible endpoint - a self-hosted model server, a gateway, whatever - by writing `~/.config/claw-code-go/config.json`:

```json
{
  "base_url": "http://localhost:8080/v1",
  "api_key": "not-needed",
  "model": "your-model-id",
  "context_window": 81920
}
```

`context_window` matters more than it looks: it's what compaction is actually measured against (not `max_tokens`, the output cap), and the built-in default is sized for real Claude models. Leave it unset against a small local model and compaction's own trigger point can end up *larger* than the model's real context window - meaning it never fires in time. Set it to your model's real total context length.

When `base_url` is set, claw-code-go switches to an OpenAI-shaped request format automatically (`detectProvider`) - no separate flag needed.

### Server Mode (dispatch surface)

`serve` starts an HTTP server for dispatching tasks programmatically instead of driving the TUI by hand - built for scripted/agent-driven use (a CI step, another tool, a bot), not humans:

```sh
./claw-code-go serve --addr 127.0.0.1:4097
```

- `POST /dispatch` - `{"task": "...", "dir": "/path/to/project"}`, returns `{"id": "..."}` immediately, never blocks
- `GET /session/{id}` - poll for status: `running`, `done`, `failed`, or `interrupted` (a deliberately distinct state from `failed` - see below), plus the session id and message count once known
- `POST /session/{id}/interrupt` - stop a running dispatch (sends SIGTERM; the dispatched process saves its session and exits cleanly rather than being killed outright)
- `GET /metrics` - Prometheus text-exposition format: active dispatch count, totals by terminal status, duration sum/count

**How it actually works**: each dispatch spawns claw-code-go itself as a fresh child process (`-prompt <task> -session-dir <isolated dir> -permission-mode bypass`, with the working directory set to `dir`) rather than juggling multiple sessions inside one long-running process. That gives each dispatch real OS-level directory isolation for free - no shared state, no risk of one task's tool calls reaching another's files.

**Two things worth knowing if you're dispatching unattended, not just driving it by hand:**
- Every dispatch denies `web_fetch`/`web_search` automatically (via `CLAW_CODE_BLOCKED_TOOLS`, no config needed) - `bypass` mode otherwise auto-allows everything, including bash, which is exactly the point of dispatching, but outbound network tools are the one thing this deliberately holds back regardless of that setting.
- Every dispatch also gets a path-boundary check (`CLAW_CODE_DISPATCH_BOUNDARY`) refusing `read_file`/`write_file`/`file_edit` calls that resolve outside its own working directory - defense in depth on top of the OS-level isolation above. This does **not** extend to `bash` - a shell command string isn't a structured path, so `cd .. && rm file` isn't something a per-argument check can catch. Scope `dir` to exactly the project you mean, not a parent directory holding other things you care about.

**Security**: `serve` has no authentication at all, and every dispatch runs with every tool auto-allowed. `--addr` defaults to `127.0.0.1` (loopback) for exactly this reason - widen it (e.g. `0.0.0.0:4097`, to let something like Prometheus reach it) only when you mean to, understanding that a bare host process bound wider than loopback has no container/network isolation containing that exposure the way a Docker-internal service would.

### Resume a session

```sh
./claw-code-go --session <session-id>
```

---

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--prompt` | — | Single prompt (one-shot mode) - runs synchronously and exits when done |
| `--model` | `claude-sonnet-4-20250514` | Override the model to use |
| `--session` | — | Resume a saved session by ID |
| `--session-dir` | `~/.claw-code/sessions` | Directory for session files |
| `--permission-mode` | `default` | `default` (ask when needed), `accept-edits` (auto-allow read/edit tools, still ask for bash), `bypass` (allow everything, never ask - see [Server Mode](#server-mode-dispatch-surface)), `plan` (describe only, never execute) |

There's no `--login` or `--provider` flag - see [Login](#login) above. `--repl` is currently declared but not wired to anything (a no-op); the TUI is already the default whenever `--prompt` isn't set.

### Subcommands

Separate from the flags above - these run once and exit, no TUI involved:

| Subcommand | Description |
|------------|-------------|
| `dump-manifests [--src <dir>] [--json]` | List tools, slash commands, and the source manifest |
| `bootstrap-plan [--json]` | Print the ordered startup phase plan |
| `print-system-prompt [--cwd] [--date]` | Render the full system prompt as it would actually be sent |
| `resume-session <file> [commands...]` | Replay a saved session file |
| `serve [--addr] [--session-root]` | Start the dispatch server - see below |

---

## Slash Commands (REPL)

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/login` | Authenticate - pick a provider, OAuth or API key |
| `/clear` | Clear the current session |
| `/session-list` | Browse saved sessions |
| `/model <name>` | Switch model mid-session |
| `/compact` | Manually trigger context compaction |
| `/cost` | Show current session token usage and estimated cost |
| `/exit` or `/quit` | Exit the REPL |

---

## Environment Variables

| Variable | Description |
|----------|-------------|
| `ANTHROPIC_API_KEY` | Your Anthropic API key |
| `ANTHROPIC_MODEL` | Override the default model |
| `ANTHROPIC_BASE_URL` | Override the Anthropic API base URL |
| `OPENAI_API_KEY` | OpenAI API key |
| `OPENAI_BASE_URL` | Override the OpenAI-compatible base URL - setting either this or `ANTHROPIC_BASE_URL` switches to the OpenAI-shaped request format (see [Local/Custom Models](#localcustom-models)) |
| `CLAW_CONTEXT_WINDOW` | The model's real total context length in tokens - env var equivalent of `config.json`'s `context_window`, same reason it matters (compaction's trigger point) |
| `CLAW_CODE_BLOCKED_TOOLS` | Comma-separated tool names to explicitly deny, even in `bypass` mode - what `serve` sets per dispatch, not usually something you set by hand |
| `BRAVE_API_KEY` | Powers the `web_search` tool |
| `CLAUDE_MCP_SERVERS` | JSON array of MCP server configs, layered alongside any configured via settings files |
| `CLAUDE_CODE_USE_BEDROCK` / `CLAUDE_CODE_USE_VERTEX` / `CLAUDE_CODE_USE_FOUNDRY` | Set to `1` to route through AWS Bedrock / Google Vertex AI / Azure AI Foundry instead |

---

## Project Structure

```
claw-code-go/
├── cmd/claw-code-go/        # CLI entry point
└── internal/
    ├── api/                 # Multi-provider API clients (Anthropic, OpenAI, Bedrock, Vertex, Foundry) + SSE streaming, types
    ├── auth/                # OAuth flow, credential storage, token refresh
    ├── commands/            # Slash command registry
    ├── compat/              # Upstream TS source parity manifest + diagnostic subcommands
    ├── config/              # Global config file (~/.config/claw-code-go/config.json)
    ├── context/             # Context assembly (git, memory, sysinfo)
    ├── mcp/                 # Model Context Protocol client (stdio + SSE)
    ├── permissions/         # Permission enforcement & rule engine
    ├── runtime/             # Agentic conversation loop, config, session persistence
    ├── serve/               # HTTP dispatch server (see Server Mode) - process-per-dispatch, /metrics
    ├── tools/               # Built-in tool implementations
    ├── tui/                 # Bubble Tea TUI (model, styles, theme, history)
    └── usage/               # Token tracking and cost estimation
```

---

## Roadmap

- [x] Phase 1 — Foundation: API client, basic conversation loop, core tools
- [x] Phase 2 — TUI: Bubble Tea UI, streaming, spinner, slash commands
- [x] Phase 3 — OAuth + multi-provider scaffolding
- [x] Phase 4 — MCP (Model Context Protocol) integration
- [x] Phase 5 — Permissions & Safety
- [x] Phase 6 — Context compaction
- [x] Phase 7 — Multi-provider login UX
- [x] Phase 8 — Compat harness modes
- [x] Phase 9 — Full TUI foundation (themes, history view)
- [x] Phase 10 — Core tool expansion (file_edit, web_fetch, web_search, todo_write)
- [x] Phase 11 — Permissions + config system
- [x] Phase 12 — Context assembly + memory directory
- [x] Phase 13 — Cost tracking + session history UI
- [ ] Phase 14 — LSP integration
- [ ] Phase 15 — Plugin/extension system
- [x] Phase 16 — Dispatch server (`serve` mode): HTTP API, Prometheus metrics, distinct interrupted/failed/done states

---

## Contributing

PRs welcome. Run `go build ./...` and `go vet ./...` before submitting.

---

## License

MIT
