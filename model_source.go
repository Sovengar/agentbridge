package agentbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ModelSource answers where the picker reads models and efforts. The contract
// hides the origin (a local catalog, a provider endpoint, a static list), so a
// caller can swap it without touching the picker.
type ModelSource interface {
	Options(ctx context.Context, agent Agent) (Options, error)
}

// LocalSource reads the model catalog that the agent itself keeps on disk. It
// is the historical OptionsFor behaviour behind the ModelSource interface.
type LocalSource struct{}

// Options returns the local catalog of the agent. A missing or malformed
// catalog is not an error: the picker stays usable with an empty list.
func (LocalSource) Options(_ context.Context, agent Agent) (Options, error) {
	name := agent.Name
	if name == "" {
		name = agent.Binary
	}
	return OptionsFor(name), nil
}

// StaticSource returns one fixed option set, for agents with no discoverable
// catalog or for tests.
type StaticSource struct {
	Data Options
}

// Options returns the static set as given.
func (s StaticSource) Options(context.Context, Agent) (Options, error) {
	return s.Data, nil
}

// APISource lists models from an OpenAI-compatible /models endpoint. It is the
// provider-backed source: the answer comes from the provider, not from disk.
type APISource struct {
	Endpoint string
	APIKey   string
	Headers  map[string]string
	// HTTPClient is optional; nil uses http.DefaultClient.
	HTTPClient *http.Client
	// ModelPrefix, when set, is prepended to every listed model ID
	// (for agents whose model value is provider/model).
	ModelPrefix string
}

// Options sends GET {endpoint}/models and turns the answer into picker options.
// A non-2xx response is an APIError.
func (a APISource) Options(ctx context.Context, _ Agent) (Options, error) {
	endpoint := strings.TrimRight(a.Endpoint, "/")
	if endpoint == "" {
		return Options{}, errors.New("api source: endpoint is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/models", nil)
	if err != nil {
		return Options{}, fmt.Errorf("api source: create request: %w", err)
	}
	if a.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.APIKey)
	}
	for key, value := range a.Headers {
		req.Header.Set(key, value)
	}

	client := a.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return Options{}, fmt.Errorf("api source: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Options{}, fmt.Errorf("api source: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Options{}, &APIError{StatusCode: resp.StatusCode, Body: string(data)}
	}

	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Options{}, fmt.Errorf("api source: parse response: %w", err)
	}
	out := Options{Models: make([]string, 0, len(parsed.Data))}
	for _, model := range parsed.Data {
		if model.ID == "" {
			continue
		}
		out.Models = append(out.Models, a.ModelPrefix+model.ID)
	}
	return normalizeOptions(out), nil
}

// CompositeSource queries its sources in order and returns the first answer
// with at least one model. An error from one source does not stop the chain:
// the next source still gets a turn, and the last error is reported only when
// every source failed.
type CompositeSource struct {
	Sources []ModelSource
}

// Options returns the first non-empty answer in the chain.
func (c CompositeSource) Options(ctx context.Context, agent Agent) (Options, error) {
	var lastErr error
	for _, source := range c.Sources {
		options, err := source.Options(ctx, agent)
		if err != nil {
			lastErr = err
			continue
		}
		if len(options.Models) > 0 {
			return options, nil
		}
	}
	return Options{}, lastErr
}

// CachedSource memoizes another source for a TTL so the picker does not repeat
// a network call on every keypress. The zero TTL disables caching.
type CachedSource struct {
	Source ModelSource
	TTL    time.Duration
	// Now is optional; nil uses time.Now. Tests set a fake clock.
	Now func() time.Time

	mu     sync.Mutex
	cached Options
	expiry time.Time
	valid  bool
}

// Options returns the cached answer while it is fresh and refreshes it after
// the TTL. A failed refresh keeps the previous value.
func (c *CachedSource) Options(ctx context.Context, agent Agent) (Options, error) {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	if c.TTL <= 0 {
		return c.Source.Options(ctx, agent)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.valid && now().Before(c.expiry) {
		return c.cached, nil
	}
	options, err := c.Source.Options(ctx, agent)
	if err != nil {
		if c.valid {
			return c.cached, nil
		}
		return Options{}, err
	}
	c.cached = options
	c.expiry = now().Add(c.TTL)
	c.valid = true
	return options, nil
}

// Gateway routes a picker query to the source registered for the agent and
// falls back to Default. It is the one entry point an app holds; the sources
// behind it stay replaceable.
type Gateway struct {
	Default ModelSource
	ByAgent map[string]ModelSource
}

// Options returns the options of the agent's source, or of the default source
// when none is registered.
func (g Gateway) Options(ctx context.Context, agent Agent) (Options, error) {
	if source, ok := g.ByAgent[agent.Name]; ok && source != nil {
		return source.Options(ctx, agent)
	}
	if g.Default != nil {
		return g.Default.Options(ctx, agent)
	}
	return Options{}, nil
}
