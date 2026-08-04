// Package cli implements the quartr command-line interface: argument
// parsing, command dispatch, request shaping, and pagination.
//
// All commands are driven from the resources map in resources.go. Adding
// a new resource is a single map entry — no per-command handler code.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"quartr-cli/internal/quartr"
)

// Version is the CLI release string, surfaced via `quartr --version`.
const Version = "0.1.0"

type app struct {
	out    io.Writer
	errOut io.Writer
	cfg    effectiveConfig
	client *quartr.Client
}

func newApp(out, errOut io.Writer, cfg effectiveConfig) *app {
	return &app{
		out:    out,
		errOut: errOut,
		cfg:    cfg,
		client: cfg.newClient(),
	}
}

// Run is the package entry point. It parses args (without the leading
// program name), builds the effective config from flags/env/file, and
// dispatches the command. Returns the exit code: 0 for success, 1 for a
// runtime error, 2 for a usage error.
func Run(args []string, out, errOut io.Writer) int {
	globals, commandArgs, err := extractGlobalFlags(args)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	if globals.Version {
		fmt.Fprintf(out, "quartr %s\n", Version)
		return 0
	}

	cfg, err := buildConfig(globals)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}

	a := newApp(out, errOut, cfg)
	if len(commandArgs) == 0 {
		a.printRootHelp()
		return 0
	}
	if globals.Help {
		commandArgs = append(commandArgs, "--help")
	}
	if err := a.dispatch(commandArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(errOut, err)
		if hint := errorHint(err); hint != "" {
			fmt.Fprintln(errOut, hint)
		}
		var usage *usageError
		if errors.As(err, &usage) {
			return 2
		}
		return 1
	}
	return 0
}

func (a *app) dispatch(args []string) error {
	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "help":
		if len(rest) == 0 {
			a.printRootHelp()
			return nil
		}
		return a.dispatch(append(rest, "--help"))
	case "auth":
		return a.handleAuth(rest)
	case "request", "raw":
		return a.handleRequest(rest)
	case "companies", "events", "documents", "reports", "slides", "transcripts", "audio", "event-types", "document-types", "live-audio", "live-transcripts":
		return a.handleResourceByName(cmd, rest)
	case "live":
		if len(rest) > 0 {
			sub := rest[0]
			if sub == "audio" {
				return a.handleResourceByName("live-audio", rest[1:])
			}
			if sub == "transcripts" || sub == "transcript" {
				return a.handleResourceByName("live-transcripts", rest[1:])
			}
		}
		return a.handleResourceByName("live", rest)
	default:
		return fmt.Errorf("unknown command %q; run `quartr help`", cmd)
	}
}

func (a *app) handleResourceByName(name string, args []string) error {
	r, ok := resourceByName(name)
	if !ok {
		return fmt.Errorf("unknown command %q; run `quartr help`", name)
	}
	return a.handleResource(r, args)
}
