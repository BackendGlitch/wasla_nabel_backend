-- Recovery Script for Deleted Worker Account
-- This script will help restore a deleted worker account and their records
-- 
-- IMPORTANT: Before running this script, you need to provide:
-- 1. The staff ID that was deleted (we know it's: staff_1764529704354997788)
-- 2. The worker's CIN, first_name, last_name, phone_number, and role
--
-- Step 1: Create a backup of current state (already done - see backup files)
-- Step 2: Restore the staff account
-- Step 3: Update all orphaned bookings to point back to this staff member

-- ============================================
-- STEP 1: Verify the deleted staff ID
-- ============================================
-- Check if staff still exists (should return 0 rows)
SELECT 'Checking if staff exists...' as step;
SELECT id, cin, first_name, last_name, role 
FROM staff 
WHERE id = 'staff_1764529704354997788';

-- ============================================
-- STEP 2: Check orphaned bookings count
-- ============================================
SELECT 'Checking orphaned bookings...' as step;
SELECT COUNT(*) as orphaned_bookings_count
FROM bookings 
WHERE created_by IS NULL;

-- ============================================
-- STEP 3: RESTORE STAFF ACCOUNT
-- ============================================
-- NOTE: You need to fill in the actual values for the worker
-- Replace the placeholders below with the actual worker information

/*
INSERT INTO staff (
    id,
    cin,
    phone_number,
    first_name,
    last_name,
    role,
    is_active,
    created_at,
    updated_at
) VALUES (
    'staff_1764529704354997788',  -- The deleted staff ID
    'XXXXXXXX',                    -- CIN (8 digits)
    '+216XXXXXXXXX',              -- Phone number (optional)
    'FIRST_NAME',                 -- First name
    'LAST_NAME',                  -- Last name
    'WORKER',                     -- Role: 'WORKER' or 'SUPERVISOR'
    true,                         -- is_active
    (SELECT MIN(created_at) FROM bookings WHERE created_by IS NULL), -- Use first booking date
    NOW()                         -- updated_at
) ON CONFLICT (id) DO NOTHING;
*/

-- ============================================
-- STEP 4: UPDATE ORPHANED BOOKINGS
-- ============================================
-- After restoring the staff account, update all bookings that have NULL created_by
-- This assumes all NULL created_by bookings belong to the deleted worker
-- If you know a more specific criteria, adjust the WHERE clause

/*
UPDATE bookings
SET created_by = 'staff_1764529704354997788',
    updated_at = NOW()
WHERE created_by IS NULL
  AND created_at >= (SELECT MIN(created_at) FROM bookings WHERE created_by IS NULL)
  AND created_at <= '2026-02-01 19:05:49';  -- Deletion timestamp from logs
*/

-- ============================================
-- STEP 5: VERIFY RECOVERY
-- ============================================
-- After running the INSERT and UPDATE, verify the recovery:

/*
SELECT 
    s.id,
    s.cin,
    s.first_name,
    s.last_name,
    s.role,
    COUNT(b.id) as total_bookings_restored
FROM staff s
LEFT JOIN bookings b ON b.created_by = s.id
WHERE s.id = 'staff_1764529704354997788'
GROUP BY s.id, s.cin, s.first_name, s.last_name, s.role;
*/

-- ============================================
-- ALTERNATIVE: If you have a backup with the staff data
-- ============================================
-- If you have a backup that contains the staff member, you can extract it:
-- pg_restore -t staff backup_file.dump | grep staff_1764529704354997788

