package preview

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/91dave/claude-sessions/internal/config"
	"github.com/91dave/claude-sessions/internal/index"
	"github.com/91dave/claude-sessions/internal/launch"
	"github.com/91dave/claude-sessions/internal/shellcmd"
)

const (
	capturePane = "tmux capture-pane -ep -t {pane}"
	timeout     = 5 * time.Second
)

func Command(c config.Config, e index.Entry) (command string, wrap bool, ok bool) {
	vars := launch.Vars(e).Set("pane", e.Pane)
	switch {
	case e.Live && e.Pane != "":
		template := c.Commands.PreviewRunning
		if template == "" {
			template = capturePane
		}
		return shellcmd.Expand(template, vars), false, true
	case c.Commands.PreviewFinished != "":
		return shellcmd.Expand(c.Commands.PreviewFinished, vars), true, true
	}
	return "", false, false
}

func Run(command string, lines, columns int, tail bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := shellcmd.Output(ctx, command, []string{
		"FZF_PREVIEW_LINES=" + strconv.Itoa(lines),
		"FZF_PREVIEW_COLUMNS=" + strconv.Itoa(columns),
	})
	if err != nil {
		return "", err
	}
	out = strings.TrimRight(out, "\n ")
	if !tail {
		return out, nil
	}
	all := strings.Split(out, "\n")
	return strings.Join(all[max(0, len(all)-lines):], "\n"), nil
}
