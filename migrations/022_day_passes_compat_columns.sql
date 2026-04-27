-- Ensure legacy and newer code paths share the same day_passes columns.
-- Safe for repeated execution on existing deployments.

ALTER TABLE day_passes
  ADD COLUMN IF NOT EXISTS is_expired BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE day_passes
  ADD COLUMN IF NOT EXISTS created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW();

ALTER TABLE day_passes
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW();

