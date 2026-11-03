# agentbridge

Shared AI agent contract for local-first Go tools.

`agentbridge` answers one question: **how does an application hand work to a local coding agent without knowing that agent's CLI?**

It gives you:

- an agent catalog based on what is installed;
- one request type: agent + model + effort + prompt;
- agent-specific argv construction;
- local model/effort discovery;
- process and HTTP invokers;
- a `mux` presenter for panes, tabs and popups.

Presentation stays in the application. `agentbridge` never draws a TUI.

> Status: `v0.3.0`, experimental. The API is usable but not frozen before `v1.0.0`.

## Install

Library:

```bash
go get github.com/sovengar/agentbridge@v0.1.0
```

CLI:

```bash
go install github.com/sovengar/agentbridge/cmd/agentbridge@v0.1.0
```

## Versioning

Releases use semantic tags. `v0.1.0` is the first published tag; the API may change before `v1.0.0`.

Pin production consumers:

```bash
go get github.com/sovengar/agentbridge@v0.1.0
```

Use `@latest` only for experiments.

## Quick start

```bash
# Which agents are installed?
agentbridge agents

# Which models does pi expose locally?
agentbridge models pi

# Start an interactive agent with a prompt
agentbridge exec --harness pi --model free --effort high --prompt "fix the failing test"

# Inspect the headless argv instead of running it
agentbridge argv --agent pi --model free --effort high -- "fix the failing test"
```

Applications can use the same contract through Go:

```go
req := ai.Request{
    Agent:  ai.Agent{Name: "pi", Binary: "pi"},
    Model:  "free",
    Effort: "high",
    Prompt: "fix the failing test",
    Cwd:    "/src/app",
}

argv := ai.InteractiveArgs(req)
```

## Design

The module separates five concerns:

| Concern | Question | API |
|---|---|---|
| Catalog | Which agents exist? | `Detect`, `Builtins` |
| Request | What does the user want? | `Request`, `Agent` |
| Invocation | How is the agent called? | `CapabilitiesFor`, `InteractiveArgs`, `ModelArgs`, `HeadlessArgs` |
| Model source | Where do models/efforts come from? | `OptionsFor` |
| Execution | Which transport runs it? | `Invoker`, `ProcessInvoker`, `APIInvoker` |

The caller owns presentation:

- an in-TUI overlay;
- a detached process;
- a tmux/herdr/tuios pane or tab;
- an inline API result.

## Agent support

`CapabilitiesFor` describes how one agent expresses model, effort and prompt.

| Agent | Model | Effort | Headless | Notes |
|---|---|---|---|---|
| `pi` | `--model` | `--thinking` | `pi -p` | Prompt is positional |
| `codex` | `--model` | `-c model_reasoning_effort=…` | `codex exec` | Prompt is positional |
| `opencode` | `--model` (headless only) | `#variant` on model | `opencode run` | Interactive TUI has no model/effort flags; use `/model` |
| `hermes` | `-m` | `--reasoning` | `hermes chat -q` | Interactive start is `hermes chat` (no initial prompt); one-shot prompts use `-z` |
| `jcode` | `-m` | unsupported | `jcode run` | Interactive TUI takes no initial prompt; model switches with `/model` |
| unknown | none | none | none | Falls back to positional prompt |

Unsupported flags are omitted. The module never invents an option.

## Model sources

The picker reads options through `ModelSource`, so the origin stays replaceable.

```go
// ModelSource answers where the picker reads models and efforts.
type ModelSource interface {
	Options(ctx context.Context, agent Agent) (Options, error)
}
```

| Source | Origin |
|---|---|
| `LocalSource{}` | The agent's catalog on disk (the table below) |
| `APISource{Endpoint: ...}` | `GET {endpoint}/models` on an OpenAI-compatible API |
| `StaticSource{Data: ...}` | One fixed list from config or tests |
| `CompositeSource{Sources: ...}` | The first source with a non-empty answer |
| `CachedSource{Source: ..., TTL: ...}` | Memoizes another source; a failed refresh keeps the old value |
| `Gateway{Default: ..., ByAgent: ...}` | Routes by agent name, else the default source |

`LocalSource` never fails the caller. A missing or malformed catalog returns empty options.

| Agent | Local file |
|---|---|
| `pi` | `~/.pi/agent/models-store.json` |
| `codex` | `~/.codex/models_cache.json` |
| `opencode` | `~/.local/state/opencode/model.json` |
| `hermes` | `~/.hermes/provider_models_cache.json`, `~/.hermes/ollama_cloud_models_cache.json` |
| `jcode` | `~/.config/jcode/openai_model_catalog_cache.json`, `~/.jcode/cache/*_models.json` |

The picker should still allow a free-text value, so a stale catalog never blocks a handoff.

```go
var source ai.ModelSource = ai.Gateway{
	Default: ai.CachedSource{
		Source: ai.CompositeSource{Sources: []ai.ModelSource{
			ai.APISource{Endpoint: "https://api.openai.com/v1", APIKey: key, ModelPrefix: "openai/"},
			ai.LocalSource{},
		}},
		TTL: time.Minute,
	},
}
options, err := source.Options(ctx, agent)
```

From the CLI, `agentbridge models <agent> --endpoint URL` lists the provider instead of disk:

