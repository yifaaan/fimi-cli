# fimi

A Go-powered AI coding agent that runs in your terminal.

fimi connects to OpenAI-compatible LLM providers and gives the model a rich set of tools — shell commands, file I/O, code search, web lookup — so it can plan, edit, and debug code alongside you in an interactive TUI session.

## Features

- **Interactive shell UI** — rich terminal interface built on Bubble Tea with transcript blocks, grouped tool activity, inline diffs, and approval prompts
- **Built-in tool suite** — bash, read/write/replace/patch files, glob, grep, web search (DuckDuckGo), URL fetch, sub-agent delegation
- **MCP bridge** — connect to external Model Context Protocol servers and expose their tools to the agent
- **Session management** — persistent session history per working directory, with `/continue`, `/resume`, `/rewind`, `/compact` commands
- **Approval system** — every destructive tool call (shell, write, replace, patch) goes through an approval gate; `/yolo` mode skips all prompts
- **Checkpoint & rollback (D-Mail)** — automatic checkpoints before each turn; send a D-Mail to roll back to any checkpoint and inject a message from "the future"
- **ACP server** — expose the agent over JSON-RPC for editor and tool integrations
- **Multiple output modes** — interactive shell (default), plain text, and streaming JSON
- **Agent profiles** — define custom agent behaviors via YAML specs with system prompts, tool lists, and sub-agents

## Quick Start

### Build

```bash
go build -o fimi ./cmd/fimi
```

### Configure

On first run fimi uses built-in defaults. To configure your provider and API key, run `/setup` inside the shell, or edit the config file directly:

```
# Linux / macOS
~/.config/fimi/config.json

# Windows
%AppData%\fimi\config.json
```

Minimal config example:

```json
{
  "default_model": "my-model",
  "models": {
    "my-model": {
      "provider": "my-provider",
      "model": "actual-model-name"
    }
  },
  "providers": {
    "my-provider": {
      "type": "openai",
      "api_key": "sk-...",
      "base_url": "https://api.example.com/v1"
    }
  }
}
```

### Run

```bash
# Start an interactive session
fimi

# Pass an initial prompt
fimi refactor the session loader

# Continue the previous session in this directory
fimi --continue

# Use a specific model
fimi --model fast-model fix the flaky test

# One-shot text output
fimi --output text explain this codebase

# Stream JSON events
fimi --output stream-json summarize the recent commits
```

## CLI Flags

| Flag | Description |
|------|-------------|
| `--continue`, `-C` | Continue the previous session for this work dir |
| `--new-session` | Explicitly start a fresh session |
| `--model <alias>` | Override the configured model for this run |
| `--output <mode>` | Output mode: `shell` (default), `text`, `stream-json` |
| `--yolo` | Skip all tool approval prompts |
| `-h`, `--help` | Show help message |
| `--` | Stop parsing flags; everything after is prompt text |

## Shell Commands

Inside the interactive shell:

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/setup` | Guided configuration wizard |
| `/compact` | Compact conversation history |
| `/rewind` | Roll back to a previous checkpoint |
| `/resume` | Resume a previous session |
| `/clear` | Clear the current transcript |
| `/task` | Manage background tasks |
| `/version` | Show version info |
| `/release-notes` | Show changelog |
| `/reload` | Reload configuration |
| `/init` | Initialize agent config in current directory |

## Agent Profiles

Agent behavior is defined in YAML files. The default agent lives at `agents/default/agent.yaml`:

```yaml
version: 1
agent:
  name: fimi
  system_prompt_path: ./system.md
  system_prompt_args:
    ROLE: coding agent
  tools:
    - agent
    - think
    - set_todo_list
    - bash
    - read_file
    - glob
    - grep
    - write_file
    - replace_file
    - patch_file
    - search_web
    - fetch_url
```

Agents support `extend` (inherit from another profile), `exclude_tools`, and `subagents` for delegation.

## Built-in Tools

| Tool | Description |
|------|-------------|
| `bash` | Execute shell commands (foreground or background) |
| `read_file` | Read file contents with optional offset/limit |
| `write_file` | Create or overwrite files |
| `replace_file` | String replacement in files (single or batch) |
| `patch_file` | Apply unified diff patches |
| `glob` | Find files by pattern |
| `grep` | Search file contents (ripgrep-style) |
| `search_web` | Web search via DuckDuckGo |
| `fetch_url` | Fetch and extract text from URLs |
| `agent` | Delegate work to a sub-agent |
| `think` | Private reasoning (not shown in output) |
| `set_todo_list` | Track task progress |
| `send_dmail` | Roll back to a past checkpoint with a message |

## MCP Integration

fimi can connect to MCP servers and expose their tools to the agent. Configure in `config.json`:

```json
{
  "mcp": {
    "enabled": true,
    "servers": {
      "filesystem": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
      }
    }
  }
}
```

## ACP Server

Run fimi as an ACP (Agent Communication Protocol) JSON-RPC server:

```bash
fimi acp
```

Supports multi-session management, streaming events, and model/mode switching per session.

## Architecture

```
cmd/fimi/           CLI entry point
internal/
  app/              Application assembly, flag parsing, session lifecycle
  agentspec/        YAML agent spec loading and merging
  approval/         Tool approval gate (yolo / auto / per-request)
  acp/              ACP JSON-RPC server
  config/           Configuration loading, validation, persistence
  contextstore/     JSONL conversation history with checkpoints
  dmail/            D-Mail (checkpoint rollback) mechanism
  llm/              LLM client abstraction (OpenAI-compatible streaming)
  mcp/              MCP client and server manager
  runtime/          Agent loop: step execution, retry, event dispatch
  session/          Session metadata and persistence
  tools/            Tool registry, built-in tool implementations
  ui/
    shell/          Interactive TUI (Bubble Tea)
    printui/        One-shot text/JSON output mode
```

## License

This project is proprietary software.
