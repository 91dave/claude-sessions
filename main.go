package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/91dave/claude-sessions/internal/ask"
	"github.com/91dave/claude-sessions/internal/config"
	"github.com/91dave/claude-sessions/internal/index"
	"github.com/91dave/claude-sessions/internal/launch"
	"github.com/91dave/claude-sessions/internal/live"
	"github.com/91dave/claude-sessions/internal/preview"
	"github.com/91dave/claude-sessions/internal/transcript"
	"github.com/91dave/claude-sessions/internal/tui"
)

var version = "dev"

const usage = `claude-sessions: find, preview and resume a past Claude Code session from any project

Usage:
  claude-sessions              interactive picker
  claude-sessions --list       print sessions as tab-separated rows
  claude-sessions --config     show the config file path and current settings

Options:
  --tmux, --jump       resume in a new tmux session, overriding the config
  --no-tmux            resume in this terminal, overriding the config
  --dry-run            print the commands instead of running them
  --version, --help

Keys:
  enter                  default action (resume, unless the config says fork)
  ctrl+o, alt+enter      the other action
  ctrl+x, alt+d          hide the session (adds it to the blocklist)
  alt+e                  end a session (remove its running marker)
  ctrl+s, ?              switch between filtering and asking Claude
  shift+↑/↓              scroll the preview
  esc                    quit

Scripting:
  --resume <id>, --fork <id>         resume or fork one session without the picker
  --action <enter|alt-enter> <id> [cwd]
  --jump-action <enter|alt-enter> <id> [cwd]   as --action, with --tmux
  --resumes-running <claude args>    print the running session those args would reopen
  --attach-running <id>              go to a running session's pane
  --preview <id> [cwd] [pane]        print a session's preview
  --ask <question>                   print the sessions Claude ranks for a question
  --has-turns <transcript>           exit 0 if the transcript holds a real turn
  --opening-prompt <transcript>      print a transcript's opening prompt
  --end <id>, --block <id>           remove a session's marker, or hide it

A running session is never resumed a second time: one in a reachable tmux pane is
jumped to, any other is refused (fork it instead).
`

type app struct {
	stdout, stderr io.Writer
	paths          config.Paths
	cfg            config.Config
	env            launch.Env
	dryRun         bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	a := &app{stdout: stdout, stderr: stderr, env: launch.CurrentEnv(), dryRun: os.Getenv("CLAUDE_SESSIONS_DRYRUN") != ""}
	paths, err := config.ResolvePaths(os.Getenv)
	if err != nil {
		return a.fail(err)
	}
	a.paths = paths
	a.cfg, err = config.Load(paths.ConfigFile, os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "claude-sessions: %v (using defaults)\n", err)
	}

	if len(args) > 0 {
		if command, ok := scripting[args[0]]; ok {
			return command(a, args[1:])
		}
	}
	return a.interactive(args)
}

