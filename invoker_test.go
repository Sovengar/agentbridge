package agentbridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var (
	_ Invoker = ProcessInvoker{}
	_ Invoker = APIInvoker{}
)

func TestProcessInvokerBuildsHeadlessArgv(t *testing.T) {
	var gotArgv []string
	var gotDir string
	p := ProcessInvoker{Run: func(_ context.Context, argv []string, dir string) (string, string, error) {
		gotArgv = argv
		gotDir = dir
		return "answer\n", "", nil
	}}
	got, err := p.Invoke(context.Background(), Request{
		Agent:  Agent{Name: "pi", Binary: "pi"},
		Model:  "provider/model",
		Effort: "high",
		Prompt: "fix it",
		Cwd:    "/work",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pi", "-p", "--model", "provider/model", "--thinking", "high", "fix it"}
	if strings.Join(gotArgv, "|") != strings.Join(want, "|") {
		t.Errorf("argv = %v, want %v", gotArgv, want)
	}
	if gotDir != "/work" {
		t.Errorf("dir = %q, want /work", gotDir)
	}
	if got.Output != "answer\n" || got.Model != "provider/model" {
		t.Errorf("result = %+v", got)
	}
}

func TestProcessInvokerWithoutHeadlessMode(t *testing.T) {
	_, err := (ProcessInvoker{}).Invoke(context.Background(), Request{Agent: Agent{Name: "unknown"}})
	if !errors.Is(err, ErrNoHeadless) {
		t.Errorf("err = %v, want ErrNoHeadless", err)
	}
}

func TestProcessInvokerErrorPrefersStderr(t *testing.T) {
	sentinel := errors.New("exit 1")
	p := ProcessInvoker{Run: func(context.Context, []string, string) (string, string, error) {
		return "partial\n", "boom\n", sentinel
	}}
	_, err := p.Invoke(context.Background(), Request{Agent: Agent{Name: "pi"}})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want wrapped sentinel", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want stderr detail", err)
	}
}

func TestProcessInvokerErrorFallsBackToStdout(t *testing.T) {
	sentinel := errors.New("exit 1")
	p := ProcessInvoker{Run: func(context.Context, []string, string) (string, string, error) {
		return "only stdout\n", " \n", sentinel
	}}
	_, err := p.Invoke(context.Background(), Request{Agent: Agent{Name: "pi"}})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want wrapped sentinel", err)
	}
	if !strings.Contains(err.Error(), "only stdout") {
		t.Errorf("err = %v, want stdout detail", err)
	}
}

func TestProcessInvokerErrorWithoutDetail(t *testing.T) {
	sentinel := errors.New("exit 1")
	p := ProcessInvoker{Run: func(context.Context, []string, string) (string, string, error) {
		return "", "", sentinel
	}}
	_, err := p.Invoke(context.Background(), Request{Agent: Agent{Name: "pi"}})
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want the original error", err)
	}
}

