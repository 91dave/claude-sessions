package ask

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/91dave/claude-sessions/internal/index"
	"github.com/91dave/claude-sessions/internal/transcript"
)

const (
	masthead = "cccccccc-0000-0000-0000-00000masthead"
	gateway  = "dddddddd-0000-0000-0000-000000gateway"
)

type call struct {
	name, dir, stdin string
	args             []string
}

func fakeRunner(reply string, err error, got *call) Runner {
	return func(_ context.Context, name string, args []string, dir, stdin string) (string, error) {
		*got = call{name: name, args: args, dir: dir, stdin: stdin}
		return reply, err
	}
}

func entries() []index.Entry {
	return []index.Entry{
		{ID: masthead, ModTime: time.Date(2026, 10, 1, 9, 30, 0, 0, time.Local), Session: transcript.Session{
			Cwd: "/tmp/proj", Branch: "main", AITitle: "Masthead publishing",
			OpeningPrompt: "How does the masthead get updated?", LastPrompt: "check the swap and drop step",
		}},
		{ID: gateway, Session: transcript.Session{Cwd: "/tmp/proj", Branch: "main", OpeningPrompt: "/qtx-logs gateway errors"}},
	}
}

func asker(run Runner) Asker {
	return Asker{Command: "claude", Model: "haiku", Dir: "/cache", Run: run}
}

func TestSessionsComeBackInTheOrderClaudeRanked(t *testing.T) {
	var got call
	reply := gateway + "\nnot-a-session\n" + masthead + "\n" + gateway + "\n"
	ranked, err := asker(fakeRunner(reply, nil, &got)).Rank(context.Background(), "where did the header publishing go", entries())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range ranked {
		ids = append(ids, e.ID)
	}
	if strings.Join(ids, " ") != gateway+" "+masthead {
		t.Errorf("ranked = %v", ids)
	}
}

func TestTheQuestionAndSessionDetailsAreSent(t *testing.T) {
	var got call
	_, err := asker(fakeRunner("", nil, &got)).Rank(context.Background(), "where did the header publishing go", entries())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"where did the header publishing go", "swap and drop", masthead + "\t", "Masthead publishing", "2026-10-01 09:30"} {
		if !strings.Contains(got.stdin, want) {
			t.Errorf("stdin lacks %q:\n%s", want, got.stdin)
		}
	}
	if got.name != "claude" || got.dir != "/cache" {
		t.Errorf("ran %q in %q", got.name, got.dir)
	}
	for _, want := range []string{"-p", "--no-session-persistence", "--strict-mcp-config"} {
		if !slices.Contains(got.args, want) {
			t.Errorf("args %v lack %s", got.args, want)
		}
	}
	if i := slices.Index(got.args, "--model"); i < 0 || got.args[i+1] != "haiku" {
		t.Errorf("args %v do not pick the haiku model", got.args)
	}
	if i := slices.Index(got.args, "--tools"); i < 0 || got.args[i+1] != "" {
		t.Errorf("args %v do not turn tools off", got.args)
	}
}

func TestNoMatchesIsEmptyNotAnError(t *testing.T) {
	var got call
	ranked, err := asker(fakeRunner("\n", nil, &got)).Rank(context.Background(), "nothing like this", entries())
	if err != nil || len(ranked) != 0 {
		t.Errorf("ranked, err = %v, %v", ranked, err)
	}
}

func TestAFailedCallIsAnError(t *testing.T) {
	var got call
	_, err := asker(fakeRunner("", errors.New("exit status 1"), &got)).Rank(context.Background(), "anything", entries())
	if err == nil {
		t.Error("a failed call was not reported")
	}
}
