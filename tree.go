package main

// BuildChildMap creates a mapping from PID to list of child PIDs.
func BuildChildMap(procs []ProcStat) map[int][]int {
	cm := make(map[int][]int)
	for _, p := range procs {
		cm[p.PPID] = append(cm[p.PPID], p.PID)
	}
	return cm
}

// Descendants returns the set of all PIDs that are descendants of root
// (including root itself). Uses iterative BFS to avoid stack overflow.
func Descendants(childMap map[int][]int, root int) map[int]bool {
	seen := make(map[int]bool)
	queue := []int{root}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		queue = append(queue, childMap[pid]...)
	}
	return seen
}
