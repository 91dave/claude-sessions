package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/91dave/claude-sessions/internal/ask"
	"github.com/91dave/claude-sessions/internal/config"
	"github.com/91dave/claude-sessions/internal/index"
	"github.com/91dave/claude-sessions/internal/launch"
	"github.com/91dave/claude-sessions/internal/live"
	"github.com/91dave/claude-sessions/internal/tui"
)

var version = "dev"

const usage = `claude-sessions: find, preview and resume a past Claude Code session from any project

Usage:
  claude-sessions              interactive picker
  claude-sessions --list       print sessions as tab-separated rows
  claude-sessions --config     show the config file path and current settings

Options:
  --tmux / --no-tmux   resume in a new tmux session, or not, overriding the config
  --dry-run            print the commands instead of running them
  --version, --help

Keys:
  enter        default action (resume, unless the config says fork)
  ctrl+o       the other action
  ctrl+x       hide the session (adds it to the blocklist)
  ctrl+s, ?    switch between filtering and asking Claude
  shift+↑/↓    scroll the preview
  esc          quit

A running session is never resumed a second time: one in a reachable tmux pane is
jumped to, any other is refused (fork it instead).
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("claude-sessions", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, usage) }
	list := flags.Bool("list", false, "")
	showConfig := flags.Bool("config", false, "")
	useTmux := flags.Bool("tmux", false, "")
	noTmux := flags.Bool("no-tmux", false, "")
	dryRun := flags.Bool("dry-run", os.Getenv("CLAUDE_SESSIONS_DRYRUN") != "", "")
	showVersion := flags.Bool("version", false, "")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}

	paths, err := config.ResolvePaths(os.Getenv)
	if err != nil {
		fmt.Fprintln(stderr, "claude-sessions:", err)
		return 1
	}
	cfg, err := config.Load(paths.ConfigFile, os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "claude-sessions: %v (using defaults)\n", err)
	}
	if *useTmux {
		cfg.Tmux = config.TmuxNewSession
	}
	if *noTmux {
		cfg.Tmux = config.TmuxOff
	}

	if *showConfig {
		printConfig(stdout, paths, cfg)
		return 0
	}

	opts := index.Options{
		ClaudeDir: paths.ClaudeDir,
		CacheFile: paths.CacheFile,
		Blocklist: paths.Blocklist,
		Alive:     live.ProcessAlive,
	}
	if *list {
		entries, err := index.Load(opts)
		if err != nil {
			fmt.Fprintln(stderr, "claude-sessions:", err)
			return 1
		}
		writeList(stdout, entries)
		return 0
	}

	return pick(opts, paths, cfg, *dryRun, stdout, stderr)
}

func pick(opts index.Options, paths config.Paths, cfg config.Config, dryRun bool, stdout, stderr io.Writer) int {
	env := launch.CurrentEnv()
	asker := ask.Asker{Command: cfg.ClaudeCommand, Model: cfg.AskModel, Dir: paths.CacheDir}
	model := tui.New(tui.Deps{
		Config: cfg,
		Load:   func() ([]index.Entry, error) { return index.Load(opts) },
		Block:  func(id string) error { return index.Block(opts.Blocklist, id) },
		Ask: func(ctx context.Context, q string, entries []index.Entry) ([]index.Entry, error) {
			return asker.Rank(ctx, q, entries)
		},
		Plan: func(action string, e index.Entry) (launch.Plan, error) {
			return launch.PlanLaunch(action, e, cfg, env)
		},
	})

	final, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	if err != nil {
		fmt.Fprintln(stderr, "claude-sessions:", err)
		return 1
	}
	plan, ok := final.(tui.Model).Result()
	if !ok {
		return 0
	}
	if dryRun {
		fmt.Fprintln(stdout, plan.String())
		return 0
	}
	if err := launch.Run(plan); err != nil {
		fmt.Fprintln(stderr, "claude-sessions:", err)
		return 1
	}
	return 0
}

func writeList(w io.Writer, entries []index.Entry) {
	for _, e := range entries {
		mark := ""
		if e.Live {
			mark = "●"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			e.ID, e.ModTime.Format("2006-01-02 15:04"), mark, e.Project(), e.Branch,
			strings.Join(strings.Fields(e.Title()), " "), e.Cwd)
	}
}

func printConfig(w io.Writer, paths config.Paths, cfg config.Config) {
	fmt.Fprintf(w, "config file:  %s\nblocklist:    %s\ncache:        %s\nclaude dir:   %s\n\n",
		paths.ConfigFile, paths.Blocklist, paths.CacheFile, paths.ClaudeDir)
	data, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Fprintln(w, string(data))
}

func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}
