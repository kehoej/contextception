-- Add indexes on co_change_pairs for fast partner lookups.
-- Without these, GetCoChangePartners does a full table scan on every Analyze() call,
-- which is catastrophic for large repos (PyTorch: 2+ minutes per file).
CREATE INDEX IF NOT EXISTS idx_cochange_file_a ON co_change_pairs(file_a, window_days);
CREATE INDEX IF NOT EXISTS idx_cochange_file_b ON co_change_pairs(file_b, window_days);
