package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

// userHomeDir is a variable so tests can point the catalogs at a temp tree.
var userHomeDir = os.UserHomeDir

// OptionsFor loads the local model registry used by the selected agent. A
// missing or malformed registry returns empty options so the picker stays
// usable with configured command templates and custom agents.
func OptionsFor(name string) Options {
	home, err := userHomeDir()
	if err != nil {
		return Options{}
	}
	switch name {
	case "pi":
		return piOptions(filepath.Join(home, ".pi", "agent", "models-store.json"))
	case "codex":
		return codexOptions(filepath.Join(home, ".codex", "models_cache.json"))
	case "opencode":
		return opencodeOptions(filepath.Join(home, ".local", "state", "opencode", "model.json"))
	case "hermes":
		return hermesOptions(filepath.Join(home, ".hermes"))
	case "jcode":
		return jcodeOptions(home)
	default:
		return Options{}
	}
}

func readJSON(path string, dst any) bool {
	data, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(data, dst) == nil
}

func piOptions(path string) Options {
	var root map[string]struct {
		Models []struct {
			ID               string         `json:"id"`
			Provider         string         `json:"provider"`
			ThinkingLevelMap map[string]any `json:"thinkingLevelMap"`
		} `json:"models"`
	}
	if !readJSON(path, &root) {
		return Options{}
	}
	var out Options
	for provider, catalog := range root {
		for _, model := range catalog.Models {
			if model.ID == "" {
				continue
			}
			modelProvider := model.Provider
			if modelProvider == "" {
				modelProvider = provider
			}
			out.Models = append(out.Models, modelProvider+"/"+model.ID)
			for level := range model.ThinkingLevelMap {
				out.Efforts = append(out.Efforts, level)
			}
		}
	}
	return normalizeOptions(out)
}

func codexOptions(path string) Options {
	var root struct {
		Models []struct {
			Slug                     string `json:"slug"`
			SupportedReasoningLevels []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if !readJSON(path, &root) {
		return Options{}
	}
	var out Options
	for _, model := range root.Models {
		if model.Slug == "" {
			continue
		}
		out.Models = append(out.Models, model.Slug)
		for _, level := range model.SupportedReasoningLevels {
			out.Efforts = append(out.Efforts, level.Effort)
		}
	}
	return normalizeOptions(out)
}

func opencodeOptions(path string) Options {
	var root struct {
		Recent []struct {
			ProviderID string `json:"providerID"`
			ModelID    string `json:"modelID"`
		} `json:"recent"`
		Favorite []struct {
			ProviderID string `json:"providerID"`
			ModelID    string `json:"modelID"`
		} `json:"favorite"`
		Variant map[string]string `json:"variant"`
	}
	if !readJSON(path, &root) {
		return Options{}
	}
	var out Options
	for _, model := range append(root.Recent, root.Favorite...) {
		if model.ProviderID != "" && model.ModelID != "" {
			out.Models = append(out.Models, model.ProviderID+"/"+model.ModelID)
		}
	}
	for _, variant := range root.Variant {
		if variant != "" {
			out.Efforts = append(out.Efforts, variant)
		}
	}
	return normalizeOptions(out)
}

func hermesOptions(dir string) Options {
	var out Options
	var providers map[string]struct {
		Models []string `json:"models"`
	}
	if readJSON(filepath.Join(dir, "provider_models_cache.json"), &providers) {
		for _, provider := range providers {
			out.Models = append(out.Models, provider.Models...)
		}
	}
	var ollama struct {
		Models []string `json:"models"`
	}
	if readJSON(filepath.Join(dir, "ollama_cloud_models_cache.json"), &ollama) {
		out.Models = append(out.Models, ollama.Models...)
	}
	return normalizeOptions(out)
}

func jcodeOptions(home string) Options {
	var out Options
	var openai struct {
		Scopes map[string]struct {
			Models []string `json:"models"`
		} `json:"scopes"`
	}
	if readJSON(filepath.Join(home, ".config", "jcode", "openai_model_catalog_cache.json"), &openai) {
		for _, scope := range openai.Scopes {
			out.Models = append(out.Models, scope.Models...)
		}
	}
	paths, _ := filepath.Glob(filepath.Join(home, ".jcode", "cache", "*_models.json"))
	for _, path := range paths {
		var catalog struct {
			Models []struct {
				ID string `json:"id"`
			} `json:"models"`
		}
		if !readJSON(path, &catalog) {
			continue
		}
		for _, model := range catalog.Models {
			out.Models = append(out.Models, model.ID)
		}
	}
	return normalizeOptions(out)
}

func normalizeOptions(options Options) Options {
	options.Models = uniqueSorted(options.Models)
	options.Efforts = uniqueSorted(options.Efforts)
	return options
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}
