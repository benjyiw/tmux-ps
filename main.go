package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
)

// ANSI color codes.
var (
	bold   = "\033[1m"
	dim    = "\033[2m"
	reset  = "\033[0m"
	red    = "\033[31m"
	yellow = "\033[33m"
	green  = "\033[32m"
)

func init() {
	// Disable colors when stdout is not a terminal (e.g. piped).
	if fi, err := os.Stdout.Stat(); err == nil {
		if fi.Mode()&os.ModeCharDevice == 0 {
			bold, dim, reset = "", "", ""
			red, yellow, green = "", "", ""
		}
	}
}

// PaneSummary is the aggregated stats for one tmux pane.
type PaneSummary struct {
	Pane       *Pane
	CPUPct     float64
	MemPct     float64
	RSS        int64 // KB
	NumProcs   int
	TopProcess string
	TopCPU     float64
	Procs      []ProcStat
}

func formatRSS(kb int64) string {
	switch {
	case kb >= 1048576:
		return fmt.Sprintf("%.1fG", float64(kb)/1048576)
	case kb >= 1024:
		return fmt.Sprintf("%.0fM", float64(kb)/1024)
	default:
		return fmt.Sprintf("%dK", kb)
	}
}

func colorForCPU(pct float64) string {
	switch {
	case pct >= 50:
		return red
	case pct >= 10:
		return yellow
	default:
		return green
	}
}

func main() {
	topN := flag.Int("n", 0, "show top N panes (0 = all)")
	sortField := flag.String("s", "cpu", "sort by: cpu, mem, rss, procs")
	filterPane := flag.String("p", "", "show all processes for a specific pane (e.g. \"work:0.1\")")
	showTree := flag.Bool("t", false, "show process tree for top pane")
	groupBySession := flag.Bool("g", false, "group by session, sorted by cumulative memory")
	flag.Parse()

	panes, err := ListPanes()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(panes) == 0 {
		fmt.Fprintf(os.Stderr, "No tmux panes found.\n")
		os.Exit(1)
	}

	ttyMap := TTYToPaneMap(panes)

	procs, err := ReadAllProcs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading /proc: %v\n", err)
		os.Exit(1)
	}

	// Group procs by pane.
	paneProcs := make(map[string][]ProcStat) // key: short tty
	for _, p := range procs {
		if _, ok := ttyMap[p.TTY]; ok {
			paneProcs[p.TTY] = append(paneProcs[p.TTY], p)
		}
	}

	// Handle -p flag: show per-process detail.
	if *filterPane != "" {
		showPaneDetail(*filterPane, panes, paneProcs)
		return
	}

	// Build summaries.
	var summaries []PaneSummary
	for i := range panes {
		short := strings.TrimPrefix(panes[i].TTY, "/dev/")
		pp := paneProcs[short]

		var s PaneSummary
		s.Pane = &panes[i]
		s.Procs = pp
		for _, p := range pp {
			s.CPUPct += p.CPUPct
			s.MemPct += p.MemPct
			s.RSS += p.RSS
			s.NumProcs++
			if p.CPUPct > s.TopCPU {
				s.TopCPU = p.CPUPct
				s.TopProcess = p.Comm
			}
		}
		if s.TopProcess == "" {
			s.TopProcess = panes[i].Command
		}
		summaries = append(summaries, s)
	}

	// Pane sort comparator used by both flat and grouped modes.
	paneSort := func(a, b PaneSummary) bool {
		switch *sortField {
		case "mem":
			if a.MemPct != b.MemPct {
				return a.MemPct > b.MemPct
			}
			return a.CPUPct > b.CPUPct
		case "rss":
			if a.RSS != b.RSS {
				return a.RSS > b.RSS
			}
			return a.CPUPct > b.CPUPct
		case "procs":
			if a.NumProcs != b.NumProcs {
				return a.NumProcs > b.NumProcs
			}
			return a.CPUPct > b.CPUPct
		default: // cpu
			if a.CPUPct != b.CPUPct {
				return a.CPUPct > b.CPUPct
			}
			return a.MemPct > b.MemPct
		}
	}

	if *groupBySession {
		renderGrouped(summaries, paneSort, *topN)
	} else {
		renderFlat(summaries, paneSort, *topN)
	}

	// Handle -t flag.
	if *showTree && len(summaries) > 0 {
		sort.Slice(summaries, func(i, j int) bool {
			return paneSort(summaries[i], summaries[j])
		})
		showProcessTree(summaries[0])
	}
}

