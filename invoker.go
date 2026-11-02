package ai

import "context"

// Invoker executes a Request. The transport is the implementation's concern:
// a local headless process, an HTTP API, or an in-process provider.
type Invoker interface {
	Invoke(ctx context.Context, r Request) (Result, error)
}

// Usage is the token accounting reported by an API. A local process usually
// leaves it zero.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Result is the output of one invocation, independent of the transport.
type Result struct {
	Output string
	Model  string
	Usage  Usage
}
