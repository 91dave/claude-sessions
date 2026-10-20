package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/91dave/claude-sessions/internal/config"
	"github.com/91dave/claude-sessions/internal/index"
	"github.com/91dave/claude-sessions/internal/launch"
)

type mode int

const (
	modeFilter mode = iota
	modeAsk
)

const (
	filterPrompt = "session> "
	askPrompt    = "ask> "
	askGhost     = "describe the session, then enter"
)

type Deps struct {
	Config config.Config
	Load   func() ([]index.Entry, error)
	Block  func(id string) error
	Ask    func(ctx context.Context, question string, entries []index.Entry) ([]index.Entry, error)
	Plan   func(action string, e index.Entry) (launch.Plan, error)
}

type row struct {
	entry *index.Entry
	note  string
}

type Model struct {
	deps  Deps
	input lineInput
	mode  mode

	all       []index.Entry
	base      []index.Entry
	rankedFor string
	emptyNote string
	rows      []row

	cursor        int
	offset        int
	previewOffset int
	width         int
	height        int

	loading  bool
	asking   bool
	status   string
	plan     *launch.Plan
	quitting bool
}

type loadedMsg struct {
	entries []index.Entry
	err     error
}

type askedMsg struct {
	question string
	ranked   []index.Entry
	err      error
}

func New(deps Deps) Model {
	return Model{deps: deps, input: lineInput{Prompt: filterPrompt}, loading: true}
}

func (m Model) Init() tea.Cmd {
	load := m.deps.Load
	return func() tea.Msg {
		entries, err := load()
		return loadedMsg{entries: entries, err: err}
	}
}

func (m Model) Result() (launch.Plan, bool) {
	if m.plan == nil {
		return launch.Plan{}, false
	}
	return *m.plan, true
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scrollToCursor()
	case loadedMsg:
		m.loading = false
		if msg.err != nil {
			m.status = "could not read sessions: " + msg.err.Error()
		}
		m.all, m.base = msg.entries, msg.entries
		m.refilter()
	case askedMsg:
		m.showRanking(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		m.quitting = true
		return m, tea.Quit
	case tea.KeyCtrlS:
		m.toggleMode()
		return m, nil
	case tea.KeyEnter:
		if m.mode == modeAsk {
			return m.startAsking()
		}
		return m.pick(m.deps.Config.DefaultAction)
	case tea.KeyCtrlO:
		return m.pick(m.deps.Config.OtherAction())
	case tea.KeyCtrlX:
		m.hideSelected()
		return m, nil
	case tea.KeyUp, tea.KeyCtrlP:
		m.moveCursor(-1)
		return m, nil
	case tea.KeyDown, tea.KeyCtrlN:
		m.moveCursor(1)
		return m, nil
	case tea.KeyPgUp:
		m.moveCursor(-m.listHeight())
		return m, nil
	case tea.KeyPgDown:
		m.moveCursor(m.listHeight())
		return m, nil
	case tea.KeyShiftUp:
		m.previewOffset = max(0, m.previewOffset-1)
		return m, nil
	case tea.KeyShiftDown:
		m.previewOffset++
		return m, nil
	}

	before := m.input.Value()
	m.input.Update(msg)
	if after := m.input.Value(); strings.HasPrefix(after, "?") {
		m.input.SetValue(strings.TrimPrefix(after, "?"))
		m.toggleMode()
	} else if m.mode == modeFilter && after != before {
		m.status = ""
		m.refilter()
	}
	return m, nil
}

func (m *Model) toggleMode() {
	m.status = ""
	if m.mode == modeFilter {
		m.mode = modeAsk
		m.input.Prompt = askPrompt
		m.input.Placeholder = askGhost
		return
	}
	m.mode = modeFilter
	m.input.Prompt = filterPrompt
	m.input.Placeholder = ""
	m.base, m.rankedFor, m.emptyNote = m.all, "", ""
	m.refilter()
}

func (m Model) startAsking() (tea.Model, tea.Cmd) {
	question := strings.TrimSpace(m.input.Value())
	if question == "" || m.asking {
		return m, nil
	}
	m.asking = true
	m.status = ""
	ask, all := m.deps.Ask, m.all
	return m, func() tea.Msg {
		ranked, err := ask(context.Background(), question, all)
		return askedMsg{question: question, ranked: ranked, err: err}
	}
}

func (m *Model) showRanking(msg askedMsg) {
	m.asking = false
	m.mode = modeFilter
	m.input.Prompt = filterPrompt
	m.input.Placeholder = ""
	m.input.SetValue("")
	m.base, m.rankedFor = msg.ranked, msg.question
	switch {
	case msg.err != nil:
		m.base, m.emptyNote = nil, "ask failed: "+msg.err.Error()
	case len(msg.ranked) == 0:
		m.emptyNote = "no matching sessions for: " + msg.question
	default:
		m.emptyNote = ""
	}
	m.refilter()
}

func (m Model) pick(action string) (tea.Model, tea.Cmd) {
	e := m.selected()
	if e == nil {
		return m, nil
	}
	plan, err := m.deps.Plan(action, *e)
	if err != nil {
		m.status = err.Error()
		if errors.Is(err, launch.ErrAlreadyRunning) {
			m.status += " (ctrl+o forks)"
		}
		return m, nil
	}
	m.plan = &plan
	m.quitting = true
	return m, tea.Quit
}

func (m *Model) hideSelected() {
	e := m.selected()
	if e == nil {
		return
	}
	id, title := e.ID, e.Title()
	if err := m.deps.Block(id); err != nil {
		m.status = "could not hide session: " + err.Error()
		return
	}
	notID := func(x index.Entry) bool { return x.ID == id }
	m.all = slices.DeleteFunc(slices.Clone(m.all), notID)
	m.base = slices.DeleteFunc(slices.Clone(m.base), notID)
	m.status = fmt.Sprintf("hidden: %s", title)
	cursor := m.cursor
	m.refilter()
	m.cursor = min(cursor, max(0, len(m.rows)-1))
	m.scrollToCursor()
}

func (m *Model) refilter() {
	query := ""
	if m.mode == modeFilter {
		query = m.input.Value()
	}
	m.rows = nil
	for i := range m.base {
		if index.Match(m.base[i], query) {
			m.rows = append(m.rows, row{entry: &m.base[i]})
		}
	}
	if len(m.rows) == 0 && m.emptyNote != "" {
		m.rows = append(m.rows, row{note: m.emptyNote})
	}
	m.cursor, m.offset, m.previewOffset = 0, 0, 0
}

func (m *Model) moveCursor(delta int) {
	if len(m.rows) == 0 {
		return
	}
	m.cursor = min(max(0, m.cursor+delta), len(m.rows)-1)
	m.previewOffset = 0
	m.scrollToCursor()
}

func (m *Model) scrollToCursor() {
	h := m.listHeight()
	if h <= 0 {
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
}

func (m Model) selected() *index.Entry {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return m.rows[m.cursor].entry
}

func (m Model) query() string {
	if m.mode == modeFilter {
		return m.input.Value()
	}
	return ""
}
