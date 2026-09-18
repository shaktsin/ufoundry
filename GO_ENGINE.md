# UFoundry Go engine

The Go engine is the new core of UFoundry: one binary (`ufoundry`) that is both the always-on engine and the CLI. The Mac app and other clients talk to it through the engine protocol described below. It runs alongside the Python app during the migration and shares its `~/.ufoundry` folder and SQLite database.

Status: milestones M0–M2 of the [rewrite plan](https://claude.ai/code/artifact/259caf02-238a-46e2-ad53-ee346d97c407), plus the engine side of M5 (chats, models, complexity, API keys and usage). Connectors, skills, MCP, tasks and agent teams are still served by the Python app.

## Quick start

```sh
make go-build                       # → bin/ufoundry
./bin/ufoundry engine               # run in the foreground (Ctrl-C to stop)
./bin/ufoundry service install      # or: start at login (launchd / systemd --user)

./bin/ufoundry key add --provider claude --label personal   # prompts for the key, then tests it
./bin/ufoundry chat "what's in my workspace?"
./bin/ufoundry chat                 # interactive
```

API keys go to the macOS Keychain (service `com.ufoundry`); on Linux they are kept in `~/.ufoundry/secrets.json` (mode 0600). On first start the engine imports the keys the Python app already has, for any provider with no key yet: `api_key` values in `config.yaml`, `UFOUNDRY_*`/`UMABOT_*` environment variables, `~/.ufoundry/.env`, and the Python app's Keychain entries (service `ufoundry`, or `umabot` from before the rename). You can then remove keys from `config.yaml`.

## CLI

| Command | What it does |
| --- | --- |
| `chat [-t THREAD] [-p PROVIDER] [-m MODEL] [-c LEVEL] [-k KEY] [MESSAGE]` | Send a message, streaming the reply. Asks `[y/N]` for approvals. No message = interactive. |
| `thread list / show / search / rename / pin / unpin / archive / unarchive / delete / fork / export` | Chat history. Old Python sessions appear as threads with ids `legacy-s<N>`. |
| `thread set ID [-p] [-m] [-c] [-k]` | Save a thread's provider, model, complexity and key. |
| `key list / add / test / default / enable / disable / fallback / rotate / delete / budget` | API keys per provider, monthly budgets (`--hard-stop` blocks requests at 100%). |
| `model list [-p P] [--all] / hide / show / price / refresh` | Model catalog: bundled models + models the provider reports, hidden flags, price overrides. |
| `complexity [show] / default LEVEL` | Complexity presets and the default level. |
| `usage [--by credential\|model\|thread\|role\|day] [--days N] [--key ID]` | Tokens and cost. |
| `approvals / approve ID / deny ID` | Answer approvals from another terminal. |
| `status`, `service install\|uninstall\|status`, `version` | Engine management. |

## Model selection and complexity

For each turn the engine resolves provider → model → API key → complexity, first match wins:

1. The turn override (`-p/-m/-c/-k`, or the composer pickers in the app).
2. The thread's saved settings.
3. Defaults: `llm.provider` / `llm.model`, `llm.providers.<p>.default_model`, then the bundled default.

The key is the requested one, else the provider's default key, else its oldest enabled key. With `fallback` enabled on other keys, a rate-limited request is retried with the next one.

| Level | Reasoning | Max tool steps | Output tokens |
| --- | --- | --- | --- |
| quick | off | 5 | 4,096 |
| standard | medium | 15 | 16,384 |
| deep | high | 40 | 32,768 |
| auto (default) | picks quick / standard / deep per message with a local heuristic; the turn records what it picked | | |

Reasoning maps to Claude adaptive thinking + `output_config.effort` (or a thinking budget for Haiku 4.5), OpenAI `reasoning_effort`, and Gemini `thinkingLevel` (or `thinkingBudget` for 2.5). Models without reasoning ignore it.

## Usage and cost

Every model request writes one row to `llm_usage`: key, provider, model, thread, turn, role (`chat`, `title`), input / cached / output / reasoning tokens, cost, latency and status. Cost uses the bundled price table (`internal/models/catalog.json`, list prices checked 2026-09-18) or your override (`ufoundry model price`). Models without a known price cost $0 and show `?`. When a provider reports no usage, tokens are estimated and flagged.

## New config keys

```yaml
runtime:
  engine_ws_port: 8766          # protocol WebSocket (0 = off); separate from the Python gateway's ws_port
  socket_path: ~/.ufoundry/run/engine.sock
policy:
  approval_timeout_minutes: 30  # unanswered approvals count as denied
models:
  default_complexity: auto      # auto | quick | standard | deep
  complexity:                   # optional overrides per level
    deep: {max_tool_steps: 60}
  roles:
    title: {provider: claude, model: claude-haiku-4-5-20251001}
llm:
  providers:
    openai_compatible: {base_url: http://localhost:11434/v1, default_model: llama3.3}
```

## Protocol

JSON-RPC 2.0, one message per line on the Unix socket `~/.ufoundry/run/engine.sock` (0600), or one per text frame on `ws://127.0.0.1:8766/ws` with `Authorization: Bearer <~/.ufoundry/run/token>` (or `?token=`). The token is regenerated at each engine start. Clients must call `initialize` first (`protocolVersion` major must match; `admin: true` to receive and answer approvals).

Types are in `internal/protocol`. Methods:

| Group | Methods |
| --- | --- |
| Session | `initialize`, `engine/status`, `events/subscribe` (`{all: true}` or `{threadIds: [...]}`) |
| Threads | `thread/start`, `thread/list`, `thread/read`, `thread/rename`, `thread/pin`, `thread/archive`, `thread/delete`, `thread/fork`, `thread/search`, `thread/export`, `thread/setSettings` |
| Turns | `turn/start` (`text`, `attachments`, `override`), `turn/interrupt` |
| Approvals | `approval/list`, `approval/respond` |
| Providers, models | `provider/list`, `model/list`, `model/setHidden`, `model/setPrice`, `model/refresh` |
| API keys | `credential/list`, `credential/add`, `credential/test`, `credential/update`, `credential/rotate`, `credential/delete` |
| Usage | `usage/summary`, `usage/setBudget` |
| Complexity | `complexity/getDefaults`, `complexity/setDefaults` |

Notifications: `turn/started`, `turn/completed` (with `usage`), `item/started`, `item/delta`, `item/completed`, `thread/updated`, `approval/request` and `approval/resolved` (admin clients), `usage/budgetWarning`. A client receives thread events for threads it started, read or sent a turn to, or all threads after `events/subscribe {all: true}`.

Approvals are a notification plus `approval/respond` rather than a server-to-client request: the first admin answer wins, later ones get error `-32005`, and everyone gets `approval/resolved`.

## Layout

```
cmd/ufoundry/        CLI + engine entry point
internal/protocol/   protocol types and method names
internal/server/     Unix socket + WebSocket transports (e2e tests live here)
internal/client/     Go protocol client (used by the CLI)
internal/engine/     threads, turns, agent loop, approvals, titles
internal/llm/        Claude, OpenAI(-compatible) and Gemini streaming adapters (plain HTTP)
internal/models/     catalog, prices, cost, complexity presets and Auto classifier
internal/credentials/ API keys, Keychain, budgets, fallback
internal/tools/      built-in tools (file.read/list/write, shell.run) + workspace ACL
internal/policy/     approval policy
internal/store/      SQLite (pure Go, WASM build of SQLite) + migrations
internal/config/     config.yaml loader (reads the Python app's keys)
internal/secrets/    Keychain (macOS) / file store
```

## Building without golang.org access

`go.mod` fetches `golang.org/x/*` from their GitHub mirrors through `replace` directives. Where `golang.org` is reachable you can drop the `replace` block and run `go mod tidy`.
