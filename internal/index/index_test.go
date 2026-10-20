package index

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const caveat = `{"type":"user","isMeta":true,"message":{"role":"user","content":"<local-command-caveat>Caveat</local-command-caveat>"}}`
const loginOnly = caveat + "\n" + `{"type":"user","cwd":"/tmp/proj","gitBranch":"main","message":{"role":"user","content":"<command-name>/login</command-name>"}}` + "\n"
const reply = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}` + "\n"

type fixture struct {
	t       *testing.T
	claude  string
	project string
	opts    Options
	running map[int]bool
}

func newFixture(t *testing.T) *fixture {
	root := t.TempDir()
	f := &fixture{t: t, claude: filepath.Join(root, "claude"), running: map[int]bool{}}
	f.project = filepath.Join(f.claude, "projects", "-tmp-proj")
	for _, d := range []string{f.project, filepath.Join(f.claude, "sessions")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.opts = Options{
		ClaudeDir: f.claude,
		CacheFile: filepath.Join(root, "cache", "cache.json"),
		Blocklist: filepath.Join(root, "config", "blocklist"),
		Alive:     func(pid int) bool { return f.running[pid] },
	}
	return f
}

func (f *fixture) transcript(id, body string, modified time.Time) {
	f.t.Helper()
	path := filepath.Join(f.project, id+".jsonl")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		f.t.Fatal(err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) markRunning(id string, pid int, tmux string) {
	f.t.Helper()
	f.running[pid] = true
	body := `{"sessionId":"` + id + `","pid":` + strconv.Itoa(pid) + `,"cwd":"/tmp/proj","tmux":"` + tmux + `"}`
	if err := os.WriteFile(filepath.Join(f.claude, "sessions", strconv.Itoa(pid)+".json"), []byte(body), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) load() []Entry {
	f.t.Helper()
	entries, err := Load(f.opts)
	if err != nil {
		f.t.Fatalf("Load: %v", err)
	}
	return entries
}

func ids(entries []Entry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.ID)
	}
	return out
}

const dead = "aaaaaaaa-0000-0000-0000-00000000dead"

func TestTurnlessSessionsAreHiddenUnlessRunning(t *testing.T) {
	f := newFixture(t)
	f.transcript(dead, loginOnly, time.Now())

	if n := len(f.load()); n != 0 {
		t.Fatalf("a turn-less session is listed (%d rows)", n)
	}

	f.markRunning(dead, 4242, "")
	entries := f.load()
	if len(entries) != 1 || !entries[0].Live {
		t.Fatalf("a turn-less session still running is not listed as live: %+v", entries)
	}

	f.running = map[int]bool{}
	f.transcript(dead, loginOnly+reply, time.Now().Add(time.Second))
	if n := len(f.load()); n != 1 {
		t.Errorf("a session that gained a turn did not come back (%d rows)", n)
	}
}

func TestRunningSessionCarriesItsMarker(t *testing.T) {
	f := newFixture(t)
	f.transcript(dead, loginOnly+reply, time.Now())
	f.markRunning(dead, 4242, "cc-proj:@1.%3")
	e := f.load()[0]
	if e.Marker.PID != 4242 || e.Marker.TmuxPane() != "%3" {
		t.Errorf("marker = %+v", e.Marker)
	}
}

func TestNewestFirst(t *testing.T) {
	f := newFixture(t)
	now := time.Now()
	f.transcript("older-session", loginOnly+reply, now.Add(-time.Hour))
	f.transcript("newer-session", loginOnly+reply, now)
	f.transcript("oldest-session", loginOnly+reply, now.Add(-2*time.Hour))

	got := strings.Join(ids(f.load()), " ")
	if got != "newer-session older-session oldest-session" {
		t.Errorf("order = %s", got)
	}
}

func TestEmptyAndUserlessTranscriptsAreSkipped(t *testing.T) {
	f := newFixture(t)
	f.transcript("empty-session", "", time.Now())
	f.transcript("summary-only", `{"type":"summary","summary":"x"}`+"\n", time.Now())
	if n := len(f.load()); n != 0 {
		t.Errorf("got %d rows, want 0", n)
	}
}

func TestBlockedSessionsAreHidden(t *testing.T) {
	f := newFixture(t)
	f.transcript("keep-session", loginOnly+reply, time.Now())
	f.transcript("hide-session", loginOnly+reply, time.Now())

	if err := Block(f.opts.Blocklist, "hide-session"); err != nil {
		t.Fatal(err)
	}
	if err := Block(f.opts.Blocklist, "hide-session"); err != nil {
		t.Fatal(err)
	}

	if got := strings.Join(ids(f.load()), " "); got != "keep-session" {
		t.Errorf("listed = %s", got)
	}
	data, _ := os.ReadFile(f.opts.Blocklist)
	if string(data) != "hide-session\n" {
		t.Errorf("blocklist = %q, want one line", data)
	}
}

func TestCacheIsReusedWhileTheFileIsUnchanged(t *testing.T) {
	f := newFixture(t)
	stamp := time.Now().Add(-time.Minute).Truncate(time.Second)
	body := `{"type":"user","cwd":"/tmp/proj","message":{"role":"user","content":"original ask"}}` + "\n" + reply
	f.transcript("cached-session", body, stamp)
	f.load()

	swapped := strings.Replace(body, "original ask", "replaced ask", 1)
	f.transcript("cached-session", swapped, stamp)
	if got := f.load()[0].OpeningPrompt; got != "original ask" {
		t.Errorf("an unchanged file was re-parsed: %q", got)
	}

	f.transcript("cached-session", swapped, stamp.Add(time.Second))
	if got := f.load()[0].OpeningPrompt; got != "replaced ask" {
		t.Errorf("a changed file was not re-parsed: %q", got)
	}
}

func TestACorruptCacheIsIgnored(t *testing.T) {
	f := newFixture(t)
	f.transcript("some-session", loginOnly+reply, time.Now())
	os.MkdirAll(filepath.Dir(f.opts.CacheFile), 0o755)
	os.WriteFile(f.opts.CacheFile, []byte("{not json"), 0o644)
	if n := len(f.load()); n != 1 {
		t.Errorf("got %d rows, want 1", n)
	}
}

func TestMatch(t *testing.T) {
	f := newFixture(t)
	f.transcript("masthead-session", `{"type":"user","cwd":"/tmp/proj","gitBranch":"feature/header","message":{"role":"user","content":"How does the masthead get updated?"}}
`+reply+`{"type":"ai-title","aiTitle":"Masthead publishing"}
{"type":"summary","summary":"Traced the publish orchestrator"}
{"type":"last-prompt","lastPrompt":"check the swap and drop step"}
`, time.Now())
	e := f.load()[0]

	cases := map[string]bool{
		"":                      true,
		"masthead":              true,
		"MASTHEAD publishing":   true,
		"proj":                  true,
		"feature/header":        true,
		"orchestrator swap":     true,
		"masthead gateway":      false,
		"   masthead   updated": true,
	}
	for query, want := range cases {
		if got := Match(e, query); got != want {
			t.Errorf("Match(%q) = %v, want %v", query, got, want)
		}
	}
}