```sh
agentbridge models opencode --endpoint https://api.openai.com/v1 --api-key "$OPENAI_API_KEY" --prefix openai/
```

## Invokers

`ProcessInvoker` runs the headless argv without a shell and captures stdout. stderr is diagnostics, not output.

```go
var invoker ai.Invoker = ai.ProcessInvoker{}
res, err := invoker.Invoke(ctx, req)
```

`APIInvoker` calls an OpenAI-compatible `/chat/completions` endpoint.

```go
api := ai.APIInvoker{
    Endpoint: "https://api.openai.com/v1",
    APIKey:   os.Getenv("OPENAI_API_KEY"),
    Model:    "gpt-5",
    System:   "You are a coding assistant.",
}
```

Both accept injectable seams (`ProcessRunner`, `HTTPClient`) for tests.

Anthropic's Messages API is not implemented yet.

## `mux` presenter

`scripts/mux` is a terminal-multiplexer abstraction. It is not AI-specific: it decides **where** a command appears, never **what** the command is.

```bash
mux [options] -- COMMAND [ARG...]

--backend auto|tmux|herdr|tuios|kitty|wezterm|none
--layout auto|pane|tab|popup|overlay
--direction right|down
--working-dir DIR
--container NAME
--label NAME
--session-id ID
--width 80%
--height 80%
--no-focus
--dry-run
```

Backend detection:

1. `HERDR_ENV=1` → `herdr`
2. `TUIOS_ENV=1` → `tuios`
3. `KITTY_LISTEN_ON` → `kitty`
4. `WEZTERM_PANE` → `wezterm`
5. `TMUX` → `tmux`
6. otherwise `none`

| Backend | pane | tab | popup / overlay |
|---|---|---|---|
| tmux | yes | yes | yes (tmux >= 3.2) |
| herdr | yes | yes | yes: split + zoom; a true popup is config-only |
| tuios | yes | yes | yes |
| kitty | yes | yes | yes (`--type=overlay`) |
| wezterm | yes | yes | no, degrades to a pane |
| none | inline | inline | inline |

`--container` is a workspace (herdr) or session (tmux/tuios), not a path. `--working-dir` is the real process directory. Popup layouts ignore the container.

## Handoff contract

A consumer keeps **one** command for every agent. The agent-specific argv is
built by `agentbridge exec`; the presenter only decides where the process
appears. Adding an agent must not touch the consumer's config.

The contract is these five inputs and no others:

| Input | Meaning |
|---|---|
| `--harness` | display name from `agentbridge agents` |
| `--model` | model id from `agentbridge models <harness>`; empty omits the flag |
| `--effort` | effort level; empty, or unsupported by the agent, omits the flag |
| `--prompt-file` | private file with the exact prompt bytes; `--prompt` exists for tests |
| `--cwd` | working directory of the process |

Duties of `agentbridge exec`: resolve capabilities, omit empty or unsupported
flags, never invent one, keep the prompt as a single argv element (or a file),
and `exec` so the TTY, signals and exit code survive. It knows nothing about
panes, tabs or overlays.

The consumer expands its own placeholders into this one command:

```toml
[handoff]
command = "mux --container {{container}} --layout tab --working-dir {{working_dir}} --session-id {{session_id}} -- agentbridge exec --harness {{harness}} --model {{model}} --effort {{effort}} --prompt-file {{prompt_file}}"
```

A complete consumer example lives in [`examples/tsk-config.toml`](examples/tsk-config.toml).

### A real Herdr popup is configuration, not a plugin

Herdr's `popup` layout is expressed in the user's own `config.toml` under
`[[keys.command]]` with `type = "popup"`. `mux` and `agentbridge` add no plugin
and no integration module: they only build the command the keybinding runs.

```toml
[[keys.command]]
key = "alt+a"
type = "popup"
command = "mux --layout popup --working-dir %d -- agentbridge exec --harness pi --prompt-file %f"
description = "Ask AI about this project"
```

A backend that has no native popup degrades to a pane: `mux` prints the layout
it really used, so the degradation is visible instead of silent.

## CLI reference

```text
agentbridge agents
agentbridge models <agent>
agentbridge argv --agent <agent> [--model M] [--effort E] [--cwd DIR] [--] <prompt>
agentbridge exec --harness <agent> [--model M] [--effort E] [--binary BIN]
        [--cwd DIR] (--prompt-file FILE | --prompt TEXT) [--dry-run]
```

`agentbridge exec` uses `syscall.Exec`, so it preserves the TTY, signals and exit code.

## Development

```bash
go test ./...
go vet ./...
./scripts/mux_test.sh
```

The `mux` test uses fake backends; it never touches a real multiplexer.

## Limitations

- Interactive prompt placement is agent-specific: `opencode`, `hermes` and `jcode` take no model/effort flags in the TUI or no initial prompt at all, so a handoff to them starts a bare interactive session and the prompt is lost. Use the headless mode (`opencode run`, `hermes chat -q`, `jcode run`) to deliver a prompt.
- Anthropic Messages API is not implemented.
- Model catalogs are local, undocumented files; a changed format returns an empty list.
- `mux` is POSIX `sh`; Windows is out of scope.

## License

MIT. See [LICENSE](LICENSE).
