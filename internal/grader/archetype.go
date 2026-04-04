package grader

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/kehoej/contextception/internal/classify"
	"github.com/kehoej/contextception/internal/db"
)

// Archetype category names.
const (
	// Core archetypes (original 10).
	ArchService    = "Service/Controller"
	ArchModel      = "Model/Schema"
	ArchMiddleware = "Middleware/Plugin"
	ArchHighFanIn  = "High Fan-in Utility"
	ArchPage       = "Page/Route/Endpoint"
	ArchAuth       = "Auth/Security"
	ArchLeaf       = "Leaf Component"
	ArchConfig     = "Config/Constants"
	ArchBarrel     = "Barrel/Index"
	ArchTest       = "Test File"

	// Extended archetypes (new).
	ArchDBMigration    = "Database/Migration"
	ArchValidation     = "Serialization/Validation"
	ArchErrorHandling  = "Error Handling"
	ArchCLICommand     = "CLI/Command"
	ArchEventMessage   = "Event/Message"
	ArchInterface      = "Interface/Contract"
	ArchOrchestrator   = "Orchestrator"
	ArchHotspot        = "Hotspot"
)

// ArchetypeCandidate represents a detected file for an archetype slot.
type ArchetypeCandidate struct {
	Archetype string `json:"archetype"`
	File      string `json:"file"`
	Indegree  int    `json:"indegree"`
	Outdegree int    `json:"outdegree"`
	Role      string `json:"role,omitempty"`
}

// AllArchetypeNames returns all available archetype category names in detection order.
func AllArchetypeNames() []string {
	names := make([]string, len(archetypeRegistry))
	for i, d := range archetypeRegistry {
		names[i] = d.name
	}
	return names
}

// archetypeDetector defines a named archetype detection strategy.
type archetypeDetector struct {
	name   string
	detect func(ctx *archetypeContext) *ArchetypeCandidate
}

// archetypeContext holds shared state across archetype detection.
type archetypeContext struct {
	idx          *db.Index
	used         map[string]bool
	stableThresh int
	candidates   []ArchetypeCandidate // accumulated results (needed by Test File)
}

// archetypeRegistry defines all archetype detectors in detection order.
// Order matters: earlier detectors claim files first (via the used map).
var archetypeRegistry = []archetypeDetector{
	// Core archetypes (original 10).
	{ArchService, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickBestMatch(ctx, ArchService,
			[]string{"service", "controller", "handler", "repository", "store", "provider", "dao"},
			func(_ string, s db.SignalRow) int { return s.Outdegree })
	}},
	{ArchModel, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickBestMatch(ctx, ArchModel,
			[]string{"model", "schema", "types", "entity", "dto", "interface", "record"},
			func(_ string, s db.SignalRow) int { return s.Indegree })
	}},
	{ArchMiddleware, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickBestMatch(ctx, ArchMiddleware,
			[]string{"middleware", "plugin", "interceptor", "hook", "filter", "guard", "pipe", "decorator"},
			func(_ string, s db.SignalRow) int { return s.Indegree + s.Outdegree })
	}},
	{ArchHighFanIn, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickHighFanIn(ctx)
	}},
	{ArchPage, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickBestMatch(ctx, ArchPage,
			[]string{"page", "route", "endpoint", "api", "view", "screen"},
			func(_ string, s db.SignalRow) int {
				score := s.Outdegree
				if s.IsEntrypoint {
					score += 100
				}
				return score
			})
	}},
	{ArchAuth, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickBestMatch(ctx, ArchAuth,
			[]string{"auth", "security", "session", "token", "permission"},
			func(_ string, s db.SignalRow) int { return s.Indegree + s.Outdegree })
	}},
	{ArchLeaf, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickLeaf(ctx)
	}},
	{ArchConfig, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickBestMatch(ctx, ArchConfig,
			[]string{"config", "constants", "settings", "env", "options"},
			func(_ string, s db.SignalRow) int { return s.Indegree })
	}},
	{ArchBarrel, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickBarrel(ctx)
	}},
	{ArchTest, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickTestFile(ctx)
	}},

	// Extended archetypes (new categories).
	{ArchDBMigration, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickBestMatch(ctx, ArchDBMigration,
			[]string{"migration", "migrate", "seed", "fixture"},
			func(_ string, s db.SignalRow) int { return s.Outdegree })
	}},
	{ArchValidation, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickBestMatch(ctx, ArchValidation,
			[]string{"serializer", "validator", "transformer", "mapper", "converter"},
			func(_ string, s db.SignalRow) int { return s.Indegree + s.Outdegree })
	}},
	{ArchErrorHandling, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickBestMatch(ctx, ArchErrorHandling,
			[]string{"error", "exception", "fault"},
			func(_ string, s db.SignalRow) int { return s.Indegree })
	}},
	{ArchCLICommand, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickBestMatch(ctx, ArchCLICommand,
			[]string{"cmd", "command", "cli"},
			func(_ string, s db.SignalRow) int { return s.Outdegree })
	}},
	{ArchEventMessage, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickBestMatch(ctx, ArchEventMessage,
			[]string{"event", "listener", "subscriber", "consumer", "producer", "emitter"},
			func(_ string, s db.SignalRow) int { return s.Indegree + s.Outdegree })
	}},
	{ArchInterface, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickBestMatch(ctx, ArchInterface,
			[]string{"interface", "abstract", "trait", "protocol", "contract"},
			func(_ string, s db.SignalRow) int { return s.Indegree })
	}},
	{ArchOrchestrator, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickOrchestrator(ctx)
	}},
	{ArchHotspot, func(ctx *archetypeContext) *ArchetypeCandidate {
		return pickHotspot(ctx)
	}},
}

