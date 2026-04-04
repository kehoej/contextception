-- Contextception SQLite Schema (v1)
--
-- This is the concrete starting schema for the Contextception index.
-- See ADR-0002 for versioning/migration strategy.

-- Schema metadata
CREATE TABLE meta (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL
);

-- Seed with initial values
INSERT INTO meta (key, value) VALUES ('schema_version', '1');
INSERT INTO meta (key, value) VALUES ('last_indexed_at', '');
INSERT INTO meta (key, value) VALUES ('repo_root', '');

-- Indexed files
CREATE TABLE files (
    path            TEXT PRIMARY KEY,   -- repo-relative path (e.g., 'src/auth/login.py')
    content_hash    TEXT NOT NULL,       -- SHA-256 of file contents (for incremental updates)
    last_modified   INTEGER NOT NULL,    -- Unix timestamp of last modification
    language        TEXT NOT NULL,       -- detected language ('python', 'typescript', 'javascript')
    size_bytes      INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_files_language ON files(language);

-- Dependency edges between files
CREATE TABLE edges (
    src_file            TEXT NOT NULL,       -- importing file (repo-relative path)
    dst_file            TEXT NOT NULL,       -- imported file (repo-relative path)
    edge_type           TEXT NOT NULL,       -- 'import', 'reexport'
    specifier           TEXT NOT NULL,       -- original import specifier as written in source
    resolution_method   TEXT NOT NULL,       -- 'relative', 'package_local', 'tsconfig_paths', 'workspace'
    line_number         INTEGER,             -- line number of the import statement
    PRIMARY KEY (src_file, dst_file, specifier),
    FOREIGN KEY (src_file) REFERENCES files(path) ON DELETE CASCADE,
    FOREIGN KEY (dst_file) REFERENCES files(path) ON DELETE CASCADE
);

CREATE INDEX idx_edges_src ON edges(src_file);
CREATE INDEX idx_edges_dst ON edges(dst_file);

-- Precomputed structural signals per file
CREATE TABLE signals (
    file_path       TEXT PRIMARY KEY,
    indegree        INTEGER NOT NULL DEFAULT 0,     -- number of files that import this file
    outdegree       INTEGER NOT NULL DEFAULT 0,     -- number of files this file imports
    is_entrypoint   INTEGER NOT NULL DEFAULT 0,     -- 1 if detected as entrypoint, 0 otherwise
    is_utility      INTEGER NOT NULL DEFAULT 0,     -- 1 if detected as utility (high fan-out, low fan-in pattern)
    FOREIGN KEY (file_path) REFERENCES files(path) ON DELETE CASCADE
);

-- Unresolved imports (external deps or failed resolution)
CREATE TABLE unresolved (
    src_file    TEXT NOT NULL,       -- file containing the import
    specifier   TEXT NOT NULL,       -- the import specifier that couldn't be resolved
    reason      TEXT NOT NULL,       -- 'external', 'not_found', 'namespace_package', 'dynamic_import'
    line_number INTEGER,             -- line number of the import statement
    PRIMARY KEY (src_file, specifier),
    FOREIGN KEY (src_file) REFERENCES files(path) ON DELETE CASCADE
);

CREATE INDEX idx_unresolved_src ON unresolved(src_file);

-- Package roots detected in the repository
CREATE TABLE package_roots (
    path            TEXT PRIMARY KEY,   -- repo-relative path to package root directory
    detection_method TEXT NOT NULL,      -- 'pyproject_toml', 'setup_py', 'setup_cfg', 'src_layout', 'repo_root'
    language        TEXT NOT NULL        -- language this package root applies to
);
