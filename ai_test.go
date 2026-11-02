package ai

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func fakeLook(available ...string) func(string) (string, error) {
	return func(binary string) (string, error) {
		if slices.Contains(available, binary) {
			return "/usr/bin/" + binary, nil
		}
		return "", errors.New("not found")
	}
}

func names(agents []Agent) []string {
	out := make([]string, 0, len(agents))
	for _, agent := range agents {
		out = append(out, agent.Name)
	}
	return out
}

func TestBuiltinsReturnsACopy(t *testing.T) {
	first := Builtins()
	if len(first) == 0 {
		t.Fatal("Builtins is empty")
	}
	first[0].Name = "mutated"
	if Builtins()[0].Name == "mutated" {
		t.Error("Builtins returned the shared slice")
	}
}

func TestDetectOnlyInstalledBuiltins(t *testing.T) {
	got := names(Detect(nil, fakeLook("opencode")))
	if len(got) != 1 || got[0] != "opencode" {
		t.Errorf("Detect = %v, want [opencode]", got)
	}
}

func TestDetectAddsDeclaredUnknownAgent(t *testing.T) {
	got := Detect([]Agent{{Name: "my-tool", Binary: "my-tool-bin"}}, fakeLook())
	if len(got) != 1 || got[0].Name != "my-tool" || got[0].Binary != "my-tool-bin" {
		t.Errorf("Detect = %+v, want the declared my-tool entry", got)
	}
}

func TestDetectDefaultsDeclaredBinaryToName(t *testing.T) {
	got := Detect([]Agent{{Name: "my-tool"}}, fakeLook())
	if len(got) != 1 || got[0].Binary != "my-tool" {
		t.Errorf("Detect = %+v, want binary defaulting to the name", got)
	}
}

func TestDetectDeclaredWinsOverBuiltin(t *testing.T) {
	got := Detect([]Agent{{Name: "opencode", Binary: "opencode-custom"}}, fakeLook("opencode"))
	var found []Agent
	for _, agent := range got {
		if agent.Name == "opencode" {
			found = append(found, agent)
		}
	}
	if len(found) != 1 {
		t.Fatalf("Detect = %+v, want exactly one opencode entry", got)
	}
	if found[0].Binary != "opencode-custom" {
		t.Errorf("opencode binary = %q, want the declared override", found[0].Binary)
	}
}

func TestDetectSortedAlphabetically(t *testing.T) {
	got := names(Detect([]Agent{{Name: "zzz"}, {Name: "aaa"}}, fakeLook("opencode")))
	want := []string{"aaa", "opencode", "zzz"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Detect order = %v, want %v", got, want)
	}
}

func TestCapabilitiesFor(t *testing.T) {
	for _, tc := range []struct {
		name       string
		model      []string
		effort     []string
		headless   []string
		effortJoin string
	}{
		{"pi", []string{"--model", "{value}"}, []string{"--thinking", "{value}"}, []string{"-p"}, ""},
		{"codex", []string{"--model", "{value}"}, []string{"-c", "model_reasoning_effort={value}"}, []string{"exec"}, ""},
		{"opencode", []string{"--model", "{value}"}, nil, []string{"run"}, "#"},
		{"hermes", []string{"-m", "{value}"}, []string{"--reasoning", "{value}"}, []string{"chat", "-q"}, ""},
		{"jcode", []string{"-m", "{value}"}, nil, []string{"run"}, ""},
		{"unknown", nil, nil, nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := CapabilitiesFor(tc.name)
			if strings.Join(got.ModelFlag, " ") != strings.Join(tc.model, " ") {
				t.Errorf("ModelFlag = %v, want %v", got.ModelFlag, tc.model)
			}
			if strings.Join(got.EffortFlag, " ") != strings.Join(tc.effort, " ") {
				t.Errorf("EffortFlag = %v, want %v", got.EffortFlag, tc.effort)
			}
			if strings.Join(got.Headless, " ") != strings.Join(tc.headless, " ") {
				t.Errorf("Headless = %v, want %v", got.Headless, tc.headless)
			}
			if got.EffortJoin != tc.effortJoin {
				t.Errorf("EffortJoin = %q, want %q", got.EffortJoin, tc.effortJoin)
			}
		})
	}
}

