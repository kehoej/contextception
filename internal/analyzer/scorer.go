package analyzer

import (
	"sort"
)

// Scoring weights per ADR-0009 MVP.
const (
	weightIndegree   = 4.0
	weightDistance    = 3.0
	weightEntrypoint = 1.0
	weightAPISurface = 1.5
	proximityBonus   = 2.0

	apiSurfaceThreshold = 3 // indegree >= this counts as API surface

	// Historical modifier weights (ADR-0009, ADR-0010).
	weightCoChange = 2.0
	weightChurn    = 1.0
	historicalCap  = 0.5 // cap: historical_modifier <= 0.5 * structural_weight

	// Relaxed historical cap for 2-hop candidates: co-change can dominate
	// over indegree at distance 2, letting co-change partners outrank noise.
	historicalCapTwoHop = 1.5

	// Bonus for transitive callers discovered via barrel-piercing traversal.
	transitiveCallerBonus = 1.0
)

// scoredCandidate pairs a candidate with its computed score.
type scoredCandidate struct {
	candidate
	Score float64
}

// scoreAll computes scores for all candidates and returns them sorted.
func scoreAll(candidates []candidate, maxIndegree int) []scoredCandidate {
	// Compute maxCoChangeFreq from the candidate set.
	maxCoChangeFreq := 0
	for _, c := range candidates {
		if c.CoChangeFreq > maxCoChangeFreq {
			maxCoChangeFreq = c.CoChangeFreq
		}
	}

	scored := make([]scoredCandidate, len(candidates))
	for i, c := range candidates {
		scored[i] = scoredCandidate{
			candidate: c,
			Score:     computeScore(c, maxIndegree, maxCoChangeFreq),
		}
	}

	// Deterministic sort: score desc -> indegree desc -> path asc.
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		if scored[i].Signals.Indegree != scored[j].Signals.Indegree {
			return scored[i].Signals.Indegree > scored[j].Signals.Indegree
		}
		return scored[i].Path < scored[j].Path
	})

	return scored
}

// computeScore calculates the relevance score for a single candidate.
// The score combines structural weight (indegree, distance, entrypoint, API surface)
// with a capped historical modifier (co-change frequency, churn).
func computeScore(c candidate, maxIndegree int, maxCoChangeFreq int) float64 {
	// Subject gets max possible score.
	if c.Distance == 0 {
		return subjectScore
	}

	// Normalized indegree: 0.0-1.0.
	var normalizedIndegree float64
	if maxIndegree > 0 {
		normalizedIndegree = float64(c.Signals.Indegree) / float64(maxIndegree)
	}

	// Distance score: 1.0 for direct, 0.0 for two-hop.
	var distanceScore float64
	if c.Distance == 1 {
		distanceScore = 1.0
	}

	// Entrypoint score.
	var entrypointScore float64
	if c.Signals.IsEntrypoint {
		entrypointScore = 1.0
	}

	// API surface score.
	var apiSurfaceScore float64
	if c.Signals.Indegree >= apiSurfaceThreshold {
		apiSurfaceScore = 1.0
	}

	// Proximity modifier.
	var proximity float64
	if c.Distance == 1 {
		proximity = proximityBonus
	}

	// Transitive caller bonus (barrel-piercing discovery).
	var tcBonus float64
	if c.IsTransitiveCaller {
		tcBonus = transitiveCallerBonus
	}

	structuralWeight := (weightIndegree * normalizedIndegree) +
		(weightDistance * distanceScore) +
		(weightEntrypoint * entrypointScore) +
		(weightAPISurface * apiSurfaceScore) +
		proximity + tcBonus

	// Historical modifier (capped per ADR-0009).
	// Use a relaxed cap for 2-hop candidates so co-change can differentiate them.
	var normalizedCoChange float64
	if maxCoChangeFreq > 0 {
		normalizedCoChange = float64(c.CoChangeFreq) / float64(maxCoChangeFreq)
	}
	historicalModifier := (weightCoChange * normalizedCoChange) + (weightChurn * c.Churn)
	capMult := historicalCap
	if c.Distance == 2 {
		capMult = historicalCapTwoHop
	}
	cap := capMult * structuralWeight
	if historicalModifier > cap {
		historicalModifier = cap
	}

	return structuralWeight + historicalModifier
}