func renderFlat(summaries []PaneSummary, less func(a, b PaneSummary) bool, topN int) {
	sort.Slice(summaries, func(i, j int) bool {
		return less(summaries[i], summaries[j])
	})

	if topN > 0 && topN < len(summaries) {
		summaries = summaries[:topN]
	}

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "SESSION\tWIN\tPANE\tTTY\tCPU%%\tMEM%%\tRSS\tPROCS\tTOP PROCESS\n")
	for _, s := range summaries {
		fmt.Fprintf(w, "%s\t%d\t.%d\t%s\t%5.1f\t%5.1f\t%s\t%d\t%s\n",
			s.Pane.Session,
			s.Pane.Window,
			s.Pane.PaneIdx,
			s.Pane.TTY,
			s.CPUPct,
			s.MemPct,
			formatRSS(s.RSS),
			s.NumProcs,
			s.TopProcess,
		)
	}
	w.Flush()

	lines := strings.Split(buf.String(), "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		if i == 0 {
			fmt.Println(bold + line + reset)
		} else if i-1 < len(summaries) {
			c := colorForCPU(summaries[i-1].CPUPct)
			fmt.Println(c + line + reset)
		}
	}
}

func renderGrouped(summaries []PaneSummary, less func(a, b PaneSummary) bool, topN int) {
	type sessionGroup struct {
		name     string
		panes    []PaneSummary
		totalCPU float64
		totalMem float64
		totalRSS int64
		numProcs int
	}
	sessionMap := make(map[string]*sessionGroup)
	var sessionNames []string
	for _, s := range summaries {
		name := s.Pane.Session
		g, ok := sessionMap[name]
		if !ok {
			g = &sessionGroup{name: name}
			sessionMap[name] = g
			sessionNames = append(sessionNames, name)
		}
		g.panes = append(g.panes, s)
		g.totalCPU += s.CPUPct
		g.totalMem += s.MemPct
		g.totalRSS += s.RSS
		g.numProcs += s.NumProcs
	}

	// Sort sessions by cumulative memory desc.
	sort.Slice(sessionNames, func(i, j int) bool {
		si, sj := sessionMap[sessionNames[i]], sessionMap[sessionNames[j]]
		if si.totalMem != sj.totalMem {
			return si.totalMem > sj.totalMem
		}
		return si.totalCPU > sj.totalCPU
	})

	// Build ordered pane list with session boundaries.
	var ordered []PaneSummary
	var boundaries []int
	for _, name := range sessionNames {
		g := sessionMap[name]
		sort.Slice(g.panes, func(i, j int) bool {
			return less(g.panes[i], g.panes[j])
		})
		boundaries = append(boundaries, len(ordered))
		ordered = append(ordered, g.panes...)
	}

	if topN > 0 && topN < len(ordered) {
		ordered = ordered[:topN]
	}

	// Write plain text to tabwriter: session name on first row of each group.
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "SESSION\tWIN\tPANE\tTTY\tCPU%%\tMEM%%\tRSS\tPROCS\tTOP PROCESS\n")
	for i, s := range ordered {
		session := ""
		for _, b := range boundaries {
			if b == i {
				session = s.Pane.Session
				break
			}
		}
		fmt.Fprintf(w, "%s\t%d\t.%d\t%s\t%5.1f\t%5.1f\t%s\t%d\t%s\n",
			session,
			s.Pane.Window,
			s.Pane.PaneIdx,
			s.Pane.TTY,
			s.CPUPct,
			s.MemPct,
			formatRSS(s.RSS),
			s.NumProcs,
			s.TopProcess,
		)
	}
	w.Flush()

	// Build boundary lookup: data line index → session group index.
	boundarySet := make(map[int]int)
	for gi, b := range boundaries {
		boundarySet[b] = gi
	}

	lines := strings.Split(buf.String(), "\n")
	tableWidth := 0
	if len(lines) > 0 {
		tableWidth = len(lines[0])
	}
	for i, line := range lines {
		if line == "" {
			continue
		}
		if i == 0 {
			fmt.Println(bold + line + reset)
			continue
		}
		dataIdx := i - 1
		if dataIdx >= len(ordered) {
			continue
		}
		if gi, ok := boundarySet[dataIdx]; ok {
			g := sessionMap[sessionNames[gi]]
			label := fmt.Sprintf("━ %s ", g.name)
			stats := fmt.Sprintf("  %.1f%% cpu  %.1f%% mem  %s rss  %d procs",
				g.totalCPU, g.totalMem, formatRSS(g.totalRSS), g.numProcs)
			padLen := tableWidth - len([]rune(label)) - len([]rune(stats))
			if padLen < 4 {
				padLen = 4
			}
			pad := strings.Repeat("━", padLen)
			fmt.Println(dim + label + pad + stats + reset)
		}
		c := colorForCPU(ordered[dataIdx].CPUPct)
		fmt.Println(c + line + reset)
	}
}