func TestModelArgs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		model  string
		effort string
		want   string
	}{
		{"pi", "provider/model", "high", `--model 'provider/model' --thinking 'high'`},
		{"pi", "", "high", `--thinking 'high'`},
		{"pi", "provider/model", "", `--model 'provider/model'`},
		{"pi", "", "", ""},
		{"codex", "gpt-5", "high", `--model 'gpt-5' -c 'model_reasoning_effort=high'`},
		{"opencode", "provider/model", "high", `--model 'provider/model#high'`},
		{"hermes", "model", "low", `-m 'model' --reasoning 'low'`},
		{"jcode", "model", "high", `-m 'model'`},
		{"unknown", "model", "high", ""},
	} {
		t.Run(tc.name+"/"+tc.model+"/"+tc.effort, func(t *testing.T) {
			if got := ModelArgs(Agent{Name: tc.name}, tc.model, tc.effort); got != tc.want {
				t.Errorf("ModelArgs = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestModelArgsQuotesSingleQuotes(t *testing.T) {
	got := ModelArgs(Agent{Name: "pi"}, "o'pen", "")
	want := `--model 'o'\''pen'`
	if got != want {
		t.Errorf("ModelArgs = %q, want %q", got, want)
	}
}

func TestInteractiveArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    Request
		want []string
	}{
		{
			name: "pi positional prompt",
			r:    Request{Agent: Agent{Name: "pi", Binary: "pi"}, Model: "free", Effort: "high", Prompt: "fix it"},
			want: []string{"pi", "--model", "free", "--thinking", "high", "fix it"},
		},
		{
			name: "codex positional prompt",
			r:    Request{Agent: Agent{Name: "codex"}, Prompt: "fix it"},
			want: []string{"codex", "fix it"},
		},
		{
			name: "opencode prompt flag, model set in TUI",
			r:    Request{Agent: Agent{Name: "opencode"}, Model: "p/m", Effort: "high", Prompt: "fix it"},
			want: []string{"opencode", "--prompt", "fix it"},
		},
		{
			name: "unknown positional prompt",
			r:    Request{Agent: Agent{Name: "my-tool", Binary: "/opt/my-tool"}, Prompt: "fix it"},
			want: []string{"/opt/my-tool", "fix it"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := InteractiveArgs(tc.r)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("InteractiveArgs = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestInteractiveArgsOmitsUnsupportedEffort(t *testing.T) {
	got := InteractiveArgs(Request{Agent: Agent{Name: "jcode"}, Model: "m", Effort: "high", Prompt: "p"})
	want := []string{"jcode", "-m", "m", "p"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("InteractiveArgs = %v, want %v", got, want)
	}
}

func TestHeadlessArgs(t *testing.T) {
	got, err := HeadlessArgs(Request{
		Agent:  Agent{Name: "pi", Binary: "pi"},
		Model:  "provider/model",
		Effort: "high",
		Prompt: "fix it",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pi", "-p", "--model", "provider/model", "--thinking", "high", "fix it"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("HeadlessArgs = %v, want %v", got, want)
	}
}

func TestHeadlessArgsUsesDeclaredBinary(t *testing.T) {
	got, err := HeadlessArgs(Request{Agent: Agent{Name: "pi", Binary: "/opt/pi"}, Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != "/opt/pi" {
		t.Errorf("argv[0] = %q, want the declared binary", got[0])
	}
}

func TestHeadlessArgsDefaultsBinaryToName(t *testing.T) {
	got, err := HeadlessArgs(Request{Agent: Agent{Name: "pi"}, Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != "pi" {
		t.Errorf("argv[0] = %q, want the agent name", got[0])
	}
}

func TestHeadlessArgsWithoutHeadlessMode(t *testing.T) {
	_, err := HeadlessArgs(Request{Agent: Agent{Name: "unknown"}, Prompt: "p"})
	if !errors.Is(err, ErrNoHeadless) {
		t.Errorf("err = %v, want ErrNoHeadless", err)
	}
}

func TestHeadlessArgsOmitsUnsupportedEffort(t *testing.T) {
	got, err := HeadlessArgs(Request{Agent: Agent{Name: "jcode"}, Model: "m", Effort: "high", Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"jcode", "run", "-m", "m", "p"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("HeadlessArgs = %v, want %v", got, want)
	}
}
