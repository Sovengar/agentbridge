// Command ai inspects the shared AI catalog and runs an agent interactively.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/sovengar/ai"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "agents":
		cmdAgents()
	case "models":
		cmdModels(os.Args[2:])
	case "argv":
		cmdArgv(os.Args[2:])
	case "exec":
		cmdExec(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "ai: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `ai - shared AI agent catalog, argv builders and executor

Usage:
  ai agents
  ai models <agent>
  ai argv --agent <agent> [--model M] [--effort E] [--cwd DIR] [--] <prompt>
  ai exec --harness <agent> [--model M] [--effort E] [--binary BIN]
          [--cwd DIR] (--prompt-file FILE | --prompt TEXT) [--dry-run]

Examples:
  ai agents
  ai models pi
  ai argv --agent pi --model free --effort high -- "fix the test"
  ai exec --harness pi --model free --effort high --prompt-file /tmp/p
`)
}

func cmdAgents() {
	for _, agent := range ai.Detect(nil, exec.LookPath) {
		fmt.Printf("%s\t%s\n", agent.Name, agent.Binary)
	}
}

func cmdModels(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: ai models <agent>")
		os.Exit(2)
	}
	options := ai.OptionsFor(args[0])
	fmt.Println("models:")
	for _, model := range options.Models {
		fmt.Printf("  %s\n", model)
	}
	fmt.Println("efforts:")
	for _, effort := range options.Efforts {
		fmt.Printf("  %s\n", effort)
	}
}

func cmdArgv(args []string) {
	fs := flag.NewFlagSet("argv", flag.ExitOnError)
	agent := fs.String("agent", "", "agent name")
	model := fs.String("model", "", "model")
	effort := fs.String("effort", "", "effort")
	cwd := fs.String("cwd", "", "working directory")
	_ = fs.Parse(args)
	if *agent == "" {
		fmt.Fprintln(os.Stderr, "usage: ai argv --agent <agent> [--model M] [--effort E] [--cwd DIR] -- <prompt>")
		os.Exit(2)
	}
	argv, err := ai.HeadlessArgs(ai.Request{
		Agent:  ai.Agent{Name: *agent, Binary: *agent},
		Model:  *model,
		Effort: *effort,
		Prompt: joinArgs(fs.Args()),
		Cwd:    *cwd,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ai: %v\n", err)
		os.Exit(1)
	}
	out, _ := json.MarshalIndent(argv, "", "  ")
	fmt.Println(string(out))
}

func cmdExec(args []string) {
	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	harness := fs.String("harness", "", "agent name")
	model := fs.String("model", "", "model")
	effort := fs.String("effort", "", "effort")
	binary := fs.String("binary", "", "agent binary override")
	cwd := fs.String("cwd", "", "working directory")
	promptFile := fs.String("prompt-file", "", "file containing the prompt")
	prompt := fs.String("prompt", "", "prompt text")
	dryRun := fs.Bool("dry-run", false, "print argv and exit")
	_ = fs.Parse(args)

	if *harness == "" {
		fmt.Fprintln(os.Stderr, "usage: ai exec --harness <agent> [--model M] [--effort E] [--binary BIN] [--cwd DIR] (--prompt-file FILE | --prompt TEXT)")
		os.Exit(2)
	}
	text, err := readPrompt(*promptFile, *prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ai: %v\n", err)
		os.Exit(2)
	}
	bin := *binary
	if bin == "" {
		bin = *harness
	}
	argv := ai.InteractiveArgs(ai.Request{
		Agent:  ai.Agent{Name: *harness, Binary: bin},
		Model:  *model,
		Effort: *effort,
		Prompt: text,
		Cwd:    *cwd,
	})
	if *dryRun {
		out, _ := json.MarshalIndent(argv, "", "  ")
		fmt.Println(string(out))
		return
	}
	if *cwd != "" {
		if err := os.Chdir(*cwd); err != nil {
			fmt.Fprintf(os.Stderr, "ai: %v\n", err)
			os.Exit(1)
		}
	}
	path, err := exec.LookPath(argv[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "ai: %v\n", err)
		os.Exit(1)
	}
	if err := syscall.Exec(path, argv, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "ai: %v\n", err)
		os.Exit(1)
	}
}

func readPrompt(file, text string) (string, error) {
	if file != "" && text != "" {
		return "", fmt.Errorf("use --prompt-file or --prompt, not both")
	}
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	if text != "" {
		return text, nil
	}
	return "", fmt.Errorf("missing --prompt-file or --prompt")
}

func joinArgs(args []string) string {
	out := ""
	for i, arg := range args {
		if i > 0 {
			out += " "
		}
		out += arg
	}
	return out
}
