package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
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
	showTree := flag.Bool("t", false, "show process trees for all panes")
	groupBySession := flag.Bool("g", false, "group by session, sorted by cumulative memory")
	watch := flag.Bool("w", false, "watch mode: periodically refresh like top")
	interval := flag.Float64("i", 2, "refresh interval in seconds (used with -w)")
	flag.Parse()

	if *watch {
		m := newWatchModel(
			time.Duration(*interval*float64(time.Second)),
			*sortField,
			*showTree,
			*groupBySession,
			*topN,
		)
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	panes, err := ListPanes()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(panes) == 0 {
		fmt.Fprintf(os.Stderr, "No tmux panes found.\n")
		os.Exit(1)
	}

	procs, err := ReadAllProcs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading /proc: %v\n", err)
		os.Exit(1)
	}

	summaries, paneProcs := buildSummaries(panes, procs)

	// Handle -p flag: show per-process detail.
	if *filterPane != "" {
		out, err := showPaneDetail(*filterPane, panes, paneProcs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(out)
		return
	}

	paneSort := makePaneSort(*sortField)

	if *showTree {
		fmt.Print(renderTree(summaries, paneSort, *topN))
	} else if *groupBySession {
		fmt.Print(renderGrouped(summaries, paneSort, *topN))
	} else {
		fmt.Print(renderFlat(summaries, paneSort, *topN))
	}
}

func makePaneSort(sortField string) func(a, b PaneSummary) bool {
	return func(a, b PaneSummary) bool {
		switch sortField {
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
		default:
			if a.CPUPct != b.CPUPct {
				return a.CPUPct > b.CPUPct
			}
			return a.MemPct > b.MemPct
		}
	}
}

// buildSummaries groups processes by pane and builds a PaneSummary for each.
// It returns the summaries and the per-pane process map (keyed by short TTY).
func buildSummaries(panes []Pane, procs []ProcStat) ([]PaneSummary, map[string][]ProcStat) {
	// Build PID lookup and child map for tree walking.
	pidMap := make(map[int]ProcStat)
	for _, p := range procs {
		pidMap[p.PID] = p
	}
	childMap := BuildChildMap(procs)

	// Group procs by pane using tree walk from pane PID.
	paneProcs := make(map[string][]ProcStat) // key: short tty
	for _, pane := range panes {
		short := strings.TrimPrefix(pane.TTY, "/dev/")
		if pane.PID > 0 {
			// Tree walk: find all descendants of pane's root process
			desc := Descendants(childMap, pane.PID)
			for pid := range desc {
				if ps, ok := pidMap[pid]; ok {
					paneProcs[short] = append(paneProcs[short], ps)
				}
			}
		} else {
			// Fallback: TTY matching (pane_pid unavailable)
			for _, p := range procs {
				if p.TTY == short {
					paneProcs[short] = append(paneProcs[short], p)
				}
			}
		}
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

	return summaries, paneProcs
}

func renderFlat(summaries []PaneSummary, less func(a, b PaneSummary) bool, topN int) string {
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

	var sb strings.Builder
	lines := strings.Split(buf.String(), "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		if i == 0 {
			sb.WriteString(bold + line + reset + "\n")
		} else if i-1 < len(summaries) {
			c := colorForCPU(summaries[i-1].CPUPct)
			sb.WriteString(c + line + reset + "\n")
		}
	}
	return sb.String()
}

func renderGrouped(summaries []PaneSummary, less func(a, b PaneSummary) bool, topN int) string {
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

	var sb strings.Builder
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
			sb.WriteString(bold + line + reset + "\n")
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
			sb.WriteString(dim + label + pad + stats + reset + "\n")
		}
		c := colorForCPU(ordered[dataIdx].CPUPct)
		sb.WriteString(c + line + reset + "\n")
	}
	return sb.String()
}

func showPaneDetail(paneID string, panes []Pane, paneProcs map[string][]ProcStat) (string, error) {
	// Find the pane.
	var target *Pane
	for i := range panes {
		if panes[i].PaneID() == paneID {
			target = &panes[i]
			break
		}
	}
	if target == nil {
		var available []string
		for _, p := range panes {
			available = append(available, p.PaneID())
		}
		return "", fmt.Errorf("pane %q not found\nAvailable panes:\n  %s", paneID, strings.Join(available, "\n  "))
	}

	short := strings.TrimPrefix(target.TTY, "/dev/")
	pp := paneProcs[short]

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%sProcesses in pane %s (%s)%s\n\n", bold, paneID, target.TTY, reset))

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
			sb.WriteString(bold + line + reset + "\n")
		} else if i-1 < len(pp) {
			c := colorForCPU(pp[i-1].CPUPct)
			sb.WriteString(c + line + reset + "\n")
		}
	}
	return sb.String(), nil
}

