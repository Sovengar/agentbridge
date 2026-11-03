package ai

import (
	"errors"
	"strings"
)

// ErrNoHeadless is returned when an agent has no non-interactive mode.
var ErrNoHeadless = errors.New("agent has no headless mode")

// CapabilitiesFor returns the argv conventions of one agent. An unknown agent
// has no flags and no headless mode.
func CapabilitiesFor(name string) Capabilities {
	switch name {
	case "pi":
		return Capabilities{
			ModelFlag:  []string{"--model", "{value}"},
			EffortFlag: []string{"--thinking", "{value}"},
			Headless:   []string{"-p"},
		}
	case "codex":
		return Capabilities{
			ModelFlag:  []string{"--model", "{value}"},
			EffortFlag: []string{"-c", "model_reasoning_effort={value}"},
			Headless:   []string{"exec"},
		}
	case "opencode":
		return Capabilities{
			ModelFlag:  []string{"--model", "{value}"},
			PromptFlag: []string{"--prompt", "{value}"},
			Headless:   []string{"run"},
			EffortJoin: "#",
		}
	case "hermes":
		return Capabilities{
			ModelFlag:           []string{"-m", "{value}"},
			EffortFlag:          []string{"--reasoning", "{value}"},
			Headless:            []string{"chat", "-q"},
			Interactive:         []string{"chat"},
			NoInteractivePrompt: true,
		}
	case "jcode":
		return Capabilities{
			ModelFlag:           []string{"-m", "{value}"},
			Headless:            []string{"run"},
			NoInteractivePrompt: true,
		}
	default:
		return Capabilities{}
	}
}

// ModelArgs returns the shell-quoted model and effort flags for the selected
// agent. It is used by handoff command templates, which run through a shell.
// An empty value omits its flag; an agent without the flag ignores the value.
func ModelArgs(agent Agent, model, effort string) string {
	caps := CapabilitiesFor(agent.Name)
	model, effort = combineModelEffort(caps, model, effort)
	args := shellFlags(caps.ModelFlag, model)
	args = append(args, shellFlags(caps.EffortFlag, effort)...)
	return strings.Join(args, " ")
}

// InteractiveArgs returns the raw argv that starts an agent interactively with
// the prompt already loaded. The prompt placement is agent-specific: positional
// for pi/codex, --prompt for opencode, and positional for unknown agents. An
// agent whose interactive UI cannot take an initial prompt (jcode, hermes chat)
// gets no prompt argument.
func InteractiveArgs(r Request) []string {
	caps := CapabilitiesFor(r.Agent.Name)
	binary := r.Agent.Binary
	if binary == "" {
		binary = r.Agent.Name
	}
	argv := make([]string, 0, 1+len(caps.Interactive)+len(caps.ModelFlag)+len(caps.EffortFlag)+len(caps.PromptFlag)+1)
	argv = append(argv, binary)
	argv = append(argv, caps.Interactive...)
	// opencode's interactive TUI has no --model/--variant flags; model choice
	// happens inside the TUI with /model. Headless `opencode run` does accept
	// --model provider/model#variant.
	if r.Agent.Name != "opencode" {
		model, effort := combineModelEffort(caps, r.Model, r.Effort)
		argv = append(argv, flags(caps.ModelFlag, model)...)
		argv = append(argv, flags(caps.EffortFlag, effort)...)
	}
	switch {
	case caps.NoInteractivePrompt:
	case len(caps.PromptFlag) > 0:
		argv = append(argv, flags(caps.PromptFlag, r.Prompt)...)
	default:
		argv = append(argv, r.Prompt)
	}
	return argv
}

// combineModelEffort merges the two request values for agents that encode them
// in a single model string (opencode: provider/model#variant).
func combineModelEffort(caps Capabilities, model, effort string) (string, string) {
	if caps.EffortJoin != "" && model != "" && effort != "" {
		return model + caps.EffortJoin + effort, ""
	}
	return model, effort
}

// HeadlessArgs returns the raw argv for a non-interactive run: the agent
// binary, its headless subcommand, model/effort flags and the prompt as the
// final argument. The prompt is one argv element, never a shell string.
func HeadlessArgs(r Request) ([]string, error) {
	caps := CapabilitiesFor(r.Agent.Name)
	if len(caps.Headless) == 0 {
		return nil, ErrNoHeadless
	}
	binary := r.Agent.Binary
	if binary == "" {
		binary = r.Agent.Name
	}
	argv := make([]string, 0, 1+len(caps.Headless)+len(caps.ModelFlag)+len(caps.EffortFlag)+1)
	argv = append(argv, binary)
	argv = append(argv, caps.Headless...)
	model, effort := combineModelEffort(caps, r.Model, r.Effort)
	argv = append(argv, flags(caps.ModelFlag, model)...)
	argv = append(argv, flags(caps.EffortFlag, effort)...)
	argv = append(argv, r.Prompt)
	return argv, nil
}

// shellFlags renders a flag template for a shell: a bare flag stays bare and
// only the value (or the combined key=value part) is quoted.
func shellFlags(template []string, value string) []string {
	if value == "" {
		return nil
	}
	out := make([]string, len(template))
	for i, part := range template {
		switch {
		case part == "{value}":
			out[i] = shellQuote(value)
		case strings.Contains(part, "{value}"):
			out[i] = shellQuote(strings.ReplaceAll(part, "{value}", value))
		default:
			out[i] = part
		}
	}
	return out
}

// flags expands a flag template with the value. An empty value omits the flag.
func flags(template []string, value string) []string {
	if value == "" {
		return nil
	}
	out := make([]string, len(template))
	for i, part := range template {
		out[i] = strings.ReplaceAll(part, "{value}", value)
	}
	return out
}

// shellQuote wraps a value in single quotes, escaping embedded single quotes,
// so the value reaches the shell as one literal argument.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
