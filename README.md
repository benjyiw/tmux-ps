# tmux-ps

Point-in-time resource usage per tmux pane. Like `ps`, but with tmux information.

Reads `/proc` directly — no dependency on `ps` or external tools beyond `tmux` itself.

## Install

```
go install github.com/benjyiw/tmux-ps@latest
```

Or build from source:

```
go build
```

## Usage

```
tmux-ps [flags]
```

| Flag | Description |
|------|-------------|
| `-n NUM` | Show top N panes (default: all) |
| `-s FIELD` | Sort by: `cpu` (default), `mem`, `rss`, `procs` |
| `-g` | Group by session, sorted by cumulative memory |
| `-p PANE` | Show all processes for a specific pane (e.g. `work:0.1`) |
| `-t` | Show process tree for the top pane |

