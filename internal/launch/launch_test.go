package launch

import (
	"errors"
	"strings"
	"testing"

	"github.com/91dave/claude-sessions/internal/config"
	"github.com/91dave/claude-sessions/internal/index"
	"github.com/91dave/claude-sessions/internal/live"
	"github.com/91dave/claude-sessions/internal/transcript"
)

const id = "aaaaaaaa-0000-0000-0000-00000000dead"

type fakeTmux struct {
	panes    map[string]bool
	sessions map[string]bool
}

func env(goos string, inTmux, hasTmux bool, tmux fakeTmux) Env {
	return Env{
		GOOS:          goos,
		InTmux:        inTmux,
		HasTmux:       hasTmux,
		PaneAlive:     func(p string) bool { return tmux.panes[p] },
		SessionExists: func(s string) bool { return tmux.sessions[s] },
		DirExists:     func(d string) bool { return d != "/gone" },
	}
}

func entry(cwd string, running bool, tmuxTarget string) index.Entry {
	e := index.Entry{ID: id, Session: transcript.Session{Cwd: cwd}}
	if running {
		e.Live = true
		e.Marker = live.Marker{SessionID: id, PID: 42, Tmux: tmuxTarget}
	}
	return e
}

func cfg(tmux string) config.Config {
	c := config.Defaults()
	c.Tmux = tmux
	return c
}

func plan(t *testing.T, action string, e index.Entry, c config.Config, ev Env) Plan {
	t.Helper()
	p, err := PlanLaunch(action, e, c, ev)
	if err != nil {
		t.Fatalf("PlanLaunch: %v", err)
	}
	return p
}

func TestASessionThatIsNotRunningIsResumedInItsFolder(t *testing.T) {
	p := plan(t, config.ActionResume, entry("/tmp/proj", false, ""), cfg("off"), env("linux", false, true, fakeTmux{}))
	if got := p.String(); got != "cd /tmp/proj && claude --resume "+id {
		t.Errorf("plan = %q", got)
	}
}

func TestForkAddsForkSession(t *testing.T) {
	p := plan(t, config.ActionFork, entry("/tmp/proj", false, ""), cfg("off"), env("linux", false, true, fakeTmux{}))
	if got := p.String(); got != "cd /tmp/proj && claude --resume "+id+" --fork-session" {
		t.Errorf("plan = %q", got)
	}
}

func TestTheConfiguredClaudeCommandIsUsed(t *testing.T) {
	c := cfg("off")
	c.ClaudeCommand = "claude-beta"
	p := plan(t, config.ActionResume, entry("/tmp/proj", false, ""), c, env("linux", false, false, fakeTmux{}))
	if !strings.Contains(p.String(), "claude-beta --resume") {
		t.Errorf("plan = %q", p.String())
	}
}

func TestAFolderThatHasGoneIsRefused(t *testing.T) {
	_, err := PlanLaunch(config.ActionResume, entry("/gone", false, ""), cfg("off"), env("linux", false, true, fakeTmux{}))
	if err == nil || !strings.Contains(err.Error(), "/gone") {
		t.Errorf("err = %v", err)
	}
}

func TestARunningSessionInALivePaneIsJumpedTo(t *testing.T) {
	tmux := fakeTmux{panes: map[string]bool{"%3": true}}
	e := entry("/tmp/proj", true, "cc-proj:@1.%3")

	inside := plan(t, config.ActionResume, e, cfg("off"), env("linux", true, true, tmux)).String()
	want := "tmux select-window -t @1\ntmux select-pane -t %3\ntmux switch-client -t cc-proj"
	if inside != want {
		t.Errorf("inside tmux plan = %q, want %q", inside, want)
	}

	outside := plan(t, config.ActionResume, e, cfg("off"), env("linux", false, true, tmux)).String()
	want = "tmux select-window -t @1\ntmux select-pane -t %3\ntmux attach-session -t cc-proj"
	if outside != want {
		t.Errorf("outside tmux plan = %q, want %q", outside, want)
	}
}

