-- Composite covering index for the login query:
--   SELECT ... FROM staff WHERE cin = $1 AND is_active = true
-- This replaces the separate idx_staff_cin and idx_staff_active indexes
-- with a single index that the planner can use as an index-only scan.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_staff_cin_active
  ON staff (cin, is_active)
  WHERE is_active = true;
