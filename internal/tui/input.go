package tui

import (
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var cursorCell = lipgloss.NewStyle().Reverse(true)

type lineInput struct {
	Prompt      string
	Placeholder string
	value       []rune
	pos         int
}

func (in *lineInput) Value() string { return string(in.value) }

func (in *lineInput) SetValue(s string) {
	in.value = []rune(s)
	in.pos = len(in.value)
}

func (in *lineInput) Update(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyRunes:
		in.insert(msg.Runes)
	case tea.KeySpace:
		in.insert([]rune{' '})
	case tea.KeyBackspace:
		if in.pos > 0 {
			in.value = append(in.value[:in.pos-1], in.value[in.pos:]...)
			in.pos--
		}
	case tea.KeyDelete:
		if in.pos < len(in.value) {
			in.value = append(in.value[:in.pos], in.value[in.pos+1:]...)
		}
	case tea.KeyLeft, tea.KeyCtrlB:
		in.pos = max(0, in.pos-1)
	case tea.KeyRight, tea.KeyCtrlF:
		in.pos = min(len(in.value), in.pos+1)
	case tea.KeyHome, tea.KeyCtrlA:
		in.pos = 0
	case tea.KeyEnd, tea.KeyCtrlE:
		in.pos = len(in.value)
	case tea.KeyCtrlU:
		in.value = in.value[in.pos:]
		in.pos = 0
	case tea.KeyCtrlK:
		in.value = in.value[:in.pos]
	case tea.KeyCtrlW:
		in.deleteWordBack()
	}
}

func (in *lineInput) insert(r []rune) {
	tail := append([]rune{}, in.value[in.pos:]...)
	in.value = append(append(in.value[:in.pos], r...), tail...)
	in.pos += len(r)
}

func (in *lineInput) deleteWordBack() {
	start := in.pos
	for start > 0 && unicode.IsSpace(in.value[start-1]) {
		start--
	}
	for start > 0 && !unicode.IsSpace(in.value[start-1]) {
		start--
	}
	in.value = append(in.value[:start], in.value[in.pos:]...)
	in.pos = start
}

func (in lineInput) View() string {
	if len(in.value) == 0 {
		return in.Prompt + cursorCell.Render(" ") + dimStyle.Render(in.Placeholder)
	}
	before := string(in.value[:in.pos])
	if in.pos == len(in.value) {
		return in.Prompt + before + cursorCell.Render(" ")
	}
	return in.Prompt + before + cursorCell.Render(string(in.value[in.pos])) + string(in.value[in.pos+1:])
}
