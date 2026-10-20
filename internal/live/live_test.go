package live

import (
	"os"
	"path/filepath"
	"testing"
)

func writeMarker(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func aliveOnly(pids ...int) func(int) bool {
	return func(pid int) bool {
		for _, p := range pids {
			if p == pid {
				return true
			}
		}
		return false
	}
}

func TestReadKeepsOnlyRunningProcesses(t *testing.T) {
	dir := t.TempDir()
	writeMarker(t, dir, "100.json", `{"pid":100,"sessionId":"running","cwd":"/p","tmux":"cc-p:@1.%2"}`)
	writeMarker(t, dir, "200.json", `{"pid":200,"sessionId":"exited","cwd":"/p"}`)

	markers := Read(dir, aliveOnly(100))

	if _, ok := markers["exited"]; ok {
		t.Error("a marker left by a process that has exited counts as running")
	}
	m, ok := markers["running"]
	if !ok {
		t.Fatal("a running session is missing")
	}
	if m.PID != 100 || m.Tmux != "cc-p:@1.%2" {
		t.Errorf("marker = %+v", m)
	}
}

func TestHalfWrittenMarkerDoesNotHideOthers(t *testing.T) {
	dir := t.TempDir()
	writeMarker(t, dir, "100.json", `{"pid":100,"sessionId":"runn`)
	writeMarker(t, dir, "200.json", `{"pid":200,"sessionId":"also-running"}`)

	if _, ok := Read(dir, aliveOnly(100, 200))["also-running"]; !ok {
		t.Error("a half-written marker hid a running session")
	}
}

func TestMissingDirectoryMeansNothingRuns(t *testing.T) {
	if n := len(Read(filepath.Join(t.TempDir(), "absent"), aliveOnly(1))); n != 0 {
		t.Errorf("got %d markers, want 0", n)
	}
}

func TestOnlyJSONFilesAreMarkers(t *testing.T) {
	dir := t.TempDir()
	writeMarker(t, dir, "100.key", `{"pid":100,"sessionId":"not-a-marker"}`)
	if n := len(Read(dir, aliveOnly(100))); n != 0 {
		t.Errorf("got %d markers, want 0", n)
	}
}

func TestTmuxTarget(t *testing.T) {
	m := Marker{Tmux: "cc-repos:@12.%26"}
	if m.TmuxSession() != "cc-repos" || m.TmuxWindow() != "@12" || m.TmuxPane() != "%26" {
		t.Errorf("session, window, pane = %q, %q, %q", m.TmuxSession(), m.TmuxWindow(), m.TmuxPane())
	}
	if (Marker{}).TmuxPane() != "" {
		t.Error("a marker without a tmux target reported a pane")
	}
}

func TestThisProcessIsAlive(t *testing.T) {
	if !ProcessAlive(os.Getpid()) {
		t.Error("the test process is not alive")
	}
	if ProcessAlive(0) || ProcessAlive(-1) {
		t.Error("an invalid pid is alive")
	}
}