func TestRunProcessCapturesStreams(t *testing.T) {
	stdout, stderr, err := runProcess(context.Background(), []string{"sh", "-c", "printf out; printf err >&2"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "out" || stderr != "err" {
		t.Errorf("stdout/stderr = %q/%q", stdout, stderr)
	}
}

func TestRunProcessReturnsExitError(t *testing.T) {
	_, stderr, err := runProcess(context.Background(), []string{"sh", "-c", "printf bad >&2; exit 3"}, "")
	if err == nil {
		t.Fatal("want an exit error")
	}
	if stderr != "bad" {
		t.Errorf("stderr = %q, want bad", stderr)
	}
}

func TestRunProcessUsesWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	stdout, _, err := runProcess(context.Background(), []string{"sh", "-c", "pwd"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout) != resolved {
		t.Errorf("pwd = %q, want %q", strings.TrimSpace(stdout), resolved)
	}
}

func TestAPIInvokerSendsRequest(t *testing.T) {
	var gotPath, gotAuth, gotCustom string
	var gotBody apiRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotCustom = r.Header.Get("X-Test")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"model":"m1","choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":3,"completion_tokens":5}}`))
	}))
	defer srv.Close()

	invoker := APIInvoker{
		Endpoint:  srv.URL + "/",
		APIKey:    "secret",
		Model:     "default-model",
		System:    "system prompt",
		Headers:   map[string]string{"X-Test": "1"},
		MaxTokens: 42,
	}
	got, err := invoker.Invoke(context.Background(), Request{Model: "override-model", Prompt: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/chat/completions" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotCustom != "1" {
		t.Errorf("custom header = %q", gotCustom)
	}
	if gotBody.Model != "override-model" || gotBody.MaxTokens != 42 {
		t.Errorf("body = %+v", gotBody)
	}
	if len(gotBody.Messages) != 2 || gotBody.Messages[0].Role != "system" || gotBody.Messages[1].Content != "hi" {
		t.Errorf("messages = %+v", gotBody.Messages)
	}
	if got.Output != "hello" || got.Model != "m1" || got.Usage.InputTokens != 3 || got.Usage.OutputTokens != 5 {
		t.Errorf("result = %+v", got)
	}
}

func TestAPIInvokerDefaultsWithoutSystemOrKey(t *testing.T) {
	var gotBody apiRequest
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	invoker := APIInvoker{Endpoint: srv.URL, Model: "default-model"}
	got, err := invoker.Invoke(context.Background(), Request{Prompt: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "" {
		t.Errorf("auth = %q, want none", gotAuth)
	}
	if len(gotBody.Messages) != 1 || gotBody.Messages[0].Role != "user" {
		t.Errorf("messages = %+v", gotBody.Messages)
	}
	if gotBody.Model != "default-model" {
		t.Errorf("model = %q", gotBody.Model)
	}
	if got.Model != "default-model" {
		t.Errorf("result model = %q, want the fallback", got.Model)
	}
}

func TestAPIInvokerRejectsEmptyEndpointOrModel(t *testing.T) {
	if _, err := (APIInvoker{}).Invoke(context.Background(), Request{Prompt: "x"}); err == nil {
		t.Error("want an empty-endpoint error")
	}
	if _, err := (APIInvoker{Endpoint: "http://example.test"}).Invoke(context.Background(), Request{Prompt: "x"}); err == nil {
		t.Error("want an empty-model error")
	}
}

func TestAPIInvokerRequestCreationError(t *testing.T) {
	_, err := (APIInvoker{Endpoint: "://bad", Model: "m"}).Invoke(context.Background(), Request{Prompt: "x"})
	if err == nil {
		t.Error("want a request creation error")
	}
}

func TestAPIInvokerHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream down"))
	}))
	defer srv.Close()

	_, err := (APIInvoker{Endpoint: srv.URL, Model: "m"}).Invoke(context.Background(), Request{Prompt: "x"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusBadGateway || !strings.Contains(apiErr.Body, "upstream down") {
		t.Errorf("APIError = %+v", apiErr)
	}
	if apiErr.Error() != "api error (502): upstream down" {
		t.Errorf("Error() = %q", apiErr.Error())
	}
}

func TestAPIErrorWithoutBody(t *testing.T) {
	err := (&APIError{StatusCode: 500}).Error()
	if err != "api error (500)" {
		t.Errorf("Error() = %q", err)
	}
}

func TestAPIInvokerInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{broken"))
	}))
	defer srv.Close()
	if _, err := (APIInvoker{Endpoint: srv.URL, Model: "m"}).Invoke(context.Background(), Request{Prompt: "x"}); err == nil {
		t.Error("want a parse error")
	}
}

func TestAPIInvokerResponseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer srv.Close()
	_, err := (APIInvoker{Endpoint: srv.URL, Model: "m"}).Invoke(context.Background(), Request{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("err = %v, want the API error message", err)
	}
}

func TestAPIInvokerEmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()
	if _, err := (APIInvoker{Endpoint: srv.URL, Model: "m"}).Invoke(context.Background(), Request{Prompt: "x"}); err == nil {
		t.Error("want an empty-response error")
	}
}

func TestAPIInvokerNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	if _, err := (APIInvoker{Endpoint: url, Model: "m"}).Invoke(context.Background(), Request{Prompt: "x"}); err == nil {
		t.Error("want a network error")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read boom") }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAPIInvokerReadError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(errReader{})}, nil
	})}
	_, err := (APIInvoker{Endpoint: "http://example.test", Model: "m", HTTPClient: client}).Invoke(context.Background(), Request{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "read boom") {
		t.Errorf("err = %v, want a read error", err)
	}
}

func TestRunProcessMissingBinary(t *testing.T) {
	_, _, err := runProcess(context.Background(), []string{filepath.Join(os.TempDir(), "definitely-not-a-binary")}, "")
	if err == nil {
		t.Error("want a spawn error")
	}
}

func TestProcessInvokerDefaultRunner(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := (ProcessInvoker{}).Invoke(context.Background(), Request{
		Agent:  Agent{Name: "pi", Binary: script},
		Prompt: "do it",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Output != "ok" {
		t.Errorf("output = %q, want ok", got.Output)
	}
}
