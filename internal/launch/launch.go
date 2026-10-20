package launch

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/91dave/claude-sessions/internal/config"
	"github.com/91dave/claude-sessions/internal/index"
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
	Dir  string
	Args []string
}

type Plan struct {
	Steps []Step
}

func PlanLaunch(action string, e index.Entry, c config.Config, env Env) (Plan, error) {
	if action == config.ActionResume && e.Live {
		return planJumpToRunning(e, env)
	}
	if !env.DirExists(e.Cwd) {
		return Plan{}, fmt.Errorf("directory no longer exists: %s", e.Cwd)
	}
	claude := []string{c.ClaudeCommand, "--resume", e.ID}
	if action == config.ActionFork {
		claude = append(claude, "--fork-session")
	}
	if c.Tmux == config.TmuxNewSession && env.tmuxUsable() {
		return planNewTmuxSession(e, c, env, claude), nil
	}
	return Plan{Steps: []Step{{Dir: e.Cwd, Args: claude}}}, nil
}

func planJumpToRunning(e index.Entry, env Env) (Plan, error) {
	m := e.Marker
	pane := m.TmuxPane()
	if pane == "" || !env.tmuxUsable() || !env.PaneAlive(pane) {
		return Plan{}, fmt.Errorf("session %s is %w outside a reachable tmux pane; fork it instead", e.ID, ErrAlreadyRunning)
	}
	return Plan{Steps: []Step{
		{Args: []string{"tmux", "select-window", "-t", m.TmuxWindow()}},
		{Args: []string{"tmux", "select-pane", "-t", pane}},
		{Args: arriveAt(m.TmuxSession(), env)},
	}}, nil
}

func planNewTmuxSession(e index.Entry, c config.Config, env Env, claude []string) Plan {
	name := freeSessionName(c.TmuxPrefix+tmuxSafe(e.Project()), env.SessionExists)
	start := append([]string{"tmux", "new-session", "-d", "-s", name, "-c", e.Cwd}, claude...)
	return Plan{Steps: []Step{
		{Args: start},
		{Args: arriveAt("="+name, env)},
	}}
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
