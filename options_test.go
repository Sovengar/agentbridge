package agentbridge

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCatalog(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func withHome(t *testing.T, dir string, err error) {
	t.Helper()
	orig := userHomeDir
	userHomeDir = func() (string, error) { return dir, err }
	t.Cleanup(func() { userHomeDir = orig })
}

func TestOptionsForUnknownAgent(t *testing.T) {
	if got := OptionsFor("unknown"); len(got.Models) != 0 || len(got.Efforts) != 0 {
		t.Errorf("OptionsFor = %+v, want empty", got)
	}
}

func TestOptionsForHomeError(t *testing.T) {
	withHome(t, "", errors.New("no home"))
	if got := OptionsFor("pi"); len(got.Models) != 0 || len(got.Efforts) != 0 {
		t.Errorf("OptionsFor = %+v, want empty", got)
	}
}

func TestOptionsForPi(t *testing.T) {
	home := t.TempDir()
	withHome(t, home, nil)
	writeCatalog(t, filepath.Join(home, ".pi", "agent", "models-store.json"), `{
		"anthropic": {"models": [
			{"id": "sonnet", "thinkingLevelMap": {"high": true, "low": true}},
			{"id": ""}
		]},
		"openai": {"models": [{"id": "gpt", "provider": "openai"}]}
	}`)
	got := OptionsFor("pi")
	if strings.Join(got.Models, ",") != "anthropic/sonnet,openai/gpt" {
		t.Errorf("Models = %v", got.Models)
	}
	if strings.Join(got.Efforts, ",") != "high,low" {
		t.Errorf("Efforts = %v", got.Efforts)
	}
}

func TestOptionsForCodex(t *testing.T) {
	home := t.TempDir()
	withHome(t, home, nil)
	writeCatalog(t, filepath.Join(home, ".codex", "models_cache.json"), `{
		"models": [
			{"slug": "gpt-5", "supported_reasoning_levels": [{"effort": "high"}, {"effort": "low"}]},
			{"slug": ""},
			{"slug": "a"}
		]
	}`)
	got := OptionsFor("codex")
	if strings.Join(got.Models, ",") != "a,gpt-5" {
		t.Errorf("Models = %v", got.Models)
	}
	if strings.Join(got.Efforts, ",") != "high,low" {
		t.Errorf("Efforts = %v", got.Efforts)
	}
}

func TestOptionsForOpencode(t *testing.T) {
	home := t.TempDir()
	withHome(t, home, nil)
	writeCatalog(t, filepath.Join(home, ".local", "state", "opencode", "model.json"), `{
		"recent": [{"providerID": "p", "modelID": "m1"}, {"providerID": "", "modelID": "x"}, {"providerID": "p", "modelID": ""}],
		"favorite": [{"providerID": "p", "modelID": "m2"}],
		"variant": {"a": "high", "b": "", "c": "low"}
	}`)
	got := OptionsFor("opencode")
	if strings.Join(got.Models, ",") != "p/m1,p/m2" {
		t.Errorf("Models = %v", got.Models)
	}
	if strings.Join(got.Efforts, ",") != "high,low" {
		t.Errorf("Efforts = %v", got.Efforts)
	}
}

func TestOptionsForHermes(t *testing.T) {
	home := t.TempDir()
	withHome(t, home, nil)
	writeCatalog(t, filepath.Join(home, ".hermes", "provider_models_cache.json"), `{
		"openai-api": {"models": ["gpt-5", "a"]},
		"copilot": {"models": ["a", "claude"]}
	}`)
	writeCatalog(t, filepath.Join(home, ".hermes", "ollama_cloud_models_cache.json"), `{"models": ["glm-5.1"]}`)
	got := OptionsFor("hermes")
	if strings.Join(got.Models, ",") != "a,claude,glm-5.1,gpt-5" {
		t.Errorf("Models = %v", got.Models)
	}
}

func TestOptionsForJcode(t *testing.T) {
	home := t.TempDir()
	withHome(t, home, nil)
	writeCatalog(t, filepath.Join(home, ".config", "jcode", "openai_model_catalog_cache.json"), `{
		"scopes": {"default": {"models": ["gpt-5.6-luna", "a"]}}
	}`)
	writeCatalog(t, filepath.Join(home, ".jcode", "cache", "opencode_models.json"), `{
		"models": [{"id": "a"}, {"id": "claude-opus"}]
	}`)
	got := OptionsFor("jcode")
	if strings.Join(got.Models, ",") != "a,claude-opus,gpt-5.6-luna" {
		t.Errorf("Models = %v", got.Models)
	}
}

func TestOptionsForMalformedAndMissingCatalogs(t *testing.T) {
	home := t.TempDir()
	withHome(t, home, nil)
	writeCatalog(t, filepath.Join(home, ".pi", "agent", "models-store.json"), "{broken")
	if got := OptionsFor("pi"); len(got.Models) != 0 || len(got.Efforts) != 0 {
		t.Errorf("malformed pi options = %+v, want empty", got)
	}
	if got := OptionsFor("codex"); len(got.Models) != 0 || len(got.Efforts) != 0 {
		t.Errorf("missing codex options = %+v, want empty", got)
	}
	if got := OptionsFor("opencode"); len(got.Models) != 0 || len(got.Efforts) != 0 {
		t.Errorf("missing opencode options = %+v, want empty", got)
	}
}

func TestUniqueSortedRemovesEmptyAndDuplicates(t *testing.T) {
	got := uniqueSorted([]string{"b", "", "a", "b", "a"})
	if strings.Join(got, ",") != "a,b" {
		t.Errorf("uniqueSorted = %v, want [a b]", got)
	}
}
