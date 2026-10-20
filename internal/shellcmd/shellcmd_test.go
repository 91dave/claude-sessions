package shellcmd

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestExpandSubstitutesQuotedValues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX quoting")
	}
	vars := Vars{}.Set("pane", "%3").Set("cwd", "/tmp/my proj").Set("name", "it's")
	got := Expand("agent-state jump {pane} && cd {cwd} && echo {name}", vars)
	want := `agent-state jump %3 && cd '/tmp/my proj' && echo 'it'\''s'`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestExpandSplicesArgumentLists(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX quoting")
	}
	vars := Vars{}.SetArgs("claude", []string{"claude", "--resume", "abc", "--model", "two words"})
	if got := Expand("tmux new-session -d {claude}", vars); got != "tmux new-session -d claude --resume abc --model 'two words'" {
		t.Errorf("got %s", got)
	}
}

func TestExpandLeavesUnknownPlaceholdersAlone(t *testing.T) {
	if got := Expand("echo {unknown} {}", Vars{}); got != "echo {unknown} {}" {
		t.Errorf("got %s", got)
	}
}

func TestEmptyValuesAreQuoted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX quoting")
	}
	if got := Expand("x {pane} y", Vars{}.Set("pane", "")); got != "x '' y" {
		t.Errorf("got %s", got)
	}
}

func TestOutputRunsThroughTheShellWithEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell")
	}
	out, err := Output(context.Background(), `printf '%s|%s' "$GREETING" hello`, []string{"GREETING=hi"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "hi|hello" {
		t.Errorf("out = %q", out)
	}
}

func TestOutputReportsFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell")
	}
	_, err := Output(context.Background(), "echo broken >&2; exit 3", nil)
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Errorf("err = %v", err)
	}
}

func TestArgvUsesThePlatformShell(t *testing.T) {
	argv := Argv("echo hi")
	if argv[len(argv)-1] != "echo hi" || len(argv) != 3 {
		t.Errorf("argv = %q", argv)
	}
}
