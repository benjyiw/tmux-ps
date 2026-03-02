package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ProcStat holds resource stats for a single process.
type ProcStat struct {
	PID    int
	PPID   int
	Comm   string
	TTY    string  // short form: "pts/5"
	RSS    int64   // KB
	CPUPct float64 // lifetime average CPU%
	MemPct float64 // % of total system memory
}

// sysInfo holds system-wide constants needed for calculations.
type sysInfo struct {
	uptime      float64 // seconds
	memTotalKB  int64
	clkTck      int64
	pageSizeKB  int64
}

func getSysInfo() (sysInfo, error) {
	var info sysInfo

	// CLK_TCK is almost universally 100 on Linux
	info.clkTck = 100
	info.pageSizeKB = int64(os.Getpagesize()) / 1024

	uptime, err := readUptime()
	if err != nil {
		return info, err
	}
	info.uptime = uptime

	memTotal, err := readMemTotal()
	if err != nil {
		return info, err
	}
	info.memTotalKB = memTotal

	return info, nil
}

func readUptime() (float64, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, fmt.Errorf("reading /proc/uptime: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("/proc/uptime: unexpected format")
	}
	return strconv.ParseFloat(fields[0], 64)
}

func readMemTotal() (int64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf("reading /proc/meminfo: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return strconv.ParseInt(fields[1], 10, 64)
			}
		}
	}
	return 0, fmt.Errorf("/proc/meminfo: MemTotal not found")
}

// decodeTTY converts the tty_nr field from /proc/[pid]/stat to a short name.
// For PTY devices: major 136+ → pts, device number = (major-136)*256 + minor.
func decodeTTY(ttyNr int) string {
	if ttyNr == 0 {
		return ""
	}
	major := (ttyNr >> 8) & 0xff
	minor := (ttyNr & 0xff) | ((ttyNr >> 12) & 0xfff00)
	if major >= 136 {
		ptsNum := (major-136)*256 + minor
		return fmt.Sprintf("pts/%d", ptsNum)
	}
	return ""
}

// parseStat parses a single /proc/[pid]/stat file content.
// Returns the parsed fields needed for ProcStat calculation.
// Fields (1-indexed per proc(5)):
//
//	1: pid, 2: (comm), 4: ppid, 7: tty_nr, 14: utime, 15: stime, 22: starttime, 24: rss
func parseStat(data string) (pid int, comm string, ppid int, ttyNr int, utime, stime, starttime int64, rssPages int64, err error) {
	// comm is wrapped in parens and may contain spaces/parens, so find the last ')'
	openParen := strings.IndexByte(data, '(')
	closeParen := strings.LastIndexByte(data, ')')
	if openParen < 0 || closeParen < 0 || closeParen <= openParen {
		return 0, "", 0, 0, 0, 0, 0, 0, fmt.Errorf("malformed stat: no comm field")
	}

	pidStr := strings.TrimSpace(data[:openParen])
	pid, err = strconv.Atoi(pidStr)
	if err != nil {
		return 0, "", 0, 0, 0, 0, 0, 0, fmt.Errorf("malformed stat: bad pid %q", pidStr)
	}
	comm = data[openParen+1 : closeParen]

	// Fields after comm start at index 0 = state (field 3 in proc(5))
	rest := strings.Fields(data[closeParen+1:])
	// We need: ppid (field 4 = index 1), tty_nr (field 7 = index 4),
	//          utime (field 14 = index 11), stime (field 15 = index 12),
	//          starttime (field 22 = index 19), rss (field 24 = index 21)
	if len(rest) < 22 {
		return 0, "", 0, 0, 0, 0, 0, 0, fmt.Errorf("malformed stat: too few fields (%d)", len(rest))
	}

	ppid, _ = strconv.Atoi(rest[1])
	ttyNr, _ = strconv.Atoi(rest[4])
	utime, _ = strconv.ParseInt(rest[11], 10, 64)
	stime, _ = strconv.ParseInt(rest[12], 10, 64)
	starttime, _ = strconv.ParseInt(rest[19], 10, 64)
	rssPages, _ = strconv.ParseInt(rest[21], 10, 64)

	return pid, comm, ppid, ttyNr, utime, stime, starttime, rssPages, nil
}

// ReadAllProcs walks /proc and returns stats for all readable processes.
func ReadAllProcs() ([]ProcStat, error) {
	info, err := getSysInfo()
	if err != nil {
		return nil, err
	}

	entries, err := filepath.Glob("/proc/[0-9]*/stat")
	if err != nil {
		return nil, fmt.Errorf("globbing /proc: %w", err)
	}

	var procs []ProcStat
	for _, path := range entries {
		data, err := os.ReadFile(path)
		if err != nil {
			continue // process may have exited
		}

		pid, comm, ppid, ttyNr, utime, stime, starttime, rssPages, err := parseStat(string(data))
		if err != nil {
			continue
		}

		tty := decodeTTY(ttyNr)

		rssKB := rssPages * info.pageSizeKB

		// Lifetime average CPU%
		totalCPUSeconds := float64(utime+stime) / float64(info.clkTck)
		processUptime := info.uptime - float64(starttime)/float64(info.clkTck)
		var cpuPct float64
		if processUptime > 0 {
			cpuPct = 100.0 * totalCPUSeconds / processUptime
		}

		var memPct float64
		if info.memTotalKB > 0 {
			memPct = 100.0 * float64(rssKB) / float64(info.memTotalKB)
		}

		procs = append(procs, ProcStat{
			PID:    pid,
			PPID:   ppid,
			Comm:   comm,
			TTY:    tty,
			RSS:    rssKB,
			CPUPct: cpuPct,
			MemPct: memPct,
		})
	}

	return procs, nil
}
