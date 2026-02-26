package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Pane represents a single tmux pane.
type Pane struct {
	Session string
	Window  int
	PaneIdx int
	TTY     string // e.g. "/dev/pts/3"
	Command string // current foreground command
}

// PaneID returns a human-readable identifier like "work:0.1".
func (p Pane) PaneID() string {
	return fmt.Sprintf("%s:%d.%d", p.Session, p.Window, p.PaneIdx)
}

// WindowLabel returns "window_index window_name" if we had name, but we keep it simple.
func (p Pane) WindowLabel() string {
	return strconv.Itoa(p.Window)
}

// ListPanes shells out to tmux and returns all panes across all sessions.
func ListPanes() ([]Pane, error) {
	cmd := exec.Command("tmux", "list-panes", "-a", "-F",
		"#{session_name} #{window_index} #{pane_index} #{pane_tty} #{pane_current_command}")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tmux list-panes: %w (is tmux running?)", err)
	}
	return parsePanes(string(out))
}

func parsePanes(output string) ([]Pane, error) {
	var panes []Pane
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		// Format: "session window_index pane_index /dev/pts/N command"
		// command may contain spaces, so split into at most 5 parts
		parts := strings.SplitN(line, " ", 5)
		if len(parts) < 5 {
			continue
		}
		win, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		paneIdx, err := strconv.Atoi(parts[2])
		if err != nil {
			continue
		}
		panes = append(panes, Pane{
			Session: parts[0],
			Window:  win,
			PaneIdx: paneIdx,
			TTY:     parts[3],
			Command: parts[4],
		})
	}
	return panes, nil
}

// TTYToPaneMap builds a lookup from short TTY name (e.g. "pts/3") to Pane.
func TTYToPaneMap(panes []Pane) map[string]*Pane {
	m := make(map[string]*Pane, len(panes))
	for i := range panes {
		short := strings.TrimPrefix(panes[i].TTY, "/dev/")
		m[short] = &panes[i]
	}
	return m
}
