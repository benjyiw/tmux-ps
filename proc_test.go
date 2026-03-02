package main

import (
	"math"
	"testing"
)

func TestDecodeTTY(t *testing.T) {
	tests := []struct {
		name   string
		ttyNr  int
		want   string
	}{
		{"zero", 0, ""},
		{"pts/0", 0x8800, "pts/0"},   // major=136, minor=0
		{"pts/1", 0x8801, "pts/1"},   // major=136, minor=1
		{"pts/5", 0x8805, "pts/5"},   // major=136, minor=5
		{"pts/256", 0x8900, "pts/256"}, // major=137, minor=0 → (137-136)*256+0=256
		{"non-pty major 4", 0x0401, ""}, // major=4 (ttyS), not a PTY
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeTTY(tt.ttyNr)
			if got != tt.want {
				t.Errorf("decodeTTY(0x%x) = %q, want %q", tt.ttyNr, got, tt.want)
			}
		})
	}
}

func TestParseStat(t *testing.T) {
	// Realistic /proc/[pid]/stat line (simplified)
	// Fields: pid (comm) state ppid pgrp session tty_nr tpgid flags
	//         minflt cminflt majflt cmajflt utime stime cutime cstime
	//         priority nice num_threads itrealvalue starttime vsize rss ...
	line := "12345 (python gen.py) S 12340 12345 12340 34819 12345 4194304 " +
		"1000 0 0 0 500 100 0 0 20 0 1 0 98765 100000000 5000 " +
		"18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0"

	pid, comm, ppid, ttyNr, utime, stime, starttime, rssPages, err := parseStat(line)
	if err != nil {
		t.Fatalf("parseStat() error: %v", err)
	}
	if pid != 12345 {
		t.Errorf("pid = %d, want 12345", pid)
	}
	if comm != "python gen.py" {
		t.Errorf("comm = %q, want %q", comm, "python gen.py")
	}
	if ppid != 12340 {
		t.Errorf("ppid = %d, want 12340", ppid)
	}
	// tty_nr is field 7, index 4 in rest after comm
	// From our line: 34819 = 0x8803 → major=136, minor=3 → pts/3
	if ttyNr != 34819 {
		t.Errorf("ttyNr = %d, want 34819", ttyNr)
	}
	if utime != 500 {
		t.Errorf("utime = %d, want 500", utime)
	}
	if stime != 100 {
		t.Errorf("stime = %d, want 100", stime)
	}
	if starttime != 98765 {
		t.Errorf("starttime = %d, want 98765", starttime)
	}
	if rssPages != 5000 {
		t.Errorf("rssPages = %d, want 5000", rssPages)
	}
}

func TestParseStatCommWithParens(t *testing.T) {
	// comm field can contain parentheses
	line := "999 (bash (deleted)) S 1 999 999 0 999 0 0 0 0 0 10 5 0 0 20 0 1 0 1000 50000 200 " +
		"18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0"

	pid, comm, _, _, _, _, _, _, err := parseStat(line)
	if err != nil {
		t.Fatalf("parseStat() error: %v", err)
	}
	if pid != 999 {
		t.Errorf("pid = %d, want 999", pid)
	}
	if comm != "bash (deleted)" {
		t.Errorf("comm = %q, want %q", comm, "bash (deleted)")
	}
}

func TestParseStatMalformed(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"empty", ""},
		{"no parens", "123 bash S 1"},
		{"too few fields", "123 (bash) S 1 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := parseStat(tt.data)
			if err == nil {
				t.Error("parseStat() expected error, got nil")
			}
		})
	}
}

func TestDecodeTTYFromStat(t *testing.T) {
	// tty_nr 34819 = 0x8803 → major 136, minor 3 → pts/3
	got := decodeTTY(34819)
	if got != "pts/3" {
		t.Errorf("decodeTTY(34819) = %q, want %q", got, "pts/3")
	}
}

func TestCPUPctCalculation(t *testing.T) {
	// Simulate: utime=500, stime=100, starttime=98765 ticks, uptime=2000s, clkTck=100
	clkTck := int64(100)
	uptime := 2000.0
	utime := int64(500)
	stime := int64(100)
	starttime := int64(98765)

	totalCPUSeconds := float64(utime+stime) / float64(clkTck)
	processUptime := uptime - float64(starttime)/float64(clkTck)
	cpuPct := 100.0 * totalCPUSeconds / processUptime

	// totalCPUSeconds = 600/100 = 6.0
	// processUptime = 2000 - 987.65 = 1012.35
	// cpuPct = 100 * 6.0 / 1012.35 ≈ 0.5927
	expected := 100.0 * 6.0 / 1012.35
	if math.Abs(cpuPct-expected) > 0.001 {
		t.Errorf("cpuPct = %f, want ≈ %f", cpuPct, expected)
	}
}

func TestFormatRSS(t *testing.T) {
	tests := []struct {
		kb   int64
		want string
	}{
		{500, "500K"},
		{1024, "1M"},
		{12800, "12M"},
		{1048576, "1.0G"},
		{1572864, "1.5G"},
	}
	for _, tt := range tests {
		got := formatRSS(tt.kb)
		if got != tt.want {
			t.Errorf("formatRSS(%d) = %q, want %q", tt.kb, got, tt.want)
		}
	}
}