func (a *app) interactive(args []string) int {
	flags := flag.NewFlagSet("claude-sessions", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	flags.Usage = func() { fmt.Fprint(a.stderr, usage) }
	list := flags.Bool("list", false, "")
	showConfig := flags.Bool("config", false, "")
	useTmux := flags.Bool("tmux", false, "")
	jump := flags.Bool("jump", false, "")
	noTmux := flags.Bool("no-tmux", false, "")
	dryRun := flags.Bool("dry-run", a.dryRun, "")
	showVersion := flags.Bool("version", false, "")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	a.dryRun = *dryRun
	if *useTmux || *jump {
		a.cfg.Tmux = config.TmuxNewSession
	}
	if *noTmux {
		a.cfg.Tmux = config.TmuxOff
	}

	switch {
	case *showVersion:
		fmt.Fprintln(a.stdout, buildVersion())
		return 0
	case *showConfig:
		printConfig(a.stdout, a.paths, a.cfg)
		return 0
	case *list:
		entries, err := index.Load(a.indexOptions())
		if err != nil {
			return a.fail(err)
		}
		writeList(a.stdout, entries)
		return 0
	}
	return a.pick()
}

func (a *app) indexOptions() index.Options {
	return index.Options{
		ClaudeDir:    a.paths.ClaudeDir,
		CacheFile:    a.paths.CacheFile,
		Blocklist:    a.paths.Blocklist,
		Alive:        live.ProcessAlive,
		PaneResolver: launch.PaneResolver(a.cfg, a.env),
	}
}

func (a *app) asker() ask.Asker {
	return ask.Asker{Command: a.cfg.ClaudeCommand, Model: a.cfg.AskModel, Dir: a.paths.CacheDir}
}

func (a *app) pick() int {
	opts := a.indexOptions()
	asker := a.asker()
	model := tui.New(tui.Deps{
		Config: a.cfg,
		Load:   func() ([]index.Entry, error) { return index.Load(opts) },
		Block:  func(id string) error { return index.Block(opts.Blocklist, id) },
		End:    func(id string) error { return live.End(filepath.Join(opts.ClaudeDir, "sessions"), id) },
		Ask: func(ctx context.Context, q string, entries []index.Entry) ([]index.Entry, error) {
			return asker.Rank(ctx, q, entries)
		},
		Plan: func(action string, e index.Entry) (launch.Plan, error) {
			return launch.PlanLaunch(action, e, a.cfg, a.env)
		},
		Preview: func(e index.Entry, lines, columns int) (string, bool, bool) {
			return a.previewCommandOutput(e, lines, columns)
		},
	})

	final, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	if err != nil {
		return a.fail(err)
	}
	plan, ok := final.(tui.Model).Result()
	if !ok {
		return 0
	}
	return a.carryOut(plan)
}

func (a *app) previewCommandOutput(e index.Entry, lines, columns int) (string, bool, bool) {
	command, wrap, ok := preview.Command(a.cfg, e)
	if !ok {
		return "", false, false
	}
	out, err := preview.Run(command, lines, columns, !wrap)
	if err != nil {
		return err.Error(), true, true
	}
	return out, wrap, true
}

func (a *app) carryOut(plan launch.Plan) int {
	if a.dryRun {
		fmt.Fprintln(a.stdout, plan.String())
		return 0
	}
	if err := launch.Run(plan); err != nil {
		return a.fail(err)
	}
	return 0
}

func (a *app) fail(err error) int {
	fmt.Fprintln(a.stderr, "claude-sessions:", err)
	return 1
}

func writeList(w io.Writer, entries []index.Entry) {
	for _, e := range entries {
		mark := ""
		if e.Live {
			mark = "●"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			e.ID, e.ModTime.Format("2006-01-02 15:04"), mark, e.Project(), e.Branch,
			oneLine(e.Title()), e.Cwd, e.Pane, oneLine(e.SearchText()))
	}
}

func writeNote(w io.Writer, note string) {
	fmt.Fprintf(w, "\t\t\t\t\t%s\t\t\t\n", oneLine(note))
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
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

var scripting = map[string]func(a *app, args []string) int{
	"--resume":          func(a *app, args []string) int { return a.open(config.ActionResume, args, "") },
	"--fork":            func(a *app, args []string) int { return a.open(config.ActionFork, args, "") },
	"--action":          func(a *app, args []string) int { return a.keyAction(args, false) },
	"--jump-action":     func(a *app, args []string) int { return a.keyAction(args, true) },
	"--resumes-running": (*app).resumesRunning,
	"--attach-running":  (*app).attachRunning,
	"--preview":         (*app).printPreview,
	"--ask":             (*app).printRanking,
	"--has-turns":       hasTurns,
	"--opening-prompt":  openingPrompt,
	"--end":             (*app).end,
	"--block":           (*app).block,
}

func arg(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}
	return ""
}

func (a *app) keyAction(args []string, jump bool) int {
	if jump {
		a.cfg.Tmux = config.TmuxNewSession
	}
	action := a.cfg.DefaultAction
	switch arg(args, 0) {
	case "alt-enter", "ctrl-o", a.cfg.OtherAction():
		action = a.cfg.OtherAction()
	}
	return a.open(action, args[min(1, len(args)):], arg(args, 2))
}

func (a *app) open(action string, args []string, cwd string) int {
	e, err := index.Lookup(a.indexOptions(), arg(args, 0))
	if err != nil {
		return a.fail(err)
	}
	if cwd != "" {
		e.Cwd = cwd
	}
	plan, err := launch.PlanLaunch(action, e, a.cfg, a.env)
	if err != nil {
		return a.fail(err)
	}
	return a.carryOut(plan)
}

func (a *app) resumesRunning(args []string) int {
	id, ok := launch.ResumedID(args)
	if !ok {
		return 1
	}
	if _, running := live.Read(filepath.Join(a.paths.ClaudeDir, "sessions"), live.ProcessAlive)[id]; !running {
		return 1
	}
	fmt.Fprintln(a.stdout, id)
	return 0
}

func (a *app) attachRunning(args []string) int {
	e, err := index.Lookup(a.indexOptions(), arg(args, 0))
	if err != nil {
		return a.fail(err)
	}
	if !e.Live {
		return a.fail(fmt.Errorf("session %s is not running", e.ID))
	}
	plan, err := launch.PlanLaunch(config.ActionResume, e, a.cfg, a.env)
	if err != nil {
		return a.fail(err)
	}
	return a.carryOut(plan)
}

func (a *app) printPreview(args []string) int {
	e, err := index.Lookup(a.indexOptions(), arg(args, 0))
	if err != nil {
		fmt.Fprintln(a.stdout, err)
		return 0
	}
	if pane := arg(args, 2); pane != "" {
		e.Pane = ""
		if a.env.HasTmux && a.env.PaneAlive(pane) {
			e.Live, e.Pane = true, pane
		}
	}
	if out, _, ok := a.previewCommandOutput(e, previewLines(), previewColumns()); ok {
		fmt.Fprintln(a.stdout, out)
		return 0
	}
	fmt.Fprintln(a.stdout, tui.Summary(e))
	return 0
}

func previewLines() int   { return envInt("FZF_PREVIEW_LINES", 40) }
func previewColumns() int { return envInt("FZF_PREVIEW_COLUMNS", 100) }

func envInt(name string, fallback int) int {
	var n int
	if _, err := fmt.Sscan(os.Getenv(name), &n); err != nil || n <= 0 {
		return fallback
	}
	return n
}

func (a *app) printRanking(args []string) int {
	question := arg(args, 0)
	entries, err := index.Load(a.indexOptions())
	if err != nil {
		return a.fail(err)
	}
	ranked, err := a.asker().Rank(context.Background(), question, entries)
	switch {
	case err != nil:
		writeNote(a.stdout, "ask failed: "+err.Error())
	case len(ranked) == 0:
		writeNote(a.stdout, "no matching sessions for: "+question)
	default:
		writeList(a.stdout, ranked)
	}
	return 0
}

func hasTurns(_ *app, args []string) int {
	s, err := transcript.ParseFile(arg(args, 0))
	if err != nil || !s.HasTurns {
		return 1
	}
	return 0
}

func openingPrompt(a *app, args []string) int {
	s, err := transcript.ParseFile(arg(args, 0))
	if err != nil {
		return 1
	}
	fmt.Fprintln(a.stdout, s.OpeningPrompt)
	return 0
}

func (a *app) end(args []string) int {
	if err := live.End(filepath.Join(a.paths.ClaudeDir, "sessions"), arg(args, 0)); err != nil {
		return a.fail(err)
	}
	return 0
}

func (a *app) block(args []string) int {
	if err := index.Block(a.paths.Blocklist, arg(args, 0)); err != nil {
		return a.fail(err)
	}
	return 0
}
