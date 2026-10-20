package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	ActionResume = "resume"
	ActionFork   = "fork"

	TmuxOff        = "off"
	TmuxNewSession = "new-session"

	appDir = "claude-sessions"
)

type Config struct {
	DefaultAction string `json:"defaultAction"`
	AskModel      string `json:"askModel"`
	ClaudeCommand string `json:"claudeCommand"`
	Tmux          string `json:"tmux"`
	TmuxPrefix    string `json:"tmuxPrefix"`
}

func Defaults() Config {
	return Config{
		DefaultAction: ActionResume,
		AskModel:      "haiku",
		ClaudeCommand: "claude",
		Tmux:          TmuxOff,
		TmuxPrefix:    "cc-",
	}
}

func Load(path string, getenv func(string) string) (Config, error) {
	c := Defaults()
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return c, fmt.Errorf("reading %s: %w", path, err)
	default:
		if err := json.Unmarshal(data, &c); err != nil {
			return Defaults(), fmt.Errorf("parsing %s: %w", path, err)
		}
	}
	c.applyEnv(getenv)
	c.fallBackFromInvalid()
	return c, nil
}

func (c *Config) applyEnv(getenv func(string) string) {
	overrides := map[string]*string{
		"CLAUDE_SESSIONS_DEFAULT":   &c.DefaultAction,
		"CLAUDE_SESSIONS_ASK_MODEL": &c.AskModel,
		"CLAUDE_SESSIONS_TMUX":      &c.Tmux,
	}
	for name, field := range overrides {
		if v := getenv(name); v != "" {
			*field = v
		}
	}
}

func (c *Config) fallBackFromInvalid() {
	d := Defaults()
	if c.DefaultAction != ActionResume && c.DefaultAction != ActionFork {
		c.DefaultAction = d.DefaultAction
	}
	if c.Tmux != TmuxOff && c.Tmux != TmuxNewSession {
		c.Tmux = d.Tmux
	}
	if c.AskModel == "" {
		c.AskModel = d.AskModel
	}
	if c.ClaudeCommand == "" {
		c.ClaudeCommand = d.ClaudeCommand
	}
	if c.TmuxPrefix == "" {
		c.TmuxPrefix = d.TmuxPrefix
	}
}

func (c Config) OtherAction() string {
	if c.DefaultAction == ActionFork {
		return ActionResume
	}
	return ActionFork
}

type Paths struct {
	ClaudeDir  string
	ConfigFile string
	Blocklist  string
	CacheDir   string
	CacheFile  string
}

func ResolvePaths(getenv func(string) string) (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(home, ".config")
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = filepath.Join(home, ".cache")
	}

	claudeDir := getenv("CLAUDE_CONFIG_DIR")
	if claudeDir == "" {
		claudeDir = filepath.Join(home, ".claude")
	}
	return Paths{
		ClaudeDir:  claudeDir,
		ConfigFile: filepath.Join(configDir, appDir, "config.json"),
		Blocklist:  filepath.Join(configDir, appDir, "blocklist"),
		CacheDir:   filepath.Join(cacheDir, appDir),
		CacheFile:  filepath.Join(cacheDir, appDir, "cache.json"),
	}, nil
}
