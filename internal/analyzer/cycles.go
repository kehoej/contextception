package analyzer

import (
	"github.com/kehoej/contextception/internal/db"
)

const (
	maxCycleDepth = 6 // maximum DFS depth to prevent performance issues
	maxCycles     = 5  // maximum number of cycles to report
)

// detectCycles finds import cycles that the subject participates in.
// Uses bounded DFS following forward edges (imports) from the subject,
// returning when a path leads back to the subject.
// candidatePaths provides the full set of known files to pre-load edges for,
// avoiding per-node DB queries during DFS traversal.
func detectCycles(idx *db.Index, subject string, candidatePaths []string) [][]string {
	// Pre-load forward edges for distance-1 candidates to reduce DB round-trips.
	directImports, err := idx.GetDirectImports(subject)
	if err != nil || len(directImports) == 0 {
		return nil
	}

	// Batch-load forward edges for all candidate paths.
	// This turns potentially thousands of individual DB queries into one batch query.
	edgeCache, err := idx.GetForwardEdgesBatch(candidatePaths)
	if err != nil {
		// Fallback: use individual queries via DFS.
		edgeCache = make(map[string][]string)
	}

	var cycles [][]string
	visited := make(map[string]bool)

	// DFS from each direct import of the subject.
	for _, start := range directImports {
		if len(cycles) >= maxCycles {
			break
		}
		// Clear visited for each starting path.
		for k := range visited {
			delete(visited, k)
		}
		visited[subject] = true

		path := []string{subject, start}
		dfsFindCycles(idx, subject, start, path, visited, edgeCache, &cycles)
	}

	return cycles
}

// dfsFindCycles performs bounded DFS to find cycles back to the subject.
func dfsFindCycles(idx *db.Index, subject, current string, path []string, visited map[string]bool, edgeCache map[string][]string, cycles *[][]string) {
	if len(*cycles) >= maxCycles {
		return
	}
	if len(path) > maxCycleDepth {
		return
	}

	visited[current] = true
	defer func() { visited[current] = false }()

	// Get forward edges for current node.
	imports, ok := edgeCache[current]
	if !ok {
		var err error
		imports, err = idx.GetDirectImports(current)
		if err != nil {
			return
		}
		edgeCache[current] = imports
	}

	for _, next := range imports {
		if next == subject {
			// Found a cycle back to subject.
			cycle := make([]string, len(path)+1)
			copy(cycle, path)
			cycle[len(path)] = subject
			*cycles = append(*cycles, cycle)
			if len(*cycles) >= maxCycles {
				return
			}
			continue
		}
		if visited[next] {
			continue
		}
		dfsFindCycles(idx, subject, next, append(path, next), visited, edgeCache, cycles)
	}
}

// cycleParticipants returns the set of all files that participate in any detected cycle.
func cycleParticipants(cycles [][]string) map[string]bool {
	participants := make(map[string]bool)
	for _, cycle := range cycles {
		for _, file := range cycle {
			participants[file] = true
		}
	}
	return participants
}
