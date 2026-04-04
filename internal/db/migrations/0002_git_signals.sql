-- Git signal tables (v2)
--
-- Stores churn and co-change data extracted from git history.
-- See ADR-0009 (scoring) and ADR-0010 (risk signals).

CREATE TABLE IF NOT EXISTS git_signals (
    file_path       TEXT NOT NULL,
    signal_type     TEXT NOT NULL,       -- 'churn'
    value           REAL NOT NULL,
    window_days     INTEGER NOT NULL,
    computed_at     TEXT NOT NULL,
    PRIMARY KEY (file_path, signal_type, window_days),
    FOREIGN KEY (file_path) REFERENCES files(path) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS co_change_pairs (
    file_a      TEXT NOT NULL,
    file_b      TEXT NOT NULL,
    frequency   INTEGER NOT NULL,
    window_days INTEGER NOT NULL,
    computed_at TEXT NOT NULL,
    PRIMARY KEY (file_a, file_b, window_days)
);
