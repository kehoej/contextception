package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/kehoej/contextception/internal/classify"
	"github.com/kehoej/contextception/internal/db"
	"github.com/spf13/cobra"
)

const (
	defaultSearchLimit = 50
	maxSearchLimit     = 100
)

var (
	searchType  string
	searchLimit int
	searchJSON  bool
)

func newSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search the index for files by path or symbol",
		Long: `Search the index for files by path substring or exported symbol name.

Examples:
  contextception search auth
  contextception search --type symbol AuthService
  contextception search --limit 10 --json auth`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(args[0])
		},
	}

	cmd.Flags().StringVar(&searchType, "type", "path", "search type: path or symbol")
	cmd.Flags().IntVar(&searchLimit, "limit", 50, "max results to return")
	cmd.Flags().BoolVar(&searchJSON, "json", false, "output as JSON")

	return cmd
}

type searchResultJSON struct {
	Query   string             `json:"query"`
	Type    string             `json:"type"`
	Results []searchEntryJSON  `json:"results"`
	Count   int                `json:"count"`
}

type searchEntryJSON struct {
	File      string `json:"file"`
	Language  string `json:"language"`
	SizeBytes int64  `json:"size_bytes"`
	Indegree  int    `json:"indegree"`
	Outdegree int    `json:"outdegree"`
	Role      string `json:"role,omitempty"`
}

func runSearch(query string) error {
	if searchType != "path" && searchType != "symbol" {
		return fmt.Errorf("type must be 'path' or 'symbol'")
	}
	if searchLimit <= 0 {
		searchLimit = defaultSearchLimit
	}
	if searchLimit > maxSearchLimit {
		searchLimit = maxSearchLimit
	}

	idxPath := db.IndexPath(repoRoot)
	if _, err := os.Stat(idxPath); os.IsNotExist(err) {
		fmt.Println("No index found. Run `contextception index` to create one.")
		return nil
	}

	idx, err := db.OpenIndex(idxPath)
	if err != nil {
		return fmt.Errorf("opening index: %w", err)
	}
	defer idx.Close()

	var results []db.SearchResult
	if searchType == "path" {
		results, err = idx.SearchByPath(query, searchLimit)
	} else {
		results, err = idx.SearchBySymbol(query, searchLimit)
	}
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	// Enrich with signals for JSON output or role display.
	paths := make([]string, len(results))
	for i, r := range results {
		paths[i] = r.Path
	}
	signals, _ := idx.GetSignals(paths)
	if signals == nil {
		signals = map[string]db.SignalRow{}
	}

	maxIndegree, _ := idx.GetMaxIndegree()
	stableThresh := maxIndegree / 2
	if stableThresh < 5 {
		stableThresh = 5
	}

	if searchJSON {
		entries := make([]searchEntryJSON, len(results))
		for i, r := range results {
			sig := signals[r.Path]
			role := classify.ClassifyRole(r.Path, sig.Indegree, sig.Outdegree, sig.IsEntrypoint, sig.IsUtility, stableThresh)
			entries[i] = searchEntryJSON{
				File:      r.Path,
				Language:  r.Language,
				SizeBytes: r.SizeBytes,
				Indegree:  sig.Indegree,
				Outdegree: sig.Outdegree,
				Role:      role,
			}
		}
		output := searchResultJSON{
			Query:   query,
			Type:    searchType,
			Results: entries,
			Count:   len(entries),
		}
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling result: %w", err)
		}
		fmt.Println(string(data))
	} else {
		for _, r := range results {
			fmt.Printf("%s  [%s, %d bytes]\n", r.Path, r.Language, r.SizeBytes)
		}
		fmt.Printf("\n%d result(s)\n", len(results))
	}

	return nil
}
