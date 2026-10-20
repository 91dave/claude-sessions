package ask

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/91dave/claude-sessions/internal/index"
)

const (
	timeout      = 120 * time.Second
	instructions = "You find the user's past Claude Code sessions. You are given a question and a list " +
		"of sessions, one per line: the session id, a tab, then when it was last active, its title, project, " +
		"branch and what it was about. Reply with the ids of the sessions that answer the question, most " +
		"relevant first, one per line, at most 15. Reply with ids only. If none fit, reply with nothing."
)

type Runner func(ctx context.Context, name string, args []string, dir, stdin string) (string, error)

type Asker struct {
	Command string
	Model   string
	Dir     string
	Run     Runner
}

var sessionID = regexp.MustCompile(`[0-9A-Za-z-]{8,}`)

func (a Asker) Rank(ctx context.Context, question string, entries []index.Entry) ([]index.Entry, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	run := a.Run
	if run == nil {
		run = ExecRunner
	}
	reply, err := run(ctx, a.Command, a.args(), a.Dir, prompt(question, entries))
	if err != nil {
		return nil, fmt.Errorf("claude did not answer: %w", err)
	}
	return pickRanked(reply, entries), nil
}

func (a Asker) args() []string {
	return []string{
		"-p",
		"--model", a.Model,
		"--no-session-persistence",
		"--tools", "",
		"--strict-mcp-config",
		"--setting-sources", "project",
		"--system-prompt", instructions,
	}
}

func prompt(question string, entries []index.Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Today is %s.\n\nQuestion: %s\n\nSessions:\n", time.Now().Format("2006-01-02"), question)
	for _, e := range entries {
		fmt.Fprintf(&b, "%s\t%s %s %s %s %s\n",
			e.ID, e.ModTime.Format("2006-01-02 15:04"), e.Title(), e.Project(), e.Branch, e.SearchText())
	}
	return b.String()
}

func pickRanked(reply string, entries []index.Entry) []index.Entry {
	byID := make(map[string]index.Entry, len(entries))
	for _, e := range entries {
		byID[e.ID] = e
	}
	var ranked []index.Entry
	seen := map[string]bool{}
	for _, id := range sessionID.FindAllString(reply, -1) {
		if e, ok := byID[id]; ok && !seen[id] {
			seen[id] = true
			ranked = append(ranked, e)
		}
	}
	return ranked
}

func ExecRunner(ctx context.Context, name string, args []string, dir, stdin string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return stdout.String(), nil
}
