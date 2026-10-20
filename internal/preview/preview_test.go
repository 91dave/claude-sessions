package preview

import (
	"runtime"
	"strings"
	"testing"

	"github.com/91dave/claude-sessions/internal/config"
	"github.com/91dave/claude-sessions/internal/index"
	"github.com/91dave/claude-sessions/internal/live"
	"github.com/91dave/claude-sessions/internal/transcript"
)

func running(pane string) index.Entry {
	return index.Entry{ID: "abc", Live: true, Pane: pane,
		Marker:  live.Marker{SessionID: "abc", PID: 42, Tmux: "cc-proj:@1.%1"},
		Session: transcript.Session{Cwd: "/tmp/proj"}}
}

func finished() index.Entry {
	return index.Entry{ID: "abc", Path: "/t/abc.jsonl", Session: transcript.Session{Cwd: "/tmp/proj"}}
}

func TestARunningSessionWithAPaneIsCapturedByDefault(t *testing.T) {
	cmd, wrap, ok := Command(config.Defaults(), running("%3"))
	if !ok || wrap || cmd != "tmux capture-pane -ep -t %3" {
		t.Errorf("cmd, wrap, ok = %q, %v, %v", cmd, wrap, ok)
	}
}

func TestTheConfiguredRunningPreviewUsesTheResolvedPane(t *testing.T) {
	c := config.Defaults()
	c.Commands.PreviewRunning = "agent-state preview {pane}"
	cmd, wrap, ok := Command(c, running("%3"))
	if !ok || wrap || cmd != "agent-state preview %3" {
		t.Errorf("cmd, wrap, ok = %q, %v, %v", cmd, wrap, ok)
	}
}

func TestARunningSessionWithoutAPaneFallsBackToTheSummary(t *testing.T) {
	if _, _, ok := Command(config.Defaults(), running("")); ok {
		t.Error("a command was offered for a session with no pane")
	}
}

func TestAFinishedSessionUsesTheSummaryUnlessConfigured(t *testing.T) {
	if _, _, ok := Command(config.Defaults(), finished()); ok {
		t.Error("a command was offered by default")
	}
	c := config.Defaults()
	c.Commands.PreviewFinished = "my-previewer {transcript}"
	cmd, wrap, ok := Command(c, finished())
	if !ok || !wrap || cmd != "my-previewer /t/abc.jsonl" {
		t.Errorf("cmd, wrap, ok = %q, %v, %v", cmd, wrap, ok)
	}
}

func TestRunPassesThePreviewSizeAndKeepsTheLastLines(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell")
	}
	out, err := Run(`echo "$FZF_PREVIEW_COLUMNS x $FZF_PREVIEW_LINES"; seq 1 10; printf '\n\n'`, 3, 80, true)
	if err != nil {
		t.Fatal(err)
	}
	if out != "8\n9\n10" {
		t.Errorf("out = %q", out)
	}
	out, err = Run(`echo "$FZF_PREVIEW_COLUMNS x $FZF_PREVIEW_LINES"`, 3, 80, false)
	if err != nil || strings.TrimSpace(out) != "80 x 3" {
		t.Errorf("out, err = %q, %v", out, err)
	}
}
