package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type refreshMsg time.Time

type watchModel struct {
	interval       time.Duration
	sortField      string
	showTree       bool
	groupBySession bool
	topN           int
	summaries      []PaneSummary
	paneProcs      map[string][]ProcStat
	panes          []Pane
	err            error
	width          int
	height         int
	scrollOffset   int
}

var sortFields = []string{"cpu", "mem", "rss", "procs"}

func newWatchModel(interval time.Duration, sortField string, showTree, groupBySession bool, topN int) watchModel {
	return watchModel{
		interval:       interval,
		sortField:      sortField,
		showTree:       showTree,
		groupBySession: groupBySession,
		topN:           topN,
	}
}

func (m watchModel) Init() tea.Cmd {
	return tea.Batch(m.refresh(), m.tick())
}

func (m watchModel) tick() tea.Cmd {
	return tea.Tick(m.interval, func(t time.Time) tea.Msg {
		return refreshMsg(t)
	})
}

func (m watchModel) refresh() tea.Cmd {
	return func() tea.Msg {
		return refreshMsg(time.Now())
	}
}

func (m watchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "s":
			for i, f := range sortFields {
				if f == m.sortField {
					m.sortField = sortFields[(i+1)%len(sortFields)]
					break
				}
			}
		case "t":
			m.showTree = !m.showTree
			if m.showTree {
				m.groupBySession = false
			}
		case "g":
			m.groupBySession = !m.groupBySession
			if m.groupBySession {
				m.showTree = false
			}
		case "up", "k":
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		case "down", "j":
			if max := m.maxScroll(); m.scrollOffset < max {
				m.scrollOffset++
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case refreshMsg:
		panes, err := ListPanes()
		if err != nil {
			m.err = err
			return m, m.tick()
		}
		procs, err := ReadAllProcs()
		if err != nil {
			m.err = err
			return m, m.tick()
		}
		summaries, paneProcs := buildSummaries(panes, procs)
		m.panes = panes
		m.summaries = summaries
		m.paneProcs = paneProcs
		m.err = nil
		return m, m.tick()
	}

	return m, nil
}

func (m watchModel) tableLineCount() int {
	if len(m.summaries) == 0 {
		return 0
	}
	paneSort := makePaneSort(m.sortField)
	var table string
	if m.showTree {
		table = renderTree(m.summaries, paneSort, m.topN)
	} else if m.groupBySession {
		table = renderGrouped(m.summaries, paneSort, m.topN)
	} else {
		table = renderFlat(m.summaries, paneSort, m.topN)
	}
	return len(strings.Split(table, "\n"))
}

func (m watchModel) maxScroll() int {
	const headerLines = 3
	if m.height <= 0 {
		return 0
	}
	total := m.tableLineCount()
	available := m.height - headerLines
	if total > available {
		available-- // indicator line
	}
	if available < 1 {
		available = 1
	}
	max := total - available
	if max < 0 {
		return 0
	}
	return max
}

func (m watchModel) View() string {
	var sb strings.Builder

	now := time.Now().Format("2006-01-02 15:04:05")
	var totalCPU, totalMem float64
	var totalRSS int64
	for _, s := range m.summaries {
		totalCPU += s.CPUPct
		totalMem += s.MemPct
		totalRSS += s.RSS
	}
	header := fmt.Sprintf("%stmux-ps%s — every %s — %s — %d panes — %.1f%% cpu  %.1f%% mem  %s rss",
		bold, reset,
		m.interval, now, len(m.summaries),
		totalCPU, totalMem, formatRSS(totalRSS))
	sb.WriteString(header)
	sb.WriteString("\n")

	mode := "flat"
	if m.showTree {
		mode = "tree"
	} else if m.groupBySession {
		mode = "grouped"
	}
	sb.WriteString(fmt.Sprintf("%ssort: %s | mode: %s | q:quit s:sort t:tree g:group ↑↓:scroll%s\n\n",
		dim, m.sortField, mode, reset))

	if m.err != nil {
		sb.WriteString(fmt.Sprintf("%sError: %v%s\n", red, m.err, reset))
		return sb.String()
	}

	if len(m.summaries) == 0 {
		sb.WriteString(dim + "(no data yet)" + reset + "\n")
		return sb.String()
	}

	paneSort := makePaneSort(m.sortField)
	if m.showTree {
		sb.WriteString(renderTree(m.summaries, paneSort, m.topN))
	} else if m.groupBySession {
		sb.WriteString(renderGrouped(m.summaries, paneSort, m.topN))
	} else {
		sb.WriteString(renderFlat(m.summaries, paneSort, m.topN))
	}

	if m.height <= 0 {
		return sb.String()
	}

	all := strings.Split(sb.String(), "\n")

	// Header is the first 3 lines (title, status bar, blank line).
	const headerLines = 3
	if len(all) <= headerLines {
		return sb.String()
	}

	headerPart := all[:headerLines]
	tableLines := all[headerLines:]

	// Reserve one line for scroll indicator when content overflows.
	availableHeight := m.height - headerLines
	overflows := len(tableLines) > availableHeight
	if overflows {
		availableHeight-- // room for indicator line
	}
	if availableHeight < 1 {
		availableHeight = 1
	}

	// Clamp scroll offset.
	maxOffset := len(tableLines) - availableHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}

	end := m.scrollOffset + availableHeight
	if end > len(tableLines) {
		end = len(tableLines)
	}
	visible := tableLines[m.scrollOffset:end]

	out := append(headerPart, visible...)
	if overflows {
		indicator := fmt.Sprintf("%s↕ %d/%d%s", dim, m.scrollOffset+1, len(tableLines), reset)
		out = append(out, indicator)
	}

	return strings.Join(out, "\n")
}
