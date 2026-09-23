package main

// ProcStat holds resource stats for a single process.
type ProcStat struct {
	PID    int
	PPID   int
	Comm   string
	TTY    string  // short form: "pts/5" (Linux) or "ttys003" (macOS)
	RSS    int64   // KB
	CPUPct float64 // lifetime average CPU%
	MemPct float64 // % of total system memory
}

// ReadAllProcs returns resource stats for every readable process on the system.
// Implemented per-OS: proc_linux.go reads /proc directly, proc_darwin.go shells out to ps.
