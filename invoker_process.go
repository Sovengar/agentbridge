package agentbridge

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ProcessRunner runs one argv in a working directory and returns stdout,
// stderr and the process error. It is a seam so tests do not spawn processes.
type ProcessRunner func(ctx context.Context, argv []string, dir string) (stdout, stderr string, err error)

// ProcessInvoker runs an agent headless: it builds the argv from the shared
// capability table and captures stdout. stderr is diagnostics, not output.
type ProcessInvoker struct {
	Run ProcessRunner
}

// Invoke builds the headless argv and runs it. A non-zero exit is an error that
// carries stderr (or stdout when stderr is empty) so the TUI can explain it.
func (p ProcessInvoker) Invoke(ctx context.Context, r Request) (Result, error) {
	argv, err := HeadlessArgs(r)
	if err != nil {
		return Result{}, err
	}
	run := p.Run
	if run == nil {
		run = runProcess
	}
	stdout, stderr, err := run(ctx, argv, r.Cwd)
	if err != nil {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = strings.TrimSpace(stdout)
		}
		if detail != "" {
			return Result{}, fmt.Errorf("%s: %w", detail, err)
		}
		return Result{}, err
	}
	return Result{Output: stdout, Model: r.Model}, nil
}

// runProcess is the real runner: no shell, so the prompt stays one argv element.
func runProcess(ctx context.Context, argv []string, dir string) (string, string, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}