// DetectArchetypes selects representative files across architectural layers.
// If categories is nil or empty, all available archetypes are detected.
// If categories contains specific names, only those are detected.
// Unknown category names are silently skipped.
func DetectArchetypes(idx *db.Index, categories []string) ([]ArchetypeCandidate, error) {
	maxIndegree, _ := idx.GetMaxIndegree()
	stableThresh := maxIndegree / 2
	if stableThresh < 5 {
		stableThresh = 5
	}

	ctx := &archetypeContext{
		idx:          idx,
		used:         map[string]bool{},
		stableThresh: stableThresh,
	}

	// Build filter set (nil = run all).
	var filter map[string]bool
	if len(categories) > 0 {
		filter = make(map[string]bool, len(categories))
		for _, c := range categories {
			filter[c] = true
		}
	}

	for _, detector := range archetypeRegistry {
		if filter != nil && !filter[detector.name] {
			continue
		}
		c := detector.detect(ctx)
		if c != nil {
			ctx.candidates = append(ctx.candidates, *c)
			ctx.used[c.File] = true
		}
	}

	return ctx.candidates, nil
}

// pickBestMatch searches for files matching query patterns, scores them, and returns the best.
func pickBestMatch(ctx *archetypeContext, archetype string, queries []string, scorer func(path string, sig db.SignalRow) int) *ArchetypeCandidate {
	var allResults []db.SearchResult
	for _, q := range queries {
		results, err := ctx.idx.SearchByPath(q, 20)
		if err != nil {
			continue
		}
		allResults = append(allResults, results...)
	}
	if len(allResults) == 0 {
		return nil
	}

	// Deduplicate and filter.
	paths := make([]string, 0, len(allResults))
	seen := map[string]bool{}
	for _, r := range allResults {
		if !seen[r.Path] && !ctx.used[r.Path] && !classify.IsTestFile(r.Path) {
			seen[r.Path] = true
			paths = append(paths, r.Path)
		}
	}
	if len(paths) == 0 {
		return nil
	}

	signals, err := ctx.idx.GetSignals(paths)
	if err != nil {
		return nil
	}

	// Score and sort.
	type scored struct {
		path  string
		sig   db.SignalRow
		score int
	}
	var scoredPaths []scored
	for _, p := range paths {
		sig := signals[p]
		s := scorer(p, sig)
		scoredPaths = append(scoredPaths, scored{p, sig, s})
	}
	sort.Slice(scoredPaths, func(i, j int) bool {
		return scoredPaths[i].score > scoredPaths[j].score
	})

	best := scoredPaths[0]
	role := classify.ClassifyRole(best.path, best.sig.Indegree, best.sig.Outdegree,
		best.sig.IsEntrypoint, best.sig.IsUtility, ctx.stableThresh)
	return &ArchetypeCandidate{
		Archetype: archetype,
		File:      best.path,
		Indegree:  best.sig.Indegree,
		Outdegree: best.sig.Outdegree,
		Role:      role,
	}
}

func pickHighFanIn(ctx *archetypeContext) *ArchetypeCandidate {
	foundations, err := ctx.idx.GetFoundations(50)
	if err != nil {
		return nil
	}
	for _, f := range foundations {
		if ctx.used[f.Path] || classify.IsTestFile(f.Path) {
			continue
		}
		role := classify.ClassifyRole(f.Path, f.Indegree, f.Outdegree, false, false, ctx.stableThresh)
		return &ArchetypeCandidate{
			Archetype: ArchHighFanIn,
			File:      f.Path,
			Indegree:  f.Indegree,
			Outdegree: f.Outdegree,
			Role:      role,
		}
	}
	return nil
}

