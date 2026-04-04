// Package resolver defines the interface for mapping imports to repository files.
package resolver

import (
	"os"

	"github.com/kehoej/contextception/internal/model"
)

// Resolver maps import specifiers to repository files.
type Resolver interface {
	// Resolve attempts to map an import to a repo-relative file path.
	Resolve(srcFile string, fact model.ImportFact, repoRoot string) (model.ResolveResult, error)
}

// MultiResolver extends Resolver for languages where one import resolves to
// multiple files (e.g., Go imports target packages/directories, not files).
type MultiResolver interface {
	Resolver
	// ResolveAll returns all files that a single import resolves to.
	ResolveAll(srcFile string, fact model.ImportFact, repoRoot string) ([]model.ResolveResult, error)
}

// SamePackageResolver extends Resolver for languages where files in the same
// directory/package implicitly depend on each other (e.g., Go same-package visibility).
type SamePackageResolver interface {
	Resolver
	// ResolveSamePackageEdges returns sibling files in the same package as srcFile.
	ResolveSamePackageEdges(srcFile, repoRoot string) []model.ResolveResult
}

// FileExists returns true if path exists and is a regular file (not a directory).
func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// DirExists returns true if path exists and is a directory.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
