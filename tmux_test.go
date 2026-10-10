package main

import (
	"os/exec"
	"sort"
	"testing"
)

func TestParsePanesWithPID(t *testing.T) {
	// Format: session window_index pane_index tty pid command
	// PID comes before command so command (which may contain spaces) is the last field.
	input := "work\t0\t0\t/dev/pts/3\t12345\tbash\nwork\t0\t1\t/dev/pts/4\t12350\tvim\n"
	panes, err := parsePanes(input)
	if err != nil {
		t.Fatalf("parsePanes() error: %v", err)
	}
	if len(panes) != 2 {
		t.Fatalf("got %d panes, want 2", len(panes))
	}
	if panes[0].PID != 12345 {
		t.Errorf("pane[0].PID = %d, want 12345", panes[0].PID)
	}
	if panes[1].PID != 12350 {
		t.Errorf("pane[1].PID = %d, want 12350", panes[1].PID)
	}
}

func TestParsePanesCommandWithSpaces(t *testing.T) {
	// Command may contain spaces; it should be captured entirely as the last field.
	input := "dev\t1\t0\t/dev/pts/5\t99999\tpython my script.py\n"
	panes, err := parsePanes(input)
	if err != nil {
		t.Fatalf("parsePanes() error: %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("got %d panes, want 1", len(panes))
	}
	if panes[0].PID != 99999 {
		t.Errorf("pane[0].PID = %d, want 99999", panes[0].PID)
	}
	if panes[0].Command != "python my script.py" {
		t.Errorf("pane[0].Command = %q, want %q", panes[0].Command, "python my script.py")
	}
}

func TestParsePanesBasicFields(t *testing.T) {
	input := "main\t2\t1\t/dev/pts/7\t54321\tnvim\n"
	panes, err := parsePanes(input)
	if err != nil {
		t.Fatalf("parsePanes() error: %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("got %d panes, want 1", len(panes))
	}
	p := panes[0]
	if p.Session != "main" {
		t.Errorf("Session = %q, want %q", p.Session, "main")
	}
	if p.Window != 2 {
		t.Errorf("Window = %d, want 2", p.Window)
	}
	if p.PaneIdx != 1 {
		t.Errorf("PaneIdx = %d, want 1", p.PaneIdx)
	}
	if p.TTY != "/dev/pts/7" {
		t.Errorf("TTY = %q, want %q", p.TTY, "/dev/pts/7")
	}
	if p.PID != 54321 {
		t.Errorf("PID = %d, want 54321", p.PID)
	}
	if p.Command != "nvim" {
		t.Errorf("Command = %q, want %q", p.Command, "nvim")
	}
}

func TestParsePanesEmpty(t *testing.T) {
	panes, err := parsePanes("")
	if err != nil {
		t.Fatalf("parsePanes() error: %v", err)
	}
	if len(panes) != 0 {
		t.Fatalf("got %d panes, want 0", len(panes))
	}
}

func TestParsePanesMalformedLines(t *testing.T) {
	// Lines with too few fields or bad integers should be skipped.
	input := "bad line\nwork\t0\t0\t/dev/pts/3\t12345\tbash\nwork\tnotanum\t0\t/dev/pts/4\t12350\tvim\n"
	panes, err := parsePanes(input)
	if err != nil {
		t.Fatalf("parsePanes() error: %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("got %d panes, want 1 (only the valid line)", len(panes))
	}
	if panes[0].PID != 12345 {
		t.Errorf("pane[0].PID = %d, want 12345", panes[0].PID)
	}
}

func TestParsePanesSessionNameWithSpaces(t *testing.T) {
	input := "wt5 - 40000 - \t1\t2\t/dev/pts/4\t777\tnode\n"
	panes, err := parsePanes(input)
	if err != nil {
		t.Fatalf("parsePanes() error: %v", err)
	}
	if len(panes) != 1 || panes[0].Session != "wt5 - 40000 - " || panes[0].Window != 1 || panes[0].PaneIdx != 2 || panes[0].Command != "node" {
		t.Fatalf("unexpected panes: %+v", panes)
	}
}

func TestParsePanesAllWorktreeStyleNames(t *testing.T) {
	names := []string{
		"6",
		"gem - 3000  - Primary Agent",
		"wt1 - 40020 - ",
		"wt2 - 40030 - Report",
		"wt4 - 40060 - Proposal Protection",
		"1 2 3 4 5",
		" leading",
		"a-b_c",
	}
	var input string
	for i, name := range names {
		input += name + "\t1\t0\t/dev/pts/" + string(rune('0'+i)) + "\t100\tbash\n"
	}
	panes, err := parsePanes(input)
	if err != nil {
		t.Fatalf("parsePanes() error: %v", err)
	}
	if len(panes) != len(names) {
		t.Fatalf("got %d panes, want %d", len(panes), len(names))
	}
	for i, name := range names {
		if panes[i].Session != name {
			t.Errorf("pane[%d].Session = %q, want %q", i, panes[i].Session, name)
		}
		if panes[i].Window != 1 || panes[i].PID != 100 || panes[i].Command != "bash" {
			t.Errorf("pane[%d] fields misparsed: %+v", i, panes[i])
		}
	}
}

func TestListPanesRealTmuxSessionNamesWithSpaces(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	tmpDir := t.TempDir()
	t.Setenv("TMUX_TMPDIR", tmpDir)
	t.Setenv("TMUX", "")
	names := []string{"6", "wt1 - 40020 - ", "gem - 3000  - Primary Agent", "wt4 - 40060 - Proposal Protection"}
	for _, name := range names {
		if out, err := exec.Command("tmux", "new-session", "-d", "-s", name).CombinedOutput(); err != nil {
			t.Fatalf("tmux new-session %q: %v: %s", name, err, out)
		}
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-server").Run() })

	panes, err := ListPanes()
	if err != nil {
		t.Fatalf("ListPanes() error: %v", err)
	}
	var got []string
	for _, pane := range panes {
		got = append(got, pane.Session)
	}
	want := append([]string(nil), names...)
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("got sessions %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got sessions %q, want %q", got, want)
		}
	}
}
