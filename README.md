# ai

Shared AI agent contract for local-first Go tools.

`ai` answers one question: **how does an application hand work to a local coding agent without knowing that agent's CLI?**

It gives you:

- an agent catalog based on what is installed;
- one request type: agent + model + effort + prompt;
- agent-specific argv construction;
- local model/effort discovery;
- process and HTTP invokers;
- a `mux` presenter for panes, tabs and popups.

Presentation stays in the application. `ai` never draws a TUI.

> Status: `v0.1.0`, experimental. The API is usable but not frozen before `v1.0.0`.

## Install

Library:

```bash
go get github.com/sovengar/ai@v0.1.0
```

CLI:

```bash
go install github.com/sovengar/ai/cmd/ai@v0.1.0
```

## Versioning

Releases use semantic tags. `v0.1.0` is the first published tag; the API may change before `v1.0.0`.

Pin production consumers:

```bash
go get github.com/sovengar/ai@v0.1.0
```

Use `@latest` only for experiments.

## Quick start

```bash
# Which agents are installed?
ai agents

# Which models does pi expose locally?
ai models pi

# Start an interactive agent with a prompt
ai exec --harness pi --model free --effort high --prompt "fix the failing test"

# Inspect the headless argv instead of running it
ai argv --agent pi --model free --effort high -- "fix the failing test"
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
| `hermes` | `-m` | `--reasoning` | `hermes chat -q` | |
| `jcode` | `-m` | unsupported | `jcode run` | |
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

From the CLI, `ai models <agent> --endpoint URL` lists the provider instead of disk:

```sh
ai models opencode --endpoint https://api.openai.com/v1 --api-key "$OPENAI_API_KEY" --prefix openai/
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

## Handoff config

A consumer can keep one command for every agent. The agent-specific argv is built by `ai exec`.

```toml
[handoff]
command = "mux --container {{container}} --layout tab --working-dir {{working_dir}} --session-id {{session_id}} -- ai exec --harness {{harness}} --model {{model}} --effort {{effort}} --prompt-file {{prompt_file}}"
```

Example for [tsk](https://github.com/Sovengar/tsk):

- `{{working_dir}}` → task project directory, then configured directory, then `~/Tasks`;
- `{{container}}` → working directory basename, so one workspace per repo;
- `{{session_id}}` → stable `tsk-<project>-<task>` label;
- `{{prompt_file}}` → private prompt file.

A complete example lives in [`examples/tsk-config.toml`](examples/tsk-config.toml).

## CLI reference

```text
ai agents
ai models <agent>
ai argv --agent <agent> [--model M] [--effort E] [--cwd DIR] [--] <prompt>
ai exec --harness <agent> [--model M] [--effort E] [--binary BIN]
        [--cwd DIR] (--prompt-file FILE | --prompt TEXT) [--dry-run]
```

`ai exec` uses `syscall.Exec`, so it preserves the TTY, signals and exit code.

## Development

```bash
go test ./...
go vet ./...
./scripts/mux_test.sh
```

The `mux` test uses fake backends; it never touches a real multiplexer.

## Limitations

- Interactive prompt placement is agent-specific; `opencode` has no model/effort flags in its TUI.
- Anthropic Messages API is not implemented.
- Model catalogs are local, undocumented files; a changed format returns an empty list.
- `mux` is POSIX `sh`; Windows is out of scope.

## License

MIT. See [LICENSE](LICENSE).
