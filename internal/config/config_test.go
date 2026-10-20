package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMissingFileGivesDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "absent.json"), env(nil))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{DefaultAction: "resume", AskModel: "haiku", ClaudeCommand: "claude", Tmux: "off", TmuxPrefix: "cc-"}
	if c != want {
		t.Errorf("config = %+v, want %+v", c, want)
	}
}

func TestFileOverridesDefaultsKeyByKey(t *testing.T) {
	path := writeConfig(t, `{"defaultAction":"fork","tmux":"new-session"}`)
	c, err := Load(path, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.DefaultAction != "fork" || c.Tmux != "new-session" || c.AskModel != "haiku" || c.TmuxPrefix != "cc-" {
		t.Errorf("config = %+v", c)
	}
}

func TestEnvironmentOverridesFile(t *testing.T) {
	path := writeConfig(t, `{"defaultAction":"fork","askModel":"sonnet","tmux":"new-session"}`)
	c, err := Load(path, env(map[string]string{
		"CLAUDE_SESSIONS_DEFAULT":   "resume",
		"CLAUDE_SESSIONS_ASK_MODEL": "opus",
		"CLAUDE_SESSIONS_TMUX":      "off",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.DefaultAction != "resume" || c.AskModel != "opus" || c.Tmux != "off" {
		t.Errorf("config = %+v", c)
	}
}

func TestInvalidValuesFallBack(t *testing.T) {
	path := writeConfig(t, `{"defaultAction":"explode","tmux":"sometimes","tmuxPrefix":"","claudeCommand":""}`)
	c, err := Load(path, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.DefaultAction != "resume" || c.Tmux != "off" || c.TmuxPrefix != "cc-" || c.ClaudeCommand != "claude" {
		t.Errorf("config = %+v", c)
	}
}

func TestMalformedFileNamesItself(t *testing.T) {
	path := writeConfig(t, `{"tmux": `)
	_, err := Load(path, env(nil))
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("err = %v, want one naming %s", err, path)
	}
}

func TestOtherAction(t *testing.T) {
	if (Config{DefaultAction: "resume"}).OtherAction() != "fork" {
		t.Error("the other action of resume is not fork")
	}
	if (Config{DefaultAction: "fork"}).OtherAction() != "resume" {
		t.Error("the other action of fork is not resume")
	}
}

func TestCommandsAreReadFromTheFile(t *testing.T) {
	path := writeConfig(t, `{"tmux":"auto","commands":{"attach":"agent-state jump {pane}","livePane":"agent-state pane {pid}"}}`)
	c, err := Load(path, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Tmux != TmuxAuto {
		t.Errorf("tmux = %q, want auto", c.Tmux)
	}
	if c.Commands.Attach != "agent-state jump {pane}" || c.Commands.LivePane != "agent-state pane {pid}" {
		t.Errorf("commands = %+v", c.Commands)
	}
	if c.Commands.ReviveTmux != "" || c.Commands.PreviewRunning != "" || c.Commands.PreviewFinished != "" {
		t.Errorf("unset commands are not empty: %+v", c.Commands)
	}
}

func TestConfigPathCanBeOverridden(t *testing.T) {
	p, err := ResolvePaths(env(map[string]string{"CLAUDE_SESSIONS_CONFIG": "/elsewhere/cs.json"}))
	if err != nil {
		t.Fatal(err)
	}
	if p.ConfigFile != "/elsewhere/cs.json" {
		t.Errorf("ConfigFile = %q", p.ConfigFile)
	}
}

func TestBlocklistLivesWithUserDataOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows keeps the blocklist beside the config")
	}
	p, err := ResolvePaths(env(map[string]string{"XDG_DATA_HOME": "/data"}))
	if err != nil {
		t.Fatal(err)
	}
	if p.Blocklist != "/data/claude-sessions/blocklist" {
		t.Errorf("Blocklist = %q", p.Blocklist)
	}
	p, err = ResolvePaths(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if p.Blocklist != filepath.Join(home, ".local", "share", "claude-sessions", "blocklist") {
		t.Errorf("Blocklist = %q", p.Blocklist)
	}
}

func TestClaudeDirHonoursOverride(t *testing.T) {
	p, err := ResolvePaths(env(map[string]string{"CLAUDE_CONFIG_DIR": "/custom/claude"}))
	if err != nil {
		t.Fatal(err)
	}
	if p.ClaudeDir != "/custom/claude" {
		t.Errorf("ClaudeDir = %q", p.ClaudeDir)
	}
	for name, path := range map[string]string{"ConfigFile": p.ConfigFile, "CacheFile": p.CacheFile, "Blocklist": p.Blocklist} {
		if !strings.Contains(path, "claude-sessions") {
			t.Errorf("%s = %q is not under a claude-sessions directory", name, path)
		}
	}
}
