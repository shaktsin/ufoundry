# UFoundry Go engine

The Go engine is the new core of UFoundry: one binary (`ufoundry`) that is both the always-on engine and the CLI. The Mac app and other clients talk to it through the engine protocol described below. It runs alongside the Python app during the migration and shares its `~/.ufoundry` folder and SQLite database.

Status: milestones M0–M3 of the [rewrite plan](https://claude.ai/code/artifact/259caf02-238a-46e2-ad53-ee346d97c407) (engine, agent loop, skills, MCP, scheduled tasks), plus the engine side of M5 (chats, models, complexity, API keys and usage). Connectors (Telegram, Gmail, Discord), Google Workspace tools and agent teams are still served by the Python app.

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
| `task list [--all] / add / cancel ID / run ID / runs ID` | Scheduled tasks. `add` takes `--at 2026-09-20T09:00`, `--daily 09:00`, `--weekly mon@09:00`, `--hourly 15` or `--cron "0 9 * * mon-fri"`, plus `--tz`, `-p/-m/-c`. |
| `skill list / show NAME / install PATH_OR_GIT_URL / remove NAME` | Skills in `./skills`, `~/.ufoundry/skills` and `skill_dirs`. |
| `mcp [list] / mcp restart NAME` | MCP servers from `mcp_servers` and their status. |
| `tools` | Every tool the agent can call, with its source. |
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

## Skills, MCP and scheduled tasks

**Skills** use the same `SKILL.md` format as the Python app and are found in `./skills`, `~/.ufoundry/skills` and any `skill_dirs`. The system prompt lists each skill's name and description; the agent reads full instructions with `skill.get_instructions` and runs declared scripts with `skill.run_script`, or passes `skill: <name>` to `shell.run` to use the skill's environment. Scripts get `{"input": <args>, "config": <skills.<name>.config>}` on stdin (the `skill-template` contract), run in the skill folder with its `.venv` (created with `uv` when available, reusing venvs the Python app made), `extra_path`, `env` and `SKILL_DIR`, and never inherit the engine's own API keys. `risk_level` in `SKILL.md` (default yellow) decides approvals.

**MCP servers** from `mcp_servers` (stdio or Streamable HTTP, same keys as the Python app) start in the background; their tools appear as `mcp_<server>_<tool>`. Risk comes from the server's `risk_level` if set, otherwise from the tool's annotations: read-only is green, destructive is red, anything else yellow. A server that exits is restarted on the next call.

**Scheduled tasks** share the `tasks` table with the Python app. Each run becomes a turn in the task's own thread (so you can read its history and cost), with the task's provider/model/complexity. Approvals work as in chat. The agent can create, list and cancel tasks itself (`task.create`, `task.list`, `task.cancel`). Differences from the Python app: cron schedules are new (the Python app ignores them), and a failed one-time task is not retried in a loop. Both schedulers lease tasks before running them, so running both apps never runs a task twice.

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
skills:
  weather-fetch:
    config: {api_key_env: WEATHER_KEY}   # passed to scripts as "config"
mcp_servers:
  - name: github
    command: npx
    args: ["-y", "@modelcontextprotocol/server-github"]
    env_vars: [GITHUB_TOKEN]            # forwarded from the engine's environment
    risk_level: yellow                  # optional override for every tool on this server
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
| Tasks | `task/list`, `task/create`, `task/cancel`, `task/runNow`, `task/runs` |
| Skills | `skill/list`, `skill/get`, `skill/install`, `skill/remove` |
| MCP and tools | `mcp/list`, `mcp/restart`, `tool/list` |

Notifications: `turn/started`, `turn/completed` (with `usage`), `item/started`, `item/delta`, `item/completed`, `thread/updated`, `approval/request`, `approval/resolved`, `usage/budgetWarning` and `task/updated` (admin clients). A client receives thread events for threads it started, read or sent a turn to, or all threads after `events/subscribe {all: true}`.

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
internal/tools/      tool registry, built-in tools (file.read/list/write, shell.run), workspace ACL
internal/skills/     SKILL.md loader, runtimes/venvs, skill tools, install/remove
internal/mcp/        MCP client (stdio + Streamable HTTP) and tool bridge
internal/tasks/      schedules (incl. cron), scheduler loop, task tools
internal/procutil/   subprocess cleanup (process groups, timeouts)
internal/policy/     approval policy
internal/store/      SQLite (pure Go, WASM build of SQLite) + migrations
internal/config/     config.yaml loader (reads the Python app's keys)
internal/secrets/    Keychain (macOS) / file store
```

## Building without golang.org access

`go.mod` fetches `golang.org/x/*` from their GitHub mirrors through `replace` directives. Where `golang.org` is reachable you can drop the `replace` block and run `go mod tidy`.