func TestARunningSessionIsNeverResumedASecondTime(t *testing.T) {
	cases := map[string]struct {
		e  index.Entry
		ev Env
	}{
		"its pane has gone":    {entry("/tmp/proj", true, "cc-proj:@1.%3"), env("linux", true, true, fakeTmux{})},
		"it runs outside tmux": {entry("/tmp/proj", true, ""), env("linux", true, true, fakeTmux{})},
		"tmux is not installed": {
			entry("/tmp/proj", true, "cc-proj:@1.%3"),
			env("linux", false, false, fakeTmux{panes: map[string]bool{"%3": true}}),
		},
		"on Windows": {entry(`C:\Code\proj`, true, ""), env("windows", false, false, fakeTmux{})},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := PlanLaunch(config.ActionResume, c.e, cfg("off"), c.ev)
			if !errors.Is(err, ErrAlreadyRunning) {
				t.Errorf("err = %v, want ErrAlreadyRunning", err)
			}
		})
	}
}

func TestForkingARunningSessionGoesAhead(t *testing.T) {
	p := plan(t, config.ActionFork, entry("/tmp/proj", true, ""), cfg("off"), env("linux", false, true, fakeTmux{}))
	if !strings.Contains(p.String(), "--fork-session") {
		t.Errorf("plan = %q", p.String())
	}
}

func TestNewSessionModeResumesInAFreshTmuxSession(t *testing.T) {
	tmux := fakeTmux{sessions: map[string]bool{"cc-proj": true, "cc-proj-2": true}}
	p := plan(t, config.ActionResume, entry("/tmp/proj", false, ""), cfg("new-session"), env("linux", true, true, tmux))
	want := "tmux new-session -d -s cc-proj-3 -c /tmp/proj claude --resume " + id + "\ntmux switch-client -t =cc-proj-3"
	if got := p.String(); got != want {
		t.Errorf("plan = %q, want %q", got, want)
	}

	p = plan(t, config.ActionResume, entry("/tmp/proj", false, ""), cfg("new-session"), env("linux", false, true, fakeTmux{}))
	if !strings.HasSuffix(p.String(), "tmux attach-session -t =cc-proj") {
		t.Errorf("outside tmux plan = %q", p.String())
	}
}

func TestTmuxSessionNamesAvoidCharactersTmuxRejects(t *testing.T) {
	p := plan(t, config.ActionResume, entry("/tmp/my.app:v2", false, ""), cfg("new-session"), env("linux", false, true, fakeTmux{}))
	if !strings.Contains(p.String(), "-s cc-my_app_v2 ") {
		t.Errorf("plan = %q", p.String())
	}
}

func TestNewSessionModeIsIgnoredWithoutTmux(t *testing.T) {
	for name, ev := range map[string]Env{
		"on Windows":         env("windows", false, true, fakeTmux{}),
		"tmux not installed": env("linux", false, false, fakeTmux{}),
	} {
		t.Run(name, func(t *testing.T) {
			p := plan(t, config.ActionResume, entry("/tmp/proj", false, ""), cfg("new-session"), ev)
			if strings.Contains(p.String(), "tmux") {
				t.Errorf("plan = %q", p.String())
			}
		})
	}
}

func TestTmuxIsOffByDefault(t *testing.T) {
	p := plan(t, config.ActionResume, entry("/tmp/proj", false, ""), config.Defaults(), env("linux", false, true, fakeTmux{}))
	if strings.Contains(p.String(), "tmux") {
		t.Errorf("plan = %q", p.String())
	}
}

func TestDryRunQuotesArgumentsWithSpaces(t *testing.T) {
	p := plan(t, config.ActionResume, entry("/tmp/my proj", false, ""), cfg("off"), env("linux", false, false, fakeTmux{}))
	if got := p.String(); got != `cd "/tmp/my proj" && claude --resume `+id {
		t.Errorf("plan = %q", got)
	}
}
