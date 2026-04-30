package cli

import (
	"fmt"
	"sort"

	"github.com/kehoej/contextception/internal/extractor"
	csharpextractor "github.com/kehoej/contextception/internal/extractor/csharp"
	goextractor "github.com/kehoej/contextception/internal/extractor/golang"
	javaextractor "github.com/kehoej/contextception/internal/extractor/java"
	pyextractor "github.com/kehoej/contextception/internal/extractor/python"
	rustextractor "github.com/kehoej/contextception/internal/extractor/rust"
	tsextractor "github.com/kehoej/contextception/internal/extractor/typescript"
	"github.com/spf13/cobra"
)

func newExtensionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "extensions",
		Short: "Print supported file extensions",
		Long:  "Print file extensions supported by contextception, one per line. Useful for tooling integration.",
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, ext := range supportedExtensions() {
				fmt.Println(ext)
			}
			return nil
		},
	}
}

// supportedExtensions returns the sorted list of file extensions supported by
// all language extractors. Used by the `extensions` CLI command and by
// session/discover commands that filter to indexable files.
func supportedExtensions() []string {
	extractors := []extractor.Extractor{
		pyextractor.New(),
		tsextractor.New(),
		goextractor.New(),
		javaextractor.New(),
		rustextractor.New(),
		csharpextractor.New(),
	}
	var exts []string
	for _, ext := range extractors {
		exts = append(exts, ext.Extensions()...)
	}
	sort.Strings(exts)
	return exts
}

func isSupportedExtension(ext string) bool {
	for _, supported := range supportedExtensions() {
		if ext == supported {
			return true
		}
	}
	return false
}
