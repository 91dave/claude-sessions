package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/91dave/claude-sessions/internal/config"
	"github.com/91dave/claude-sessions/internal/index"
	"github.com/91dave/claude-sessions/internal/launch"
	"github.com/91dave/claude-sessions/internal/transcript"
)

func session(id, title, prompt string) index.Entry {
	return index.Entry{ID: id, Session: transcript.Session{
		HasUser: true, HasTurns: true, Cwd: "/tmp/proj", Branch: "main", AITitle: title, OpeningPrompt: prompt,
	}}
}

var (
	masthead = session("masthead-0000", "Masthead publishing", "How does the masthead get updated?")
	gateway  = session("gateway-00000", "Gateway errors", "/qtx-logs gateway errors")
)

type harness struct {
	t       *testing.T
	m       Model
	blocked []string
	asked   []string
	ranked  []index.Entry
	askErr  error
	planErr error
	planned []string
}

func newHarness(t *testing.T, entries ...index.Entry) *harness {
	h := &harness{t: t}
	h.m = New(Deps{
		Config: config.Defaults(),
		Load:   func() ([]index.Entry, error) { return entries, nil },
		Block: func(id string) error {
			h.blocked = append(h.blocked, id)
			return nil
		},
		Ask: func(_ context.Context, q string, _ []index.Entry) ([]index.Entry, error) {
			h.asked = append(h.asked, q)
			return h.ranked, h.askErr
		},
		Plan: func(action string, e index.Entry) (launch.Plan, error) {
			h.planned = append(h.planned, action+" "+e.ID)
			return launch.Plan{Steps: []launch.Step{{Args: []string{"claude", "--resume", e.ID}}}}, h.planErr
		},
	})
	h.send(tea.WindowSizeMsg{Width: 140, Height: 30})
	h.run(h.m.Init())
	return h
}

func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	next, cmd := h.m.Update(msg)
	h.m = next.(Model)
	h.run(cmd)
}

func (h *harness) run(cmd tea.Cmd) {
	h.t.Helper()
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
	case tea.QuitMsg:
		h.m.quitting = true
	default:
		h.send(msg)
	}
}

func (h *harness) key(k tea.KeyType) { h.send(tea.KeyMsg{Type: k}) }

func (h *harness) typeText(s string) {
	for _, r := range s {
		h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func (h *harness) visibleIDs() []string {
	var ids []string
	for _, r := range h.m.rows {
		if r.entry != nil {
			ids = append(ids, r.entry.ID)
		}
	}
	return ids
}

func TestTypingFiltersTheList(t *testing.T) {
	h := newHarness(t, masthead, gateway)
	if got := len(h.visibleIDs()); got != 2 {
		t.Fatalf("listed %d sessions, want 2", got)
	}
	h.typeText("mast")
	if got := strings.Join(h.visibleIDs(), " "); got != masthead.ID {
		t.Errorf("filtered to %q", got)
	}
}

func TestABareModifierPressLeavesTheListAlone(t *testing.T) {
	h := newHarness(t, masthead, gateway)
	h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0}})
	if got := len(h.visibleIDs()); got != 2 || h.m.input.Value() != "" {
		t.Errorf("listed %d, query %q", got, h.m.input.Value())
	}
}

func TestCtrlSSwitchesBetweenFilteringAndAsking(t *testing.T) {
	h := newHarness(t, masthead, gateway)
	h.key(tea.KeyCtrlS)
	if h.m.mode != modeAsk || h.m.input.Prompt != askPrompt {
		t.Fatalf("ctrl+s did not switch to asking: mode %v prompt %q", h.m.mode, h.m.input.Prompt)
	}
	h.typeText("mast")
	if got := len(h.visibleIDs()); got != 2 {
		t.Errorf("typing a question filtered the list to %d", got)
	}
	h.key(tea.KeyCtrlS)
	if h.m.mode != modeFilter || h.m.input.Prompt != filterPrompt {
		t.Errorf("ctrl+s did not switch back: mode %v prompt %q", h.m.mode, h.m.input.Prompt)
	}
}

func TestALeadingQuestionMarkSwitchesModeAndIsDropped(t *testing.T) {
	h := newHarness(t, masthead, gateway)
	h.typeText("?masthead")
	if h.m.mode != modeAsk || h.m.input.Value() != "masthead" {
		t.Errorf("mode %v, query %q", h.m.mode, h.m.input.Value())
	}
	h.m.input.SetValue("")
	h.typeText("why?")
	if h.m.mode != modeAsk {
		t.Error("a ? later in a question switched mode")
	}
	h.m.input.SetValue("")
	h.typeText("?")
	if h.m.mode != modeFilter {
		t.Error("a leading ? in ask mode did not switch back")
	}
}

