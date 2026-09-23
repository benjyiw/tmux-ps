package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// herdr has no numeric window/pane index like tmux — panes carry opaque
// pane_id/tab_id/workspace_id strings instead. We assign sequential indices
// in list order so the rest of the program (Pane.PaneID, rendering, tree
// walking) can stay exactly the same as the tmux backend.

type herdrPaneInfo struct {
	PaneID      string  `json:"pane_id"`
	WorkspaceID string  `json:"workspace_id"`
	TabID       string  `json:"tab_id"`
	Title       *string `json:"title"`
	Label       *string `json:"label"`
}

type herdrPaneListResponse struct {
	Result struct {
		Panes []herdrPaneInfo `json:"panes"`
	} `json:"result"`
}

type herdrProcessInfoResponse struct {
	Result struct {
		ProcessInfo struct {
			ShellPID *int    `json:"shell_pid"`
			TTY      *string `json:"tty"`
		} `json:"process_info"`
	} `json:"result"`
}

type herdrWorkspaceListResponse struct {
	Result struct {
		Workspaces []struct {
			WorkspaceID string `json:"workspace_id"`
			Label       string `json:"label"`
		} `json:"workspaces"`
	} `json:"result"`
}

// herdrWorkspaceLabels maps workspace_id -> its human-readable label, e.g.
// "w0" -> "tmux-ps". Best-effort: an empty map just falls back to raw IDs.
func herdrWorkspaceLabels() map[string]string {
	labels := make(map[string]string)
	out, err := exec.Command("herdr", "workspace", "list").Output()
	if err != nil {
		return labels
	}
	var resp herdrWorkspaceListResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return labels
	}
	for _, ws := range resp.Result.Workspaces {
		labels[ws.WorkspaceID] = ws.Label
	}
	return labels
}

// ListPanesHerdr shells out to the herdr CLI and returns all panes across all
// workspaces, in the same shape ListPanes() returns for tmux.
func ListPanesHerdr() ([]Pane, error) {
	out, err := exec.Command("herdr", "pane", "list").Output()
	if err != nil {
		return nil, fmt.Errorf("herdr pane list: %w (is herdr running?)", err)
	}

	var resp herdrPaneListResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parsing herdr pane list output: %w", err)
	}

	workspaceLabels := herdrWorkspaceLabels()

	type tabKey struct{ workspace, tab string }
	nextWindow := make(map[string]int) // workspace -> next window index
	windowOf := make(map[tabKey]int)   // (workspace, tab) -> assigned window index
	nextPaneIdx := make(map[tabKey]int)

	var panes []Pane
	for _, p := range resp.Result.Panes {
		k := tabKey{p.WorkspaceID, p.TabID}
		window, ok := windowOf[k]
		if !ok {
			window = nextWindow[p.WorkspaceID]
			windowOf[k] = window
			nextWindow[p.WorkspaceID]++
		}
		paneIdx := nextPaneIdx[k]
		nextPaneIdx[k]++

		command := ""
		switch {
		case p.Title != nil:
			command = *p.Title
		case p.Label != nil:
			command = *p.Label
		}

		pid, tty := herdrProcessInfo(p.PaneID)

		session := p.WorkspaceID
		if label, ok := workspaceLabels[p.WorkspaceID]; ok && label != "" {
			session = label
		}

		panes = append(panes, Pane{
			Session: session,
			Window:  window,
			PaneIdx: paneIdx,
			TTY:     tty,
			PID:     pid,
			Command: command,
		})
	}

	return panes, nil
}

// herdrProcessInfo looks up a pane's shell PID and TTY. Best-effort: a pane
// that herdr can't report on (e.g. it just closed) is returned with zero
// values rather than failing the whole listing.
func herdrProcessInfo(paneID string) (pid int, tty string) {
	out, err := exec.Command("herdr", "pane", "process-info", "--pane", paneID).Output()
	if err != nil {
		return 0, ""
	}

	var resp herdrProcessInfoResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return 0, ""
	}

	if resp.Result.ProcessInfo.ShellPID != nil {
		pid = *resp.Result.ProcessInfo.ShellPID
	}
	if resp.Result.ProcessInfo.TTY != nil {
		tty = *resp.Result.ProcessInfo.TTY
	}
	return pid, tty
}