func pickLeaf(ctx *archetypeContext) *ArchetypeCandidate {
	for _, q := range []string{"util", "helper", "component", "widget", "formatter", "converter", "adapter"} {
		results, err := ctx.idx.SearchByPath(q, 30)
		if err != nil {
			continue
		}
		paths := make([]string, 0, len(results))
		for _, r := range results {
			if !ctx.used[r.Path] && !classify.IsTestFile(r.Path) {
				paths = append(paths, r.Path)
			}
		}
		if len(paths) == 0 {
			continue
		}
		signals, err := ctx.idx.GetSignals(paths)
		if err != nil {
			continue
		}
		for _, p := range paths {
			s := signals[p]
			if s.Indegree <= 1 && s.Outdegree <= 2 {
				role := classify.ClassifyRole(p, s.Indegree, s.Outdegree, s.IsEntrypoint, s.IsUtility, ctx.stableThresh)
				return &ArchetypeCandidate{
					Archetype: ArchLeaf,
					File:      p,
					Indegree:  s.Indegree,
					Outdegree: s.Outdegree,
					Role:      role,
				}
			}
		}
	}
	return nil
}

func pickBarrel(ctx *archetypeContext) *ArchetypeCandidate {
	barrelNames := []string{"index.ts", "index.js", "__init__.py", "mod.rs", "lib.rs"}
	for _, name := range barrelNames {
		results, err := ctx.idx.SearchByPath(name, 20)
		if err != nil {
			continue
		}
		paths := make([]string, 0, len(results))
		for _, r := range results {
			if !ctx.used[r.Path] && classify.IsBarrelFile(r.Path) {
				paths = append(paths, r.Path)
			}
		}
		if len(paths) == 0 {
			// Also check mod.rs/lib.rs which aren't classified as barrel.
			for _, r := range results {
				base := classify.PathBase(r.Path)
				if !ctx.used[r.Path] && (base == "mod.rs" || base == "lib.rs") {
					paths = append(paths, r.Path)
				}
			}
		}
		if len(paths) == 0 {
			continue
		}
		signals, err := ctx.idx.GetSignals(paths)
		if err != nil {
			continue
		}
		best := ""
		bestIndegree := -1
		for _, p := range paths {
			s := signals[p]
			if s.Indegree > bestIndegree {
				best = p
				bestIndegree = s.Indegree
			}
		}
		if best != "" {
			s := signals[best]
			role := classify.ClassifyRole(best, s.Indegree, s.Outdegree, s.IsEntrypoint, s.IsUtility, ctx.stableThresh)
			return &ArchetypeCandidate{
				Archetype: ArchBarrel,
				File:      best,
				Indegree:  s.Indegree,
				Outdegree: s.Outdegree,
				Role:      role,
			}
		}
	}
	return nil
}

func pickTestFile(ctx *archetypeContext) *ArchetypeCandidate {
	testQueries := []string{"test", "spec"}
	for _, c := range ctx.candidates {
		base := classify.PathBase(c.File)
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		testQueries = append(testQueries, stem+"_test", "test_"+stem, stem+".test", stem+".spec")
	}

	for _, q := range testQueries {
		results, err := ctx.idx.SearchByPath(q, 20)
		if err != nil {
			continue
		}
		for _, r := range results {
			if !ctx.used[r.Path] && classify.IsTestFile(r.Path) {
				signals, _ := ctx.idx.GetSignals([]string{r.Path})
				s := signals[r.Path]
				return &ArchetypeCandidate{
					Archetype: ArchTest,
					File:      r.Path,
					Indegree:  s.Indegree,
					Outdegree: s.Outdegree,
					Role:      "test",
				}
			}
		}
	}
	return nil
}

// pickOrchestrator finds a file with high connectivity in both directions
// (high indegree AND high outdegree), indicating a structural connector.
func pickOrchestrator(ctx *archetypeContext) *ArchetypeCandidate {
	results, err := ctx.idx.GetOrchestrators(5, 5, 20, ctx.used)
	if err != nil {
		return nil
	}
	for _, f := range results {
		if ctx.used[f.Path] || classify.IsTestFile(f.Path) {
			continue
		}
		role := classify.ClassifyRole(f.Path, f.Indegree, f.Outdegree, false, false, ctx.stableThresh)
		return &ArchetypeCandidate{
			Archetype: ArchOrchestrator,
			File:      f.Path,
			Indegree:  f.Indegree,
			Outdegree: f.Outdegree,
			Role:      role,
		}
	}
	return nil
}

// pickHotspot finds the highest-churn file that hasn't been claimed by another archetype.
func pickHotspot(ctx *archetypeContext) *ArchetypeCandidate {
	path, _, err := ctx.idx.GetHighestChurnFile(90, ctx.used)
	if err != nil || path == "" {
		return nil
	}
	signals, _ := ctx.idx.GetSignals([]string{path})
	s := signals[path]
	role := classify.ClassifyRole(path, s.Indegree, s.Outdegree, s.IsEntrypoint, s.IsUtility, ctx.stableThresh)
	return &ArchetypeCandidate{
		Archetype: ArchHotspot,
		File:      path,
		Indegree:  s.Indegree,
		Outdegree: s.Outdegree,
		Role:      role,
	}
}