func showPaneDetail(paneID string, panes []Pane, paneProcs map[string][]ProcStat) {
	// Find the pane.
	var target *Pane
	for i := range panes {
		if panes[i].PaneID() == paneID {
			target = &panes[i]
			break
		}
	}
	if target == nil {
		fmt.Fprintf(os.Stderr, "Error: pane %q not found\nAvailable panes:\n", paneID)
		for _, p := range panes {
			fmt.Fprintf(os.Stderr, "  %s\n", p.PaneID())
		}
		os.Exit(1)
	}

	short := strings.TrimPrefix(target.TTY, "/dev/")
	pp := paneProcs[short]

	fmt.Printf("%sProcesses in pane %s (%s)%s\n\n", bold, paneID, target.TTY, reset)

	// Sort by CPU desc.
	sort.Slice(pp, func(i, j int) bool {
		return pp[i].CPUPct > pp[j].CPUPct
	})

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "PID\tCPU%%\tMEM%%\tRSS\tCOMMAND\n")
	for _, p := range pp {
		fmt.Fprintf(w, "%d\t%5.1f\t%5.1f\t%s\t%s\n",
			p.PID, p.CPUPct, p.MemPct, formatRSS(p.RSS), p.Comm)
	}
	w.Flush()

	lines := strings.Split(buf.String(), "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		if i == 0 {
			fmt.Println(bold + line + reset)
		} else if i-1 < len(pp) {
			c := colorForCPU(pp[i-1].CPUPct)
			fmt.Println(c + line + reset)
		}
	}
}

func showProcessTree(top PaneSummary) {
	fmt.Printf("\n%sProcess tree for top pane %s (%s):%s\n\n",
		bold, top.Pane.PaneID(), top.Pane.TTY, reset)

	// Sort by CPU desc.
	procs := make([]ProcStat, len(top.Procs))
	copy(procs, top.Procs)
	sort.Slice(procs, func(i, j int) bool {
		return procs[i].CPUPct > procs[j].CPUPct
	})

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "PID\tCPU%%\tMEM%%\tRSS\tCOMMAND\n")
	for _, p := range procs {
		fmt.Fprintf(w, "%d\t%5.1f\t%5.1f\t%s\t%s\n",
			p.PID, p.CPUPct, p.MemPct, formatRSS(p.RSS), p.Comm)
	}
	w.Flush()

	lines := strings.Split(buf.String(), "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		if i == 0 {
			fmt.Println(bold + line + reset)
		} else if i-1 < len(procs) {
			c := colorForCPU(procs[i-1].CPUPct)
			fmt.Println(c + line + reset)
		}
	}
}
