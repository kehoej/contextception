# AI Agent Guidelines

When working on this codebase:

## Getting Oriented

1. Read `CLAUDE.md` for project overview and tech stack
2. Read `docs/ARCHITECTURE.md` for the full architecture with diagrams
3. Run `contextception status` to check index health

## Development Commands

```bash
make build      # Build binary
make test       # Run all tests (includes race detector in CI)
make lint       # Run golangci-lint
make check      # Run vet + lint + test in one command
make coverage   # Generate HTML coverage report
```

## Code Conventions

- Standard Go conventions: `gofmt`, `go vet`, `golangci-lint`
- Error handling: wrap errors with `fmt.Errorf("context: %w", err)`
- No `panic()` — always return errors
- Parameterized SQL queries only (never string concatenation)
- Test files go alongside source: `foo_test.go` next to `foo.go`
- Integration test fixtures go in `testdata/`

## Architecture Quick Reference

```
internal/extractor/   Language-specific import extraction (Python, TS, Go, Java, Rust)
internal/resolver/    Module specifier → file path resolution (per-language)
internal/indexer/     Scan → extract → resolve → store pipeline
internal/analyzer/    Dependency graph traversal, scoring, categorization
internal/change/      PR/branch diff impact analysis
internal/db/          SQLite storage (migrations in internal/db/migrations/)
internal/mcpserver/   MCP server with 8 tools
```

## Adding a New Language

1. Create extractor in `internal/extractor/<lang>/` implementing `extractor.Extractor`
   - `Extensions()` must return the file extensions (e.g., `[]string{".cs"}`)
   - This is enforced by the interface. The `contextception extensions` command automatically picks up new extensions.
2. Create resolver in `internal/resolver/<lang>/` implementing `resolver.Resolver`
3. Register both in `internal/indexer/indexer.go` (instantiate extractor + resolver in `NewIndexer`)
   - Also add the extractor to `internal/cli/extensions.go` so `contextception extensions` includes it
4. Add test fixtures in `testdata/`
5. Verify: `contextception extensions` should list your new file extensions
6. See existing implementations (e.g., `internal/extractor/python/`) for reference

## Testing

- `make test` before any commit
- Validation tests in `internal/validation/` run fixtures against expected output
- Some tests clone external repos (Flask, httpx) — skippable via `SKIP_FLASK_VALIDATION=1`
- Benchmark reproduction: `benchmarks/reproduce.sh`
