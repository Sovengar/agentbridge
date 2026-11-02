package ai

import (
	"slices"
	"strings"
)

// builtinRegistry maps best-known agent display names to their executable.
// Members of this list are listed only when the binary is on PATH.
var builtinRegistry = []Agent{
	{Name: "aider", Binary: "aider"},
	{Name: "claude", Binary: "claude"},
	{Name: "codex", Binary: "codex"},
	{Name: "gemini", Binary: "gemini"},
	{Name: "hermes", Binary: "hermes"},
	{Name: "jcode", Binary: "jcode"},
	{Name: "opencode", Binary: "opencode"},
	{Name: "pi", Binary: "pi"},
}

// Builtins returns a copy of the built-in agent registry.
func Builtins() []Agent {
	return slices.Clone(builtinRegistry)
}

// Detect returns the built-in agents whose binary is on PATH, merged with the
// declared entries (additive). A declared entry with the same display name
// replaces the detected one, a missing binary defaults to the name, and the
// result is sorted by display name.
func Detect(declared []Agent, look func(string) (string, error)) []Agent {
	byName := make(map[string]Agent)
	for _, agent := range builtinRegistry {
		if _, err := look(agent.Binary); err == nil {
			byName[agent.Name] = agent
		}
	}
	for _, agent := range declared {
		if agent.Binary == "" {
			agent.Binary = agent.Name
		}
		byName[agent.Name] = agent
	}
	out := make([]Agent, 0, len(byName))
	for _, agent := range byName {
		out = append(out, agent)
	}
	slices.SortFunc(out, func(a, b Agent) int { return strings.Compare(a.Name, b.Name) })
	return out
}
