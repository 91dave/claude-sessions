package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/91dave/claude-sessions/internal/index"
	"github.com/91dave/claude-sessions/internal/transcript"
)

func TestListPrintsOneTabSeparatedRowPerSession(t *testing.T) {
	stamp := time.Date(2026, 10, 1, 9, 30, 0, 0, time.Local)
	entries := []index.Entry{
		{ID: "live-session", ModTime: stamp, Live: true, Pane: "%3", Session: transcript.Session{
			Cwd: "/tmp/proj", Branch: "main", AITitle: "Masthead\tpublishing", OpeningPrompt: "how\tdoes it",
		}},
		{ID: "old-session", ModTime: stamp, Session: transcript.Session{Cwd: `C:\Code\quartex`, OpeningPrompt: "hi"}},
	}
	var out bytes.Buffer
	writeList(&out, entries)

	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines:\n%s", len(lines), out.String())
	}
	want := "live-session\t2026-10-01 09:30\t●\tproj\tmain\tMasthead publishing\t/tmp/proj\t%3\thow does it"
	if lines[0] != want {
		t.Errorf("row = %q\nwant  %q", lines[0], want)
	}
	if fields := strings.Split(lines[1], "\t"); len(fields) != 9 || fields[2] != "" || fields[3] != "quartex" || fields[7] != "" {
		t.Errorf("row = %q", lines[1])
	}
}
