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
| `ctrl+o` | the other action: fork instead of resume, or the reverse |
| `ctrl+x` | hide the session from the list |
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
that pane. Otherwise it is refused, and you can fork it with `ctrl+o` instead.

### Other options

```
claude-sessions --list       tab-separated rows: id, last active, ●, project, branch, title, folder
claude-sessions --config     where the config, blocklist and cache live, and the settings in force
claude-sessions --dry-run    print the commands instead of running them
claude-sessions --tmux       resume in a new tmux session this time
claude-sessions --no-tmux    resume in this terminal this time
```

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
| `tmux` | `off`, or `new-session` to resume each session in a tmux session of its own, named after its folder (`cc-myproject`) | `CLAUDE_SESSIONS_TMUX` |
| `tmuxPrefix` | the prefix for those tmux session names | |

The `tmux` setting is ignored on Windows and wherever tmux is not installed.

`CLAUDE_CONFIG_DIR` is honoured if you keep Claude Code's data somewhere other than `~/.claude`.
`CLAUDE_SESSIONS_DRYRUN` set to anything acts like `--dry-run`.

Hidden sessions are listed one id per line in a `blocklist` file next to the config. Edit or delete
it to bring them back.

## How it works

Claude Code keeps each session as a `.jsonl` transcript under `~/.claude/projects/`. `claude-sessions`
reads the folder, branch, title, opening prompt, compaction summaries and last prompt from each,
caching the result so only new or changed transcripts are read again. Sessions that never got past a
slash command such as `/login` or `/clear` are left out.

A session counts as running when Claude Code's marker file for it in `~/.claude/sessions/` names a
process that is still alive.

## Develop

```sh
go test ./...
go run . --dry-run
```

Releases are built by GoReleaser when a `v*` tag is pushed.
