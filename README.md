# tmux-ps

Point-in-time resource usage per tmux (or [herdr](https://github.com/herdrdev/herdr)) pane. Like `ps`, but with multiplexer information.

On Linux, reads `/proc` directly. On macOS, shells out to `ps` (no `/proc` there).

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
| `-w` | Watch mode: periodically refresh like `top` |
| `-i SECONDS` | Refresh interval in seconds (default: 2, used with `-w`) |
| `-m BACKEND` | Multiplexer backend: `tmux` (default) or `herdr` |

### Watch mode controls

| Key | Action |
|-----|--------|
| `q` / `Ctrl-C` | Quit |
| `s` | Cycle sort: cpu → mem → rss → procs |
| `t` | Toggle tree view |
| `g` | Toggle group-by-session |

