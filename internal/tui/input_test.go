package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func typed(in *lineInput, keys ...tea.KeyMsg) {
	for _, k := range keys {
		in.Update(k)
	}
}

func runes(s string) tea.KeyMsg    { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func key(k tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: k} }

func TestLineInputEditing(t *testing.T) {
	cases := []struct {
		name string
		keys []tea.KeyMsg
		want string
	}{
		{"typing appends", []tea.KeyMsg{runes("mast"), key(tea.KeySpace), runes("head")}, "mast head"},
		{"a paste arrives whole", []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("pasted text"), Paste: true}}, "pasted text"},
		{"backspace deletes before the cursor", []tea.KeyMsg{runes("abc"), key(tea.KeyBackspace)}, "ab"},
		{"backspace at the start does nothing", []tea.KeyMsg{key(tea.KeyBackspace)}, ""},
		{"left then typing inserts", []tea.KeyMsg{runes("ac"), key(tea.KeyLeft), runes("b")}, "abc"},
		{"home then delete removes the first rune", []tea.KeyMsg{runes("éab"), key(tea.KeyHome), key(tea.KeyDelete)}, "ab"},
		{"ctrl+w deletes the word before the cursor", []tea.KeyMsg{runes("masthead header  "), key(tea.KeyCtrlW)}, "masthead "},
		{"ctrl+u clears to the start", []tea.KeyMsg{runes("abc def"), key(tea.KeyLeft), key(tea.KeyCtrlU)}, "f"},
		{"ctrl+k clears to the end", []tea.KeyMsg{runes("abc def"), key(tea.KeyCtrlA), key(tea.KeyRight), key(tea.KeyCtrlK)}, "a"},
		{"ctrl+e moves to the end", []tea.KeyMsg{runes("ab"), key(tea.KeyHome), key(tea.KeyCtrlE), runes("c")}, "abc"},
		{"a bare modifier press on Windows types nothing", []tea.KeyMsg{runes("ab"), runes("\x00"), runes("c")}, "abc"},
		{"control characters in a paste are dropped", []tea.KeyMsg{runes("a\x1b\x07b")}, "ab"},
		{"line breaks in a paste become spaces", []tea.KeyMsg{runes("one\r\ntwo\tthree")}, "one  two three"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var in lineInput
			typed(&in, c.keys...)
			if got := in.Value(); got != c.want {
				t.Errorf("value = %q, want %q", got, c.want)
			}
		})
	}
}

func TestLineInputSetValueMovesTheCursorToTheEnd(t *testing.T) {
	var in lineInput
	in.SetValue("question")
	typed(&in, runes("?"))
	if in.Value() != "question?" {
		t.Errorf("value = %q", in.Value())
	}
}

func TestLineInputView(t *testing.T) {
	in := lineInput{Prompt: "ask> ", Placeholder: "describe it"}
	if v := in.View(); !strings.Contains(v, "ask> ") || !strings.Contains(v, "describe it") {
		t.Errorf("empty view = %q", v)
	}
	in.SetValue("masthead")
	if v := in.View(); !strings.Contains(v, "masthead") || strings.Contains(v, "describe it") {
		t.Errorf("view = %q", v)
	}
}