type treeEntry struct {
	proc      ProcStat
	prefix    string
	paneLabel string // non-empty for the first entry of each pane
}

func renderTree(summaries []PaneSummary, less func(a, b PaneSummary) bool, topN int) string {
	sort.Slice(summaries, func(i, j int) bool {
		return less(summaries[i], summaries[j])
	})
	if topN > 0 && topN < len(summaries) {
		summaries = summaries[:topN]
	}

	// Build all tree entries across all panes.
	var allEntries []treeEntry
	for _, s := range summaries {
		if len(s.Procs) == 0 {
			continue
		}
		paneEntries := buildPaneTree(s)
		// Tag the first entry with the pane label.
		if len(paneEntries) > 0 {
			paneEntries[0].paneLabel = fmt.Sprintf("[%s]", s.Pane.PaneID())
		}
		allEntries = append(allEntries, paneEntries...)
	}

	if len(allEntries) == 0 {
		return dim + "(no processes)" + reset + "\n"
	}

	// Find max display widths for column alignment.
	maxPIDCol := 3
	maxCmdCol := 7 // "COMMAND"
	for _, e := range allEntries {
		w := utf8.RuneCountInString(e.prefix) + len(fmt.Sprintf("%d", e.proc.PID))
		if w > maxPIDCol {
			maxPIDCol = w
		}
		if len(e.proc.Comm) > maxCmdCol {
			maxCmdCol = len(e.proc.Comm)
		}
	}

	var sb strings.Builder

	// Print header.
	hdr := fmt.Sprintf("%-*s  %5s  %5s  %5s  %-*s  %s",
		maxPIDCol, "PID", "CPU%", "MEM%", "RSS", maxCmdCol, "COMMAND", "PANE")
	sb.WriteString(bold + hdr + reset + "\n")

	// Print entries.
	for _, e := range allEntries {
		pidStr := fmt.Sprintf("%s%d", e.prefix, e.proc.PID)
		padLen := maxPIDCol - utf8.RuneCountInString(pidStr)
		if padLen < 0 {
			padLen = 0
		}
		line := fmt.Sprintf("%s%s  %5.1f  %5.1f  %5s  %-*s",
			pidStr, strings.Repeat(" ", padLen),
			e.proc.CPUPct, e.proc.MemPct, formatRSS(e.proc.RSS),
			maxCmdCol, e.proc.Comm)
		if e.paneLabel != "" {
			line += "  " + dim + e.paneLabel + reset
		}
		c := colorForCPU(e.proc.CPUPct)
		sb.WriteString(c + line + reset + "\n")
	}
	return sb.String()
}

func buildPaneTree(s PaneSummary) []treeEntry {
	pidMap := make(map[int]ProcStat)
	childMap := make(map[int][]int)
	for _, p := range s.Procs {
		pidMap[p.PID] = p
		childMap[p.PPID] = append(childMap[p.PPID], p.PID)
	}

	// Sort children by CPU desc at each level.
	for ppid := range childMap {
		kids := childMap[ppid]
		sort.Slice(kids, func(i, j int) bool {
			return pidMap[kids[i]].CPUPct > pidMap[kids[j]].CPUPct
		})
	}

	var entries []treeEntry

	var walk func(pid int, prefix string, isLast bool, depth int)
	walk = func(pid int, prefix string, isLast bool, depth int) {
		if _, ok := pidMap[pid]; !ok {
			return
		}

		var connector string
		if depth == 0 {
			connector = ""
		} else if isLast {
			connector = prefix + "└─"
		} else {
			connector = prefix + "├─"
		}

		entries = append(entries, treeEntry{proc: pidMap[pid], prefix: connector})

		var childPrefix string
		if depth == 0 {
			childPrefix = ""
		} else if isLast {
			childPrefix = prefix + "  "
		} else {
			childPrefix = prefix + "│ "
		}

		for i, child := range childMap[pid] {
			walk(child, childPrefix, i == len(childMap[pid])-1, depth+1)
		}
	}

	walk(s.Pane.PID, "", true, 0)

	// Collect orphans.
	visited := make(map[int]bool)
	for _, e := range entries {
		visited[e.proc.PID] = true
	}
	for _, p := range s.Procs {
		if !visited[p.PID] {
			entries = append(entries, treeEntry{proc: p, prefix: "? "})
		}
	}

	return entries
}
