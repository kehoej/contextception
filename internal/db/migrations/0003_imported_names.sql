-- Add imported_names column to edges table.
-- Stores a JSON array of imported symbol names (e.g. '["User","Post"]').
-- NULL when the import is a bare module import (no specific names).
ALTER TABLE edges ADD COLUMN imported_names TEXT;
