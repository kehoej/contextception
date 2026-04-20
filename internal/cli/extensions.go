package cli

import (
	"fmt"
	"sort"

	"github.com/kehoej/contextception/internal/extractor"
	goextractor "github.com/kehoej/contextception/internal/extractor/golang"
	javaextractor "github.com/kehoej/contextception/internal/extractor/java"
	pyextractor "github.com/kehoej/contextception/internal/extractor/python"
	rustextractor "github.com/kehoej/contextception/internal/extractor/rust"
	tsextractor "github.com/kehoej/contextception/internal/extractor/typescript"
	csharpextractor "github.com/kehoej/contextception/internal/extractor/csharp"
	"github.com/spf13/cobra"
)

func newExtensionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "extensions",
		Short: "Print supported file extensions",
		Long:  "Print file extensions supported by contextception, one per line. Useful for tooling integration.",
		RunE: func(cmd *cobra.Command, args []string) error {
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

			for _, ext := range exts {
				fmt.Println(ext)
			}
			return nil
		},
	}
}
