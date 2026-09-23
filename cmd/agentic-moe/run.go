package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/anurgosw/agentic-moe/configgen"
	"github.com/anurgosw/agentic-moe/internal/buildinfo"
	"github.com/anurgosw/agentic-moe/mcpserver"
	"github.com/anurgosw/agentic-moe/provider/ollama"
	"github.com/anurgosw/agentic-moe/runtimekit"
)

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		if err := printUsage(stderr); err != nil {
			return fmt.Errorf("write usage: %w", err)
		}
		return flag.ErrHelp
	}
	switch args[0] {
	case "mcp":
		return runMCP(ctx, args[1:], stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(ctx, args[1:], stdout, stderr)
	case "config":
		return runConfig(args[1:], stdout, stderr)
	case "schema":
		return runSchema(args[1:], stdout, stderr)
	case "version":
		return runVersion(args[1:], stdout, stderr)
	case "completion":
		return runCompletion(args[1:], stdout)
	case "help", "-h", "--help":
		return printUsage(stdout)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runMCP(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("mcp requires stdio or http")
	}
	mode := args[0]
	fs := flag.NewFlagSet("mcp "+mode, flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "", "YAML or JSON configuration path")
	address := fs.String("address", "", "HTTP listen address override")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("mcp %s accepts no positional arguments", mode)
	}
	cfg, runtime, err := buildRuntime(*configPath, runtimekit.Overrides{HTTPAddress: *address})
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	server, err := mcpserver.New(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case "stdio":
		if *address != "" {
			return fmt.Errorf("--address is only valid for mcp http")
		}
		// The protocol owns stdout; human diagnostics stay on stderr.
		_ = stdout
		return server.RunStdio(ctx)
	case "http":
		token := ""
		if cfg.HTTP.BearerTokenEnv != "" {
			token = os.Getenv(cfg.HTTP.BearerTokenEnv)
			if token == "" {
				return fmt.Errorf("bearer token environment variable %q is empty", cfg.HTTP.BearerTokenEnv)
			}
		}
		if _, err := fmt.Fprintf(stderr, "agentic-moe MCP starting on %s\n", cfg.HTTP.Address); err != nil {
			return fmt.Errorf("write startup status: %w", err)
		}
		return server.RunHTTP(ctx, mcpserver.HTTPOptions{Address: cfg.HTTP.Address, BearerToken: token, AllowedHosts: cfg.HTTP.AllowedHosts, AllowedOrigins: cfg.HTTP.AllowedOrigins, MaxBodyBytes: cfg.HTTP.MaxBodyBytes, MaxConcurrent: cfg.HTTP.MaxConcurrent, RequestTimeout: time.Duration(cfg.HTTP.RequestTimeout), ShutdownGrace: time.Duration(cfg.HTTP.ShutdownGrace)})
	default:
		return fmt.Errorf("unknown mcp transport %q", mode)
	}
}

func runInit(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	target := fs.String("target", "generic", "generic, codex, vscode, or ollama")
	output := fs.String("output", "", "output path")
	binary := fs.String("binary", "agentic-moe", "binary path used by generated MCP config")
	configPath := fs.String("config", "", "framework config path used by generated MCP config")
	force := fs.Bool("force", false, "replace an existing regular file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("init accepts no positional arguments")
	}
	body, defaultPath, err := configgen.Render(*target, *binary, *configPath)
	if err != nil {
		return err
	}
	path := *output
	if path == "" {
		path = defaultPath
	}
	if err := configgen.Write(path, body, *force); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, path); err != nil {
		return fmt.Errorf("write generated path: %w", err)
	}
	return nil
}

func runDoctor(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "", "configuration path")
	jsonOutput := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("doctor accepts no positional arguments")
	}
	_, runtime, err := buildRuntime(*configPath, runtimekit.Overrides{})
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	report := runtime.Health(ctx)
	if *jsonOutput {
		if err := writeJSON(stdout, report); err != nil {
			return err
		}
		if report.Status != "ok" {
			return errors.New("doctor found degraded providers")
		}
		return nil
	}
	if _, err := fmt.Fprintf(stdout, "status: %s\nexperts: %d\nproviders: %d\n", report.Status, len(runtime.Manifest().Experts), len(report.Providers)); err != nil {
		return fmt.Errorf("write doctor report: %w", err)
	}
	if report.Status != "ok" {
		return errors.New("doctor found degraded providers")
	}
	return nil
}

func runConfig(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "validate" {
		return fmt.Errorf("config requires validate")
	}
	fs := flag.NewFlagSet("config validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("config validate requires one path")
	}
	cfg, err := runtimekit.Load(fs.Arg(0), os.Getenv, runtimekit.Overrides{})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(stdout, map[string]any{"valid": true, "version": cfg.Version})
	}
	if _, err := fmt.Fprintf(stdout, "valid configuration version %d\n", cfg.Version); err != nil {
		return fmt.Errorf("write validation result: %w", err)
	}
	return nil
}

func runSchema(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("schema", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("schema accepts no positional arguments")
	}
	body, err := runtimekit.SchemaJSON()
	if err != nil {
		return fmt.Errorf("render schema: %w", err)
	}
	if _, err := fmt.Fprintln(stdout, string(body)); err != nil {
		return fmt.Errorf("write schema: %w", err)
	}
	return nil
}

func runVersion(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("version accepts no positional arguments")
	}
	info := buildinfo.Current()
	if *jsonOutput {
		return writeJSON(stdout, info)
	}
	if _, err := fmt.Fprintf(stdout, "agentic-moe %s (%s, %s)\n", info.Version, info.Commit, info.Date); err != nil {
		return fmt.Errorf("write version: %w", err)
	}
	return nil
}

func buildRuntime(path string, overrides runtimekit.Overrides) (runtimekit.Config, *runtimekit.Runtime, error) {
	cfg, err := runtimekit.Load(path, os.Getenv, overrides)
	if err != nil {
		return runtimekit.Config{}, nil, err
	}
	var sources []runtimekit.ProviderSource
	if cfg.Ollama.Enabled {
		client, err := ollama.New(ollama.Config{BaseURL: cfg.Ollama.BaseURL, AllowRemote: cfg.Ollama.AllowRemote, Timeout: time.Duration(cfg.Ollama.Timeout), MaxBodyBytes: cfg.Ollama.MaxBodyBytes, ContextWindow: cfg.Ollama.ContextWindow, MaxParallel: cfg.Ollama.MaxParallel})
		if err != nil {
			return runtimekit.Config{}, nil, err
		}
		sources = append(sources, client)
	}
	runtime, err := runtimekit.New(cfg, runtimekit.Options{ProviderSources: sources})
	return cfg, runtime, err
}

func writeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func printUsage(w io.Writer) error {
	_, err := fmt.Fprintln(w, "usage: agentic-moe <mcp|init|doctor|config|schema|version|completion> [options]")
	return err
}

func runCompletion(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("completion requires bash, zsh, or fish")
	}
	commands := "mcp init doctor config schema version completion"
	switch strings.ToLower(args[0]) {
	case "bash":
		_, err := fmt.Fprintf(stdout, "complete -W %q agentic-moe\n", commands)
		return err
	case "zsh":
		_, err := fmt.Fprintf(stdout, "compdef '_arguments \"1:command:(%s)\"' agentic-moe\n", commands)
		return err
	case "fish":
		for _, command := range strings.Fields(commands) {
			if _, err := fmt.Fprintf(stdout, "complete -c agentic-moe -f -a %s\n", command); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported shell %q", args[0])
	}
	return nil
}
