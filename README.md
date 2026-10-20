# claude-sessions

Find, preview and resume past Claude Code sessions from any project, on Windows, macOS and Linux.

`claude --resume` only lists sessions for the folder you are in. `claude-sessions` lists every
session across every project, newest first, with a preview of what each one was about. Pick one and
it opens in the folder it was started in, resumed or forked.

It is a single binary with no runtime dependencies.

## Install

**Windows** (PowerShell):

```powershell
irm https://raw.githubusercontent.com/91dave/claude-sessions/main/install.ps1 | iex
```

This installs to `%LOCALAPPDATA%\Programs\claude-sessions` and adds it to your user `PATH`.

**macOS and Linux**:

```sh
curl -fsSL https://raw.githubusercontent.com/91dave/claude-sessions/main/install.sh | sh
```

This installs to `~/.local/bin`. Set `CLAUDE_SESSIONS_INSTALL_DIR` to choose another folder, on any
platform.

**With Go** (1.26 or later): `go install github.com/91dave/claude-sessions@latest`

Or download an archive from the [releases page](https://github.com/91dave/claude-sessions/releases).

## Use

Run `claude-sessions`. A short alias helps: `alias cs=claude-sessions`, or in PowerShell
`Set-Alias cs claude-sessions`.

| Key | Action |
|-----|--------|
| type | filter: every word must appear in the title, project, branch, opening prompt, compaction summaries or last prompt |
| `enter` | resume the selected session (or fork it, if that is your configured default) |
| `ctrl+o` or `alt+enter` | the other action: fork instead of resume, or the reverse |
| `ctrl+x` or `alt+d` | hide the session from the list |
| `alt+e` | end a session: remove its running marker, for one whose process is stuck |
| `ctrl+s`, or `?` at the start | switch between filtering and asking Claude |
| `↑` `↓` `pgup` `pgdn` | move through the list |
| `shift+↑` `shift+↓` | scroll the preview |
| `esc` | quit |

A `●` marks a session that is running right now.

### Asking Claude

When you can't remember the words to filter on, press `ctrl+s` (or type `?`), describe the session
in your own words and press `enter`. Your question and a one-line summary of each session are sent to
Claude (Haiku by default) through your installed `claude` CLI, which ranks the sessions that match.
`enter` on a result resumes it as normal, and `ctrl+s` twice returns to the full list.

### Running sessions

A session that is still running is never opened a second time, because two Claude processes writing
to one transcript would split it. If the running session is in a tmux pane, `enter` switches you to
that pane, and the preview shows the pane live. Otherwise it is refused, and you can fork it with
`ctrl+o` instead.

### Other options

```
claude-sessions --list       tab-separated rows: id, last active, ●, project, branch, title, folder, pane, about
claude-sessions --config     where the config, blocklist and cache live, and the settings in force
claude-sessions --dry-run    print the commands instead of running them
claude-sessions --tmux       resume in a new tmux session this time (--jump is the same)
claude-sessions --no-tmux    resume in this terminal this time
```

For scripts and shell wrappers, `claude-sessions --help` lists commands that work without the
picker: `--resume <id>`, `--fork <id>`, `--preview <id>`, `--ask <question>`, and
`--resumes-running <claude args>`, which prints the running session a `claude --resume` would open a
second time.

## Configure

The config file is optional. It lives at:

- Windows: `%AppData%\claude-sessions\config.json`
- macOS: `~/Library/Application Support/claude-sessions/config.json`
- Linux: `~/.config/claude-sessions/config.json`

```json
{
  "defaultAction": "resume",
  "askModel": "haiku",
  "claudeCommand": "claude",
  "tmux": "off",
  "tmuxPrefix": "cc-"
}
```

| Key | Values | Environment override |
|-----|--------|----------------------|
| `defaultAction` | `resume` or `fork`: what `enter` does | `CLAUDE_SESSIONS_DEFAULT` |
| `askModel` | the model used when asking Claude | `CLAUDE_SESSIONS_ASK_MODEL` |
| `claudeCommand` | the Claude Code executable | |
| `tmux` | `off`; `new-session` to resume each session in a tmux session of its own, named after its folder (`cc-myproject`); or `auto` to do that only when you are not already inside tmux | `CLAUDE_SESSIONS_TMUX` |
| `tmuxPrefix` | the prefix for those tmux session names | |
| `commands` | your own commands for tmux integration, below | |

The `tmux` setting is ignored on Windows and wherever tmux is not installed.

`CLAUDE_CONFIG_DIR` is honoured if you keep Claude Code's data somewhere other than `~/.claude`.
`CLAUDE_SESSIONS_CONFIG` points at a different config file. `CLAUDE_SESSIONS_DRYRUN` set to anything
acts like `--dry-run`.

Hidden sessions are listed one id per line in a `blocklist` file: `~/.local/share/claude-sessions/`
on macOS and Linux (or under `$XDG_DATA_HOME`), next to the config on Windows. Edit or delete it to
bring them back.

### Your own commands

Each entry in `commands` replaces one built-in step with a shell command (`sh -c`, or `cmd /C` on
Windows). Leave one out to keep the built-in behaviour.

| Command | Used for | Built-in behaviour |
|---------|----------|--------------------|
| `livePane` | finding the tmux pane a running session is in; the last tab-separated field of its first line of output | the pane in Claude's marker file |
| `attach` | going to a running session's pane | `tmux select-window`, `select-pane`, then `switch-client` or `attach-session` |
| `reviveTmux` | resuming a session in a new tmux session | `tmux new-session -d`, then `switch-client` or `attach-session` |
| `previewRunning` | previewing a running session that has a pane, shown unwrapped | `tmux capture-pane -ep` |
| `previewFinished` | previewing any other session | the built-in summary |

Placeholders are replaced with shell-quoted values: `{id}`, `{cwd}`, `{pid}`, `{pane}`, `{target}`
(`session:@window.%pane`), `{session}`, `{window}`, `{transcript}` (the `.jsonl` path), and, in
`reviveTmux`, `{tmuxName}` and `{claude}` (the whole `claude --resume <id>` command). Preview commands
also get `FZF_PREVIEW_LINES` and `FZF_PREVIEW_COLUMNS` set to the size of the preview.

For example, to hand panes and sessions to your own tmux tooling:

```json
{
  "tmux": "auto",
  "commands": {
    "livePane": "agent-state pane {pid}",
    "attach": "agent-state jump {pane}",
    "reviveTmux": "tmux new-session -d -s {tmuxName} -c {cwd} {claude} && tmux-sessions --attach {tmuxName}",
    "previewRunning": "agent-state preview {pane}"
  }
}
```

## How it works

Claude Code keeps each session as a `.jsonl` transcript under `~/.claude/projects/`. `claude-sessions`
reads the folder, branch, title, opening prompt, compaction summaries and last prompt from each,
caching the result so only new or changed transcripts are read again. Sessions that never got past a
slash command such as `/login` or `/clear` are left out.

A session counts as running when Claude Code's marker file for it in `~/.claude/sessions/` names a
process that is still alive.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for building, testing and releasing.
