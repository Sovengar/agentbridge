// Package agentbridge defines the shared AI request contract and the capability table
// that turns it into argv. It separates four concerns: which agents exist
// (catalog), how an agent is invoked (invoke), which models/efforts it offers
// (models), and what the caller wants (request). Presentation (overlay, pane,
// detached process, direct API) stays in the caller.
package agentbridge

// Agent is one selectable AI harness: a display name and the executable that
// runs it.
type Agent struct {
	Name   string
	Binary string
}

// Request is the invariant every AI mode shares. Mode-specific transport
// (inline API, headless subprocess, detached handoff, attached pane) never
// changes these four values.
type Request struct {
	Agent  Agent
	Model  string
	Effort string
	Prompt string
	Cwd    string
}

// Options is the model/effort picker data for one agent. A missing or
// malformed catalog returns empty options.
type Options struct {
	Models  []string
	Efforts []string
}

// Capabilities describes how one agent expresses the shared request as argv.
// ModelFlag, EffortFlag and PromptFlag are flag templates: {value} is replaced.
// Headless is the argv prefix after the binary. When PromptFlag is nil the
// prompt is appended as the final argv element; otherwise the flag carries it.
type Capabilities struct {
	ModelFlag  []string
	EffortFlag []string
	PromptFlag []string
	Headless   []string
	// Interactive is the argv prefix for an interactive start (hermes: chat).
	Interactive []string
	// NoInteractivePrompt marks an agent whose interactive UI cannot take an
	// initial prompt (jcode, hermes chat). The prompt is then dropped and the
	// caller must show it to the user or use the headless mode instead.
	NoInteractivePrompt bool
	// EffortJoin, when non-empty, appends the effort to the model value
	// (for agents that encode both in one flag, e.g. opencode provider/model#variant).
	EffortJoin string
}
