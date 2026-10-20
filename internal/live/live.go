package live

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Marker struct {
	SessionID string `json:"sessionId"`
	PID       int    `json:"pid"`
	Cwd       string `json:"cwd"`
	Tmux      string `json:"tmux"`
}

func Read(dir string, alive func(int) bool) map[string]Marker {
	markers := map[string]Marker{}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	for _, path := range paths {
		m, ok := readMarker(path)
		if ok && alive(m.PID) {
			markers[m.SessionID] = m
		}
	}
	return markers
}

func End(dir, sessionID string) error {
	paths, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	for _, path := range paths {
		if m, ok := readMarker(path); ok && m.SessionID == sessionID {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func readMarker(path string) (Marker, bool) {
	var m Marker
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &m) != nil {
		return m, false
	}
	return m, m.SessionID != "" && m.PID > 0
}

func (m Marker) TmuxSession() string {
	session, _, _ := strings.Cut(m.Tmux, ":")
	return session
}

func (m Marker) TmuxWindow() string {
	window, _, _ := strings.Cut(m.tmuxWindowAndPane(), ".")
	return window
}

func (m Marker) TmuxPane() string {
	_, pane, _ := strings.Cut(m.tmuxWindowAndPane(), ".")
	return pane
}

func (m Marker) tmuxWindowAndPane() string {
	_, rest, _ := strings.Cut(m.Tmux, ":")
	return rest
}
