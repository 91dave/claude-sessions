package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	openingPromptWindow = 50
	searchTextLimit     = 400
	untitled            = "(untitled)"
)

type Session struct {
	HasUser       bool
	Cwd           string
	Branch        string
	Started       time.Time
	OpeningPrompt string
	AITitle       string
	Summaries     []string
	LastPrompt    string
	HasTurns      bool
	Messages      int
}

func (s Session) Title() string {
	if s.AITitle != "" {
		return s.AITitle
	}
	for _, line := range strings.Split(s.OpeningPrompt, "\n") {
		if strings.TrimSpace(line) != "" {
			return line
		}
	}
	return untitled
}

func (s Session) Project() string {
	cwd := strings.TrimRight(s.Cwd, `/\`)
	if i := strings.LastIndexAny(cwd, `/\`); i >= 0 {
		return cwd[i+1:]
	}
	return cwd
}

func (s Session) SearchText() string {
	parts := append([]string{s.OpeningPrompt}, s.Summaries...)
	parts = append(parts, s.LastPrompt)
	text := strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
	if runes := []rune(text); len(runes) > searchTextLimit {
		return string(runes[:searchTextLimit])
	}
	return text
}

func ParseFile(path string) (Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, err
	}
	defer f.Close()
	return Parse(f)
}

type entry struct {
	Type       string `json:"type"`
	IsMeta     bool   `json:"isMeta"`
	Cwd        string `json:"cwd"`
	GitBranch  string `json:"gitBranch"`
	Timestamp  string `json:"timestamp"`
	AITitle    string `json:"aiTitle"`
	Summary    string `json:"summary"`
	LastPrompt string `json:"lastPrompt"`
	Message    struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

var interestingTypes = [][]byte{
	[]byte(`"type":"user"`),
	[]byte(`"type":"assistant"`),
	[]byte(`"type":"ai-title"`),
	[]byte(`"type":"summary"`),
	[]byte(`"type":"last-prompt"`),
}

func Parse(r io.Reader) (Session, error) {
	var s Session
	var users []entry
	reader := bufio.NewReaderSize(r, 64<<10)
	for {
		line, err := reader.ReadBytes('\n')
		if e, ok := decode(line); ok {
			s.absorb(e, &users)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return s, err
		}
	}
	s.OpeningPrompt = openingPrompt(users)
	return s, nil
}

func decode(line []byte) (entry, bool) {
	var e entry
	if !containsAny(line, interestingTypes) {
		return e, false
	}
	if json.Unmarshal(line, &e) != nil {
		return e, false
	}
	return e, true
}

func containsAny(line []byte, needles [][]byte) bool {
	for _, n := range needles {
		if bytes.Contains(line, n) {
			return true
		}
	}
	return false
}

func (s *Session) absorb(e entry, users *[]entry) {
	switch e.Type {
	case "user":
		s.Messages++
		if !s.HasUser {
			s.HasUser = true
			s.Started, _ = time.Parse(time.RFC3339Nano, e.Timestamp)
		}
		if s.Cwd == "" && e.Cwd != "" {
			s.Cwd, s.Branch = e.Cwd, e.GitBranch
		}
		if len(*users) < openingPromptWindow {
			*users = append(*users, e)
		}
		if !e.IsMeta && isOwnPrompt(text(e)) {
			s.HasTurns = true
		}
	case "assistant":
		s.Messages++
		s.HasTurns = true
	case "ai-title":
		if e.AITitle != "" {
			s.AITitle = e.AITitle
		}
	case "summary":
		if e.Summary != "" {
			s.Summaries = append(s.Summaries, e.Summary)
		}
	case "last-prompt":
		if e.LastPrompt != "" {
			s.LastPrompt = e.LastPrompt
		}
	}
}

func isOwnPrompt(t string) bool {
	return t != "" &&
		!strings.HasPrefix(t, "<command-name>") &&
		!strings.HasPrefix(t, "<local-command")
}

func text(e entry) string {
	content := e.Message.Content
	if len(content) == 0 {
		return ""
	}
	var plain string
	if json.Unmarshal(content, &plain) == nil {
		return plain
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &parts) != nil {
		return ""
	}
	for _, p := range parts {
		if p.Type == "text" {
			return p.Text
		}
	}
	return ""
}

var (
	commandName = regexp.MustCompile(`<command-name>([^<]*)</command-name>`)
	commandArgs = regexp.MustCompile(`(?s)<command-args>(.*?)</command-args>`)
	clearInput  = regexp.MustCompile(`^/clear( |$)`)
)

func openingPrompt(users []entry) string {
	for i, e := range users {
		if e.IsMeta {
			continue
		}
		t := text(e)
		if t == "" || strings.HasPrefix(t, "<local-command") {
			continue
		}
		if !strings.Contains(t, "<command-name>") {
			return t
		}
		if i+1 < len(users) && strings.HasPrefix(text(users[i+1]), "<local-command-stdout>") {
			continue
		}
		if typed := asTyped(t); !clearInput.MatchString(typed) {
			return typed
		}
	}
	return ""
}

func asTyped(t string) string {
	var name, args string
	if m := commandName.FindStringSubmatch(t); m != nil {
		name = m[1]
	}
	if m := commandArgs.FindStringSubmatch(t); m != nil {
		args = m[1]
	}
	return strings.TrimSuffix(name+" "+args, " ")
}
