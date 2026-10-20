package launch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/91dave/claude-sessions/internal/config"
	"github.com/91dave/claude-sessions/internal/index"
	"github.com/91dave/claude-sessions/internal/live"
	"github.com/91dave/claude-sessions/internal/shellcmd"
)

var ErrAlreadyRunning = errors.New("already running")

type Env struct {
	GOOS          string
	InTmux        bool
	HasTmux       bool
	PaneAlive     func(pane string) bool
	SessionExists func(name string) bool
	DirExists     func(dir string) bool
}

func CurrentEnv() Env {
	_, err := exec.LookPath("tmux")
	return Env{
		GOOS:          runtime.GOOS,
		InTmux:        os.Getenv("TMUX") != "",
		HasTmux:       err == nil,
		PaneAlive:     tmuxPaneAlive,
		SessionExists: tmuxSessionExists,
		DirExists: func(dir string) bool {
			info, err := os.Stat(dir)
			return err == nil && info.IsDir()
		},
	}
}

func (e Env) tmuxUsable() bool {
	return e.HasTmux && e.GOOS != "windows"
}

type Step struct {
	Dir   string
	Args  []string
	Shell string
}

func shellStep(template string, vars shellcmd.Vars) Step {
	command := shellcmd.Expand(template, vars)
	return Step{Args: shellcmd.Argv(command), Shell: command}
}

type Plan struct {
	Steps []Step
}

func PlanLaunch(action string, e index.Entry, c config.Config, env Env) (Plan, error) {
	if action == config.ActionResume && e.Live {
		return planJumpToRunning(e, c, env)
	}
	if !env.DirExists(e.Cwd) {
		return Plan{}, fmt.Errorf("directory no longer exists: %s", e.Cwd)
	}
	claude := []string{c.ClaudeCommand, "--resume", e.ID}
	if action == config.ActionFork {
		claude = append(claude, "--fork-session")
	}
	if wantsTmuxSession(c, env) {
		return planNewTmuxSession(e, c, env, claude), nil
	}
	return Plan{Steps: []Step{{Dir: e.Cwd, Args: claude}}}, nil
}

func wantsTmuxSession(c config.Config, env Env) bool {
	switch {
	case !env.tmuxUsable():
		return false
	case c.Tmux == config.TmuxNewSession:
		return true
	case c.Tmux == config.TmuxAuto:
		return !env.InTmux
	}
	return false
}

func Vars(e index.Entry) shellcmd.Vars {
	m := e.Marker
	pid := ""
	if m.PID > 0 {
		pid = strconv.Itoa(m.PID)
	}
	return shellcmd.Vars{}.
		Set("id", e.ID).
		Set("cwd", e.Cwd).
		Set("pid", pid).
		Set("pane", m.TmuxPane()).
		Set("target", m.Tmux).
		Set("session", m.TmuxSession()).
		Set("window", m.TmuxWindow()).
		Set("transcript", e.Path)
}

func planJumpToRunning(e index.Entry, c config.Config, env Env) (Plan, error) {
	m := e.Marker
	pane := m.TmuxPane()
	if pane == "" || !env.tmuxUsable() || !env.PaneAlive(pane) {
		return Plan{}, fmt.Errorf("session %s is %w outside a reachable tmux pane; fork it instead", e.ID, ErrAlreadyRunning)
	}
	if c.Commands.Attach != "" {
		return Plan{Steps: []Step{shellStep(c.Commands.Attach, Vars(e))}}, nil
	}
	return Plan{Steps: []Step{
		{Args: []string{"tmux", "select-window", "-t", m.TmuxWindow()}},
		{Args: []string{"tmux", "select-pane", "-t", pane}},
		{Args: arriveAt(m.TmuxSession(), env)},
	}}, nil
}

func planNewTmuxSession(e index.Entry, c config.Config, env Env, claude []string) Plan {
	name := freeSessionName(c.TmuxPrefix+tmuxSafe(e.Project()), env.SessionExists)
	if c.Commands.ReviveTmux != "" {
		vars := Vars(e).Set("tmuxName", name).SetArgs("claude", claude)
		return Plan{Steps: []Step{shellStep(c.Commands.ReviveTmux, vars)}}
	}
	start := append([]string{"tmux", "new-session", "-d", "-s", name, "-c", e.Cwd}, claude...)
	return Plan{Steps: []Step{
		{Args: start},
		{Args: arriveAt("="+name, env)},
	}}
}

const livePaneTimeout = 3 * time.Second

func PaneResolver(c config.Config, env Env) func(live.Marker) string {
	return func(m live.Marker) string {
		if !env.tmuxUsable() {
			return ""
		}
		pane := m.TmuxPane()
		if c.Commands.LivePane != "" {
			pane = askForPane(c.Commands.LivePane, m)
		}
		if pane == "" || !env.PaneAlive(pane) {
			return ""
		}
		return pane
	}
}

func askForPane(template string, m live.Marker) string {
	e := index.Entry{ID: m.SessionID, Marker: m}
	e.Cwd = m.Cwd
	ctx, cancel := context.WithTimeout(context.Background(), livePaneTimeout)
	defer cancel()
	out, err := shellcmd.Output(ctx, shellcmd.Expand(template, Vars(e)), nil)
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	fields := strings.Split(first, "\t")
	return strings.TrimSpace(fields[len(fields)-1])
}

func ResumedID(args []string) (string, bool) {
	for _, a := range args {
		if a == "--fork-session" {
			return "", false
		}
	}
	for i, a := range args {
		if (a == "--resume" || a == "-r") && i+1 < len(args) {
			return args[i+1], true
		}
		if id, ok := strings.CutPrefix(a, "--resume="); ok && id != "" {
			return id, true
		}
	}
	return "", false
}

func arriveAt(target string, env Env) []string {
	if env.InTmux {
		return []string{"tmux", "switch-client", "-t", target}
	}
	return []string{"tmux", "attach-session", "-t", target}
}

func tmuxSafe(name string) string {
	return strings.NewReplacer(".", "_", ":", "_").Replace(name)
}

func freeSessionName(base string, exists func(string) bool) string {
	name := base
	for n := 2; exists(name); n++ {
		name = base + "-" + strconv.Itoa(n)
	}
	return name
}

func (p Plan) String() string {
	lines := make([]string, len(p.Steps))
	for i, s := range p.Steps {
		if s.Shell != "" {
			lines[i] = s.Shell
			continue
		}
		quoted := make([]string, len(s.Args))
		for j, a := range s.Args {
			quoted[j] = quote(a)
		}
		lines[i] = strings.Join(quoted, " ")
		if s.Dir != "" {
			lines[i] = "cd " + quote(s.Dir) + " && " + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func quote(arg string) string {
	if arg == "" || strings.ContainsAny(arg, " \t\"'") {
		return strconv.Quote(arg)
	}
	return arg
}

func Run(p Plan) error {
	if len(p.Steps) == 0 {
		return nil
	}
	last := len(p.Steps) - 1
	for _, s := range p.Steps[:last] {
		cmd := exec.Command(s.Args[0], s.Args[1:]...)
		cmd.Dir = s.Dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w: %s", strings.Join(s.Args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	final := p.Steps[last]
	path, err := exec.LookPath(final.Args[0])
	if err != nil {
		return err
	}
	return replaceProcess(path, final.Args, final.Dir)
}

func tmuxPaneAlive(pane string) bool {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", pane, "#{pane_id}").Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func tmuxSessionExists(name string) bool {
	return exec.Command("tmux", "has-session", "-t", "="+name).Run() == nil
}
