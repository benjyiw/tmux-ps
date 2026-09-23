//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ReadAllProcs shells out to the BSD ps(1) that ships with macOS — there is no
// /proc on Darwin, and ps already computes the same lifetime-average CPU% and
// RSS-based MEM% that proc_linux.go derives by hand from /proc.
func ReadAllProcs() ([]ProcStat, error) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,pcpu=,pmem=,rss=,tty=,ucomm=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}

	var procs []ProcStat
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		ppid, _ := strconv.Atoi(fields[1])
		cpuPct, _ := strconv.ParseFloat(fields[2], 64)
		memPct, _ := strconv.ParseFloat(fields[3], 64)
		rss, _ := strconv.ParseInt(fields[4], 10, 64)

		tty := fields[5]
		if tty == "??" || tty == "-" {
			tty = ""
		}

		procs = append(procs, ProcStat{
			PID:    pid,
			PPID:   ppid,
			Comm:   strings.Join(fields[6:], " "),
			TTY:    tty,
			RSS:    rss,
			CPUPct: cpuPct,
			MemPct: memPct,
		})
	}

	return procs, nil
}