func TestEnterPicksTheDefaultActionAndCtrlOTheOther(t *testing.T) {
	h := newHarness(t, masthead, gateway)
	h.key(tea.KeyEnter)
	if plan, ok := h.m.Result(); !ok || !strings.Contains(plan.String(), masthead.ID) {
		t.Errorf("enter did not pick the top session: %v %v", plan, ok)
	}
	if h.planned[0] != "resume "+masthead.ID {
		t.Errorf("planned %v", h.planned)
	}

	h = newHarness(t, masthead, gateway)
	h.key(tea.KeyDown)
	h.key(tea.KeyCtrlO)
	if len(h.planned) != 1 || h.planned[0] != "fork "+gateway.ID {
		t.Errorf("planned %v", h.planned)
	}
}

func TestARefusedLaunchStaysInThePickerAndSaysWhy(t *testing.T) {
	h := newHarness(t, masthead)
	h.planErr = errors.New("session masthead-0000 is already running")
	h.key(tea.KeyEnter)
	if _, ok := h.m.Result(); ok || h.m.quitting {
		t.Error("a refused launch left the picker")
	}
	if !strings.Contains(h.m.View(), "already running") {
		t.Error("the refusal is not shown")
	}
}

func TestAskingListsRankedSessionsThenFiltersThem(t *testing.T) {
	h := newHarness(t, masthead, gateway)
	h.ranked = []index.Entry{gateway, masthead}
	h.key(tea.KeyCtrlS)
	h.typeText("header publishing")
	h.key(tea.KeyEnter)

	if len(h.asked) != 1 || h.asked[0] != "header publishing" {
		t.Fatalf("asked %v", h.asked)
	}
	if got := strings.Join(h.visibleIDs(), " "); got != gateway.ID+" "+masthead.ID {
		t.Errorf("ranked rows = %q", got)
	}
	if h.m.mode != modeFilter || h.m.input.Value() != "" {
		t.Errorf("did not return to filtering: mode %v query %q", h.m.mode, h.m.input.Value())
	}
	if !strings.Contains(h.m.View(), "header publishing") {
		t.Error("the view does not name the question")
	}

	h.key(tea.KeyCtrlS)
	h.key(tea.KeyCtrlS)
	if got := len(h.visibleIDs()); got != 2 {
		t.Errorf("ctrl+s twice did not return to every session: %d", got)
	}
}

func TestAskingWithNoQuestionDoesNothing(t *testing.T) {
	h := newHarness(t, masthead)
	h.key(tea.KeyCtrlS)
	h.key(tea.KeyEnter)
	if len(h.asked) != 0 {
		t.Errorf("asked %v", h.asked)
	}
}

func TestAskingWithNoMatchesOrAFailureShowsARowThatCannotBePicked(t *testing.T) {
	for name, askErr := range map[string]error{"no matches": nil, "failure": errors.New("boom")} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, masthead)
			h.askErr = askErr
			h.key(tea.KeyCtrlS)
			h.typeText("anything")
			h.key(tea.KeyEnter)
			if len(h.m.rows) != 1 || h.m.rows[0].entry != nil {
				t.Fatalf("rows = %+v", h.m.rows)
			}
			want := "no matching sessions"
			if askErr != nil {
				want = "ask failed"
			}
			if !strings.Contains(h.m.rows[0].note, want) {
				t.Errorf("note = %q, want %q", h.m.rows[0].note, want)
			}
			h.key(tea.KeyEnter)
			if _, ok := h.m.Result(); ok {
				t.Error("a note row was picked")
			}
		})
	}
}

func TestCtrlXHidesTheSelectedSession(t *testing.T) {
	h := newHarness(t, masthead, gateway)
	h.key(tea.KeyCtrlX)
	if len(h.blocked) != 1 || h.blocked[0] != masthead.ID {
		t.Errorf("blocked %v", h.blocked)
	}
	if got := strings.Join(h.visibleIDs(), " "); got != gateway.ID {
		t.Errorf("still listed: %q", got)
	}
}

func TestEscapeQuitsWithoutAChoice(t *testing.T) {
	h := newHarness(t, masthead)
	h.key(tea.KeyEsc)
	if _, ok := h.m.Result(); ok || !h.m.quitting {
		t.Errorf("quitting %v", h.m.quitting)
	}
}

func TestPreviewShowsTheSelectedSession(t *testing.T) {
	h := newHarness(t, masthead, gateway)
	view := h.m.View()
	for _, want := range []string{"Opening prompt", "How does the masthead get updated?", "Where it left off"} {
		if !strings.Contains(view, want) {
			t.Errorf("preview lacks %q", want)
		}
	}
	h.key(tea.KeyDown)
	if !strings.Contains(h.m.View(), "/qtx-logs gateway errors") {
		t.Error("moving down did not preview the next session")
	}
}

func TestRunningSessionsAreMarked(t *testing.T) {
	running := session("running-0000", "Live one", "hi")
	running.Live = true
	h := newHarness(t, running)
	if !strings.Contains(h.m.View(), "●") {
		t.Error("a running session is not marked")
	}
}

func TestNarrowTerminalsStillRender(t *testing.T) {
	h := newHarness(t, masthead, gateway)
	h.send(tea.WindowSizeMsg{Width: 60, Height: 20})
	for i, line := range strings.Split(h.m.View(), "\n") {
		if w := visibleWidth(line); w > 60 {
			t.Errorf("line %d is %d wide", i, w)
		}
	}
}
