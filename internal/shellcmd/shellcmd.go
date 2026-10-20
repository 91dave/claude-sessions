package shellcmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

type Vars map[string]string

func (v Vars) Set(name, value string) Vars {
	return v.with(name, Quote(value))
}

func (v Vars) SetArgs(name string, args []string) Vars {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = Quote(a)
	}
	return v.with(name, strings.Join(quoted, " "))
}

func (v Vars) with(name, text string) Vars {
	out := make(Vars, len(v)+1)
	for k, val := range v {
		out[k] = val
	}
	out[name] = text
	return out
}

var placeholder = regexp.MustCompile(`\{([A-Za-z]+)\}`)

func Expand(template string, vars Vars) string {
	return placeholder.ReplaceAllStringFunc(template, func(match string) string {
		if text, ok := vars[match[1:len(match)-1]]; ok {
			return text
		}
		return match
	})
}

var posixSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

func Quote(s string) string {
	if runtime.GOOS == "windows" {
		if s != "" && !strings.ContainsAny(s, " \t\"&|<>^") {
			return s
		}
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	if posixSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func Argv(command string) []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/C", command}
	}
	return []string{"sh", "-c", command}
}

func Output(ctx context.Context, command string, env []string) (string, error) {
	argv := Argv(command)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("%s: %w: %s", command, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
