package agentbridge

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

func TestAPISourceListsModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("Authorization = %q, want Bearer k", got)
		}
		if got := r.Header.Get("X-Extra"); got != "1" {
			t.Errorf("X-Extra = %q, want 1", got)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"b"},{"id":"a"},{"id":""}]}`))
	}))
	defer server.Close()

	source := APISource{
		Endpoint: server.URL + "/v1/",
		APIKey:   "k",
		Headers:  map[string]string{"X-Extra": "1"},
	}
	options, err := source.Options(context.Background(), Agent{Name: "x"})
	if err != nil {
		t.Fatalf("Options: %v", err)
	}
	if want := []string{"a", "b"}; !slices.Equal(options.Models, want) {
		t.Errorf("models = %v, want %v", options.Models, want)
	}
}

func TestAPISourcePrefixesModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"m"}]}`))
	}))
	defer server.Close()

	options, err := APISource{Endpoint: server.URL, ModelPrefix: "openai/"}.Options(context.Background(), Agent{})
	if err != nil {
		t.Fatalf("Options: %v", err)
	}
	if want := []string{"openai/m"}; !slices.Equal(options.Models, want) {
		t.Errorf("models = %v, want %v", options.Models, want)
	}
}

func TestAPISourceErrors(t *testing.T) {
	t.Run("empty endpoint", func(t *testing.T) {
		if _, err := (APISource{}).Options(context.Background(), Agent{}); err == nil {
			t.Fatal("want an error for an empty endpoint")
		}
	})

	t.Run("non-2xx is an APIError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`nope`))
		}))
		defer server.Close()

		_, err := APISource{Endpoint: server.URL}.Options(context.Background(), Agent{})
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
			t.Fatalf("err = %v, want a 401 APIError", err)
		}
	})

	t.Run("malformed body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{`))
		}))
		defer server.Close()

		if _, err := (APISource{Endpoint: server.URL}).Options(context.Background(), Agent{}); err == nil {
			t.Fatal("want a parse error")
		}
	})
}

func TestCompositeSourceFallsThrough(t *testing.T) {
	failing := sourceFunc(func(context.Context, Agent) (Options, error) {
		return Options{}, errors.New("boom")
	})
	empty := StaticSource{}
	full := StaticSource{Data: Options{Models: []string{"m"}}}

	options, err := CompositeSource{Sources: []ModelSource{failing, empty, full}}.
		Options(context.Background(), Agent{})
	if err != nil {
		t.Fatalf("Options: %v", err)
	}
	if !slices.Equal(options.Models, []string{"m"}) {
		t.Errorf("models = %v, want [m]", options.Models)
	}

	if _, err := (CompositeSource{Sources: []ModelSource{failing}}).Options(context.Background(), Agent{}); err == nil {
		t.Fatal("want the last error when every source fails")
	}
}

func TestCachedSourceReusesUntilExpiry(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	source := sourceFunc(func(context.Context, Agent) (Options, error) {
		calls++
		return Options{Models: []string{"m"}}, nil
	})
	cached := &CachedSource{
		Source: source,
		TTL:    time.Minute,
		Now:    func() time.Time { return now },
	}

	for range 3 {
		if _, err := cached.Options(context.Background(), Agent{}); err != nil {
			t.Fatalf("Options: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}

	now = now.Add(2 * time.Minute)
	if _, err := cached.Options(context.Background(), Agent{}); err != nil {
		t.Fatalf("Options: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 after expiry", calls)
	}
}

func TestCachedSourceKeepsValueOnError(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	failing := false
	source := sourceFunc(func(context.Context, Agent) (Options, error) {
		if failing {
			return Options{}, errors.New("boom")
		}
		return Options{Models: []string{"m"}}, nil
	})
	cached := &CachedSource{Source: source, TTL: time.Minute, Now: func() time.Time { return now }}

	if _, err := cached.Options(context.Background(), Agent{}); err != nil {
		t.Fatalf("Options: %v", err)
	}
	failing = true
	now = now.Add(2 * time.Minute)
	options, err := cached.Options(context.Background(), Agent{})
	if err != nil {
		t.Fatalf("Options after a failed refresh: %v", err)
	}
	if !slices.Equal(options.Models, []string{"m"}) {
		t.Errorf("models = %v, want the previous [m]", options.Models)
	}
}

func TestCachedSourceWithoutTTLDoesNotCache(t *testing.T) {
	calls := 0
	cached := &CachedSource{Source: sourceFunc(func(context.Context, Agent) (Options, error) {
		calls++
		return Options{}, nil
	})}
	for range 2 {
		if _, err := cached.Options(context.Background(), Agent{}); err != nil {
			t.Fatalf("Options: %v", err)
		}
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestGatewayRoutesByAgent(t *testing.T) {
	gateway := Gateway{
		Default: StaticSource{Data: Options{Models: []string{"default"}}},
		ByAgent: map[string]ModelSource{
			"dbx": StaticSource{Data: Options{Models: []string{"dbx"}}},
		},
	}

	cases := map[string]string{"dbx": "dbx", "pi": "default"}
	for agent, want := range cases {
		options, err := gateway.Options(context.Background(), Agent{Name: agent})
		if err != nil {
			t.Fatalf("%s: Options: %v", agent, err)
		}
		if !slices.Equal(options.Models, []string{want}) {
			t.Errorf("%s: models = %v, want [%s]", agent, options.Models, want)
		}
	}

	if options, err := (Gateway{}).Options(context.Background(), Agent{}); err != nil || len(options.Models) != 0 {
		t.Errorf("empty gateway = %v, %v; want empty and no error", options, err)
	}
}

func TestLocalSourceReadsTheAgentCatalog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeCatalog(t, home+"/.local/state/opencode/model.json",
		`{"recent":[{"providerID":"p","modelID":"m"}]}`)

	options, err := LocalSource{}.Options(context.Background(), Agent{Name: "opencode"})
	if err != nil {
		t.Fatalf("Options: %v", err)
	}
	if !slices.Equal(options.Models, []string{"p/m"}) {
		t.Errorf("models = %v, want [p/m]", options.Models)
	}

	anonymous, err := LocalSource{}.Options(context.Background(), Agent{Binary: "unknown"})
	if err != nil || len(anonymous.Models) != 0 {
		t.Errorf("unknown agent = %v, %v; want empty and no error", anonymous, err)
	}
}

// sourceFunc adapts a function to ModelSource.
type sourceFunc func(context.Context, Agent) (Options, error)

func (f sourceFunc) Options(ctx context.Context, agent Agent) (Options, error) {
	return f(ctx, agent)
}
