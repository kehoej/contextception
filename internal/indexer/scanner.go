package indexer

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ScannedFile represents a discovered source file with its metadata.
type ScannedFile struct {
	Path         string // repo-relative path
	AbsPath      string // absolute path
	ContentHash  string // SHA-256 hex digest
	Language     string // detected language
	LastModified int64  // Unix timestamp
	SizeBytes    int64
}

// ignoredDirs lists directory names that should be skipped during scanning
// and change detection. Used by both Scanner and isIgnoredPath.
var ignoredDirs = map[string]bool{
	".git":             true,
	".hg":              true,
	".svn":             true,
	"__pycache__":      true,
	"node_modules":     true,
	".venv":            true,
	"venv":             true,
	".env":             true,
	"env":              true,
	".tox":             true,
	".mypy_cache":      true,
	".pytest_cache":    true,
	".contextception":  true,
	"dist":             true,
	"build":            true,
	".eggs":            true,
	"*.egg-info":       true,
}

// isIgnoredPath returns true if any component of the path matches an ignored directory.
func isIgnoredPath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if ignoredDirs[part] {
			return true
		}
		for pattern := range ignoredDirs {
			if strings.Contains(pattern, "*") {
				if matched, _ := filepath.Match(pattern, part); matched {
					return true
				}
			}
		}
	}
	return false
}

// Scanner discovers and hashes source files in a repository.
type Scanner struct {
	repoRoot             string
	extMap               map[string]string // extension → language
	ignoreDirs           map[string]bool   // directory names to skip
	configIgnorePrefixes []string          // config-based path prefixes to skip
}

// NewScanner creates a Scanner for the given repository root.
// configIgnore is a list of path prefixes (with trailing slash) from config.
func NewScanner(repoRoot string, configIgnore []string) *Scanner {
	return &Scanner{
		repoRoot: repoRoot,
		extMap: map[string]string{
			".py":   "python",
			".ts":   "typescript",
			".tsx":  "typescript",
			".mts":  "typescript",
			".cts":  "typescript",
			".js":   "javascript",
			".jsx":  "javascript",
			".mjs":  "javascript",
			".cjs":  "javascript",
			".go":   "go",
			".java": "java",
			".rs":   "rust",
		},
		ignoreDirs:           ignoredDirs,
		configIgnorePrefixes: configIgnore,
	}
}

// ScanAll walks the entire repository and returns all recognized source files.
func (s *Scanner) ScanAll() ([]ScannedFile, error) {
	var files []ScannedFile

	err := filepath.WalkDir(s.repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		if d.IsDir() {
			name := d.Name()
			// Skip hidden directories and ignored directories.
			if strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			if s.ignoreDirs[name] {
				return filepath.SkipDir
			}
			// Check for wildcard patterns like *.egg-info.
			for pattern := range s.ignoreDirs {
				if strings.Contains(pattern, "*") {
					if matched, _ := filepath.Match(pattern, name); matched {
						return filepath.SkipDir
					}
				}
			}
			return nil
		}

		ext := filepath.Ext(path)
		lang, ok := s.extMap[ext]
		if !ok {
			return nil // not a recognized file type
		}

		// Check config ignore prefixes on repo-relative path.
		if len(s.configIgnorePrefixes) > 0 {
			relPath, relErr := filepath.Rel(s.repoRoot, path)
			if relErr == nil {
				relPath = filepath.ToSlash(relPath)
				for _, prefix := range s.configIgnorePrefixes {
					if strings.HasPrefix(relPath, prefix) {
						return nil
					}
				}
			}
		}

		scanned, err := s.scanFile(path, lang)
		if err != nil {
			return nil // skip files that can't be read
		}

		files = append(files, scanned)
		return nil
	})

	return files, err
}

// ScanPaths scans specific files and returns their metadata.
func (s *Scanner) ScanPaths(paths []string) ([]ScannedFile, error) {
	var files []ScannedFile

	for _, relPath := range paths {
		absPath := filepath.Join(s.repoRoot, relPath)
		ext := filepath.Ext(relPath)
		lang, ok := s.extMap[ext]
		if !ok {
			continue
		}

		scanned, err := s.scanFile(absPath, lang)
		if err != nil {
			continue // skip unreadable files
		}
		files = append(files, scanned)
	}

	return files, nil
}

func (s *Scanner) scanFile(absPath, lang string) (ScannedFile, error) {
	info, err := os.Stat(absPath)
	if err != nil {
		return ScannedFile{}, err
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return ScannedFile{}, err
	}

	hash := sha256.Sum256(content)
	relPath, err := filepath.Rel(s.repoRoot, absPath)
	if err != nil {
		return ScannedFile{}, err
	}

	return ScannedFile{
		Path:         relPath,
		AbsPath:      absPath,
		ContentHash:  fmt.Sprintf("%x", hash),
		Language:     lang,
		LastModified: info.ModTime().Unix(),
		SizeBytes:    info.Size(),
	}, nil
}
