package main

import "testing"

func TestBuildChildMap(t *testing.T) {
	procs := []ProcStat{
		{PID: 1, PPID: 0},
		{PID: 10, PPID: 1},
		{PID: 20, PPID: 10},
		{PID: 21, PPID: 10},
		{PID: 30, PPID: 20},
	}
	cm := BuildChildMap(procs)
	if len(cm[1]) != 1 {
		t.Errorf("children of 1: got %d, want 1", len(cm[1]))
	}
	if len(cm[10]) != 2 {
		t.Errorf("children of 10: got %d, want 2", len(cm[10]))
	}
	if len(cm[20]) != 1 {
		t.Errorf("children of 20: got %d, want 1", len(cm[20]))
	}
}

func TestDescendants(t *testing.T) {
	procs := []ProcStat{
		{PID: 1, PPID: 0},
		{PID: 10, PPID: 1},
		{PID: 20, PPID: 10},
		{PID: 21, PPID: 10},
		{PID: 30, PPID: 20},
		{PID: 99, PPID: 1}, // sibling branch, should NOT be included
	}
	cm := BuildChildMap(procs)
	desc := Descendants(cm, 10)

	// Should include 10 (root), 20, 21, 30
	if len(desc) != 4 {
		t.Errorf("got %d descendants, want 4: %v", len(desc), desc)
	}
	for _, want := range []int{10, 20, 21, 30} {
		if !desc[want] {
			t.Errorf("missing PID %d in descendants", want)
		}
	}
	if desc[99] {
		t.Error("PID 99 should not be a descendant of 10")
	}
}

func TestDescendantsEmpty(t *testing.T) {
	cm := map[int][]int{}
	desc := Descendants(cm, 999)
	// Should contain just the root itself
	if len(desc) != 1 || !desc[999] {
		t.Errorf("expected {999}, got %v", desc)
	}
}
