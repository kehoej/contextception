# Contributing to Contextception

Thank you for your interest in contributing to Contextception!

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/contextception.git`
3. Create a branch: `git checkout -b my-feature`
4. Make your changes
5. Run tests: `make test`
6. Run linter: `make lint`
7. Commit and push
8. Open a pull request

## Prerequisites

- **Go 1.24+** — required for building
- **GCC** — required for CGO (tree-sitter TypeScript/JavaScript extraction)
- **golangci-lint** — for linting (optional, CI runs it automatically)

## Building

```bash
make build    # Binary at ./bin/contextception
make test     # Run all tests
make lint     # Run golangci-lint
```

## Project Structure

```
cmd/contextception/    CLI entrypoint
internal/
  analyzer/            Core analysis engine
  change/              PR/branch diff analysis
  cli/                 Command handlers
  config/              Configuration parsing
  db/                  SQLite database layer
  extractor/           Language-specific extractors (python, typescript, golang, java, rust)
  git/                 Git history signals
  indexer/             Incremental indexing
  resolver/            Module resolution (per-language)
```

## Adding a New Language

Language support is pluggable. To add a new language:

1. Create an extractor in `internal/extractor/<language>/`
2. Implement the `extractor.Extractor` interface
3. Create a resolver in `internal/resolver/<language>/` if needed
4. Register both in `internal/indexer/indexer.go` (instantiate extractor + resolver in `NewIndexer`)
5. Add the extractor to `internal/cli/extensions.go` so `contextception extensions` includes it
6. Add test fixtures in `testdata/`

See existing extractors (e.g., `internal/extractor/python/`) for reference.

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Error handling: wrap errors with `fmt.Errorf("context: %w", err)`
- No `panic()` — return errors instead
- Use parameterized SQL queries (never string concatenation)

## Testing

- Write tests for new features
- Run `make test` before submitting
- Test files go alongside source files (`foo_test.go` next to `foo.go`)
- Integration test fixtures go in `testdata/`

## Pull Requests

- Keep PRs focused — one feature or fix per PR
- Include a clear description of what changed and why
- Ensure CI passes (tests + lint)

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
