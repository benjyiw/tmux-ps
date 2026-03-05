package main

import (
	"strings"
	"testing"
)

// helper to build test summaries
func testSummaries() []PaneSummary {
	pane1 := Pane{Session: "work", Window: 0, PaneIdx: 0, TTY: "/dev/pts/1", PID: 100, Command: "zsh"}
	pane2 := Pane{Session: "work", Window: 0, PaneIdx: 1, TTY: "/dev/pts/2", PID: 200, Command: "vim"}
	pane3 := Pane{Session: "dev", Window: 1, PaneIdx: 0, TTY: "/dev/pts/3", PID: 300, Command: "bash"}

	return []PaneSummary{
		{
			Pane: &pane1, CPUPct: 12.5, MemPct: 3.2, RSS: 51200,
			NumProcs: 2, TopProcess: "node", TopCPU: 10.0,
			Procs: []ProcStat{
				{PID: 101, PPID: 100, Comm: "node", CPUPct: 10.0, MemPct: 2.5, RSS: 40960},
				{PID: 100, PPID: 1, Comm: "zsh", CPUPct: 2.5, MemPct: 0.7, RSS: 10240},
			},
		},
		{
			Pane: &pane2, CPUPct: 0.5, MemPct: 1.1, RSS: 8192,
			NumProcs: 1, TopProcess: "vim", TopCPU: 0.5,
			Procs: []ProcStat{
				{PID: 200, PPID: 1, Comm: "vim", CPUPct: 0.5, MemPct: 1.1, RSS: 8192},
			},
		},
		{
			Pane: &pane3, CPUPct: 55.0, MemPct: 8.0, RSS: 204800,
			NumProcs: 3, TopProcess: "cargo", TopCPU: 50.0,
			Procs: []ProcStat{
				{PID: 301, PPID: 300, Comm: "cargo", CPUPct: 50.0, MemPct: 6.0, RSS: 153600},
				{PID: 302, PPID: 301, Comm: "rustc", CPUPct: 4.0, MemPct: 1.5, RSS: 40960},
				{PID: 300, PPID: 1, Comm: "bash", CPUPct: 1.0, MemPct: 0.5, RSS: 10240},
			},
		},
	}
}

func cpuSort(a, b PaneSummary) bool {
	if a.CPUPct != b.CPUPct {
		return a.CPUPct > b.CPUPct
	}
	return a.MemPct > b.MemPct
}

func TestRenderFlat(t *testing.T) {
	out := renderFlat(testSummaries(), cpuSort, 0)
	if out == "" {
		t.Fatal("renderFlat returned empty string")
	}
	// Header should be present
	if !strings.Contains(out, "SESSION") {
		t.Error("missing header column SESSION")
	}
	if !strings.Contains(out, "CPU%") {
		t.Error("missing header column CPU%")
	}
	// Data should be present
	if !strings.Contains(out, "work") {
		t.Error("missing session name 'work'")
	}
	if !strings.Contains(out, "dev") {
		t.Error("missing session name 'dev'")
	}
}

func TestRenderFlatTopN(t *testing.T) {
	out := renderFlat(testSummaries(), cpuSort, 1)
	// Only top 1 pane (dev session, highest CPU)
	if !strings.Contains(out, "dev") {
		t.Error("expected top pane 'dev' session")
	}
	// "work" session panes have lower CPU, should not appear as data rows
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// header + 1 data line = 2 lines
	if len(lines) != 2 {
		t.Errorf("expected 2 lines (header + 1 data), got %d", len(lines))
	}
}

func TestRenderGrouped(t *testing.T) {
	out := renderGrouped(testSummaries(), cpuSort, 0)
	if out == "" {
		t.Fatal("renderGrouped returned empty string")
	}
	if !strings.Contains(out, "SESSION") {
		t.Error("missing header column SESSION")
	}
	// Session group separator lines contain session name and stats
	if !strings.Contains(out, "work") {
		t.Error("missing session group 'work'")
	}
	if !strings.Contains(out, "dev") {
		t.Error("missing session group 'dev'")
	}
}

func TestRenderTree(t *testing.T) {
	out := renderTree(testSummaries(), cpuSort, 0)
	if out == "" {
		t.Fatal("renderTree returned empty string")
	}
	if !strings.Contains(out, "PID") {
		t.Error("missing header column PID")
	}
	if !strings.Contains(out, "COMMAND") {
		t.Error("missing header column COMMAND")
	}
	// Should contain process names
	if !strings.Contains(out, "cargo") {
		t.Error("missing process 'cargo'")
	}
	if !strings.Contains(out, "node") {
		t.Error("missing process 'node'")
	}
}

func TestRenderTreeTopN(t *testing.T) {
	out := renderTree(testSummaries(), cpuSort, 1)
	// Only top 1 pane (dev, with cargo/rustc/bash)
	if !strings.Contains(out, "cargo") {
		t.Error("expected process 'cargo' in top pane")
	}
	// "node" belongs to work session, should not appear
	if strings.Contains(out, "node") {
		t.Error("did not expect process 'node' from second pane")
	}
}

func TestShowPaneDetail(t *testing.T) {
	panes := []Pane{
		{Session: "work", Window: 0, PaneIdx: 1, TTY: "/dev/pts/2", PID: 200, Command: "vim"},
	}
	paneProcs := map[string][]ProcStat{
		"pts/2": {
			{PID: 200, PPID: 1, Comm: "vim", CPUPct: 0.5, MemPct: 1.1, RSS: 8192},
		},
	}

	out, err := showPaneDetail("work:0.1", panes, paneProcs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Fatal("showPaneDetail returned empty string")
	}
	if !strings.Contains(out, "work:0.1") {
		t.Error("missing pane ID in output")
	}
	if !strings.Contains(out, "vim") {
		t.Error("missing process 'vim' in output")
	}
}

func TestBuildSummaries(t *testing.T) {
	panes := []Pane{
		{Session: "work", Window: 0, PaneIdx: 0, TTY: "/dev/pts/3", PID: 100, Command: "bash"},
	}
	procs := []ProcStat{
		{PID: 100, PPID: 1, Comm: "bash", TTY: "pts/3", CPUPct: 0.1, MemPct: 0.5, RSS: 2048},
		{PID: 101, PPID: 100, Comm: "python", TTY: "pts/3", CPUPct: 20.0, MemPct: 2.0, RSS: 80000},
	}
	summaries, paneProcs := buildSummaries(panes, procs)
	if len(summaries) != 1 {
		t.Fatalf("got %d summaries, want 1", len(summaries))
	}
	if summaries[0].NumProcs != 2 {
		t.Errorf("NumProcs = %d, want 2", summaries[0].NumProcs)
	}
	if summaries[0].TopProcess != "python" {
		t.Errorf("TopProcess = %q, want %q", summaries[0].TopProcess, "python")
	}
	if len(paneProcs["pts/3"]) != 2 {
		t.Errorf("paneProcs[pts/3] = %d procs, want 2", len(paneProcs["pts/3"]))
	}
}

func TestShowPaneDetailNotFound(t *testing.T) {
	panes := []Pane{
		{Session: "work", Window: 0, PaneIdx: 0, TTY: "/dev/pts/1", PID: 100, Command: "zsh"},
	}
	paneProcs := map[string][]ProcStat{}

	_, err := showPaneDetail("nonexistent:0.0", panes, paneProcs)
	if err == nil {
		t.Fatal("expected error for non-existent pane, got nil")
	}
}
