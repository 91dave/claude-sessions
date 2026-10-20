# Contributing

## Build and run

You need Go (the version in `go.mod`; with `GOTOOLCHAIN=auto`, the default, an older Go downloads it).

```sh
go run . --dry-run          # the picker, printing commands instead of running them
go build -o claude-sessions .
GOOS=windows go build -o claude-sessions.exe .
```

`--dry-run` (or `CLAUDE_SESSIONS_DRYRUN=1`) is the safe way to try changes against your real
sessions. `CLAUDE_CONFIG_DIR` and `CLAUDE_SESSIONS_CONFIG` point it at other data or config.

## Layout

| Path | Responsibility |
|------|----------------|
| `main.go` | flags, the scripting commands, wiring |
| `internal/transcript` | reading one `.jsonl` transcript into the fields the picker uses |
| `internal/index` | listing every session: cache, blocklist, filtering |
| `internal/live` | which sessions are running, from Claude Code's marker files |
| `internal/config` | `config.json`, environment overrides, file locations |
| `internal/launch` | planning and running a resume, fork or jump to a running pane |
| `internal/shellcmd` | expanding and running the user's `commands` templates |
| `internal/preview` | choosing and running a preview command |
| `internal/ask` | ranking sessions by asking Claude |
| `internal/tui` | the Bubble Tea picker |

## Test

```sh
go test ./...
go vet ./...
gofmt -l .                  # prints nothing when formatting is clean
```

Most tests are plain unit tests. The picker is tested by driving its Bubble Tea model through
`Update` with key messages, so no terminal is needed; see `internal/tui/tui_test.go`.

`go test` also runs `test/compat/claude-sessions.test.sh`: the test suite of the original bash
`claude-sessions`, run against this binary to keep the two compatible. Only its helpers have been
adapted, not its assertions. It needs bash, tmux and ripgrep, and is skipped when any is missing or on
Windows. `test/compat/bin/agent-state` stands in for the external `agent-state` tool it integrates with.
To run it on its own:

```sh
go build -o /tmp/claude-sessions . && CLAUDE_SESSIONS_BIN=/tmp/claude-sessions bash test/compat/claude-sessions.test.sh
```

CI runs vet and the tests on Linux, macOS and Windows for every pull request.

## Changes

- Work on a branch and open a pull request; `main` only changes through merged PRs.
- Write the test first, and keep commits small, using
  [Conventional Commits](https://www.conventionalcommits.org) (`feat(tui): …`, `fix(launch): …`).
- New dependencies need a good reason: the tool's appeal is a small, fast, single binary.

## Release

Push a `v*` tag from `main`:

```sh
git tag -a v0.2.0 -m v0.2.0
git push origin v0.2.0
```

The release workflow runs the tests, then GoReleaser builds archives for Windows, macOS and Linux
(amd64 and arm64) and publishes them to GitHub Releases. The install scripts always fetch the latest
release, so users update by re-running them.

To check the GoReleaser config without publishing:

```sh
podman run --rm -v "$PWD":/src -w /src docker.io/goreleaser/goreleaser:v2.12.0 release --snapshot --clean --skip=publish
```
