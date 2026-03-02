package main

import (
	"testing"
)

func TestParsePanesWithPID(t *testing.T) {
	// Format: session window_index pane_index tty pid command
	// PID comes before command so command (which may contain spaces) is the last field.
	input := "work 0 0 /dev/pts/3 12345 bash\nwork 0 1 /dev/pts/4 12350 vim\n"
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
	input := "dev 1 0 /dev/pts/5 99999 python my script.py\n"
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
	input := "main 2 1 /dev/pts/7 54321 nvim\n"
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
	input := "bad line\nwork 0 0 /dev/pts/3 12345 bash\nwork notanum 0 /dev/pts/4 12350 vim\n"
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
