#!/bin/bash

# Recovery Script for Deleted Worker Account
# This script helps restore a deleted worker and their records

set -e

DB_USER="ivan"
DB_NAME="main-ste"
DB_HOST="localhost"
STAFF_ID="staff_1764529704354997788"
BACKUP_DIR="/home/ste/wasla_backend"

echo "=========================================="
echo "Worker Account Recovery Script"
echo "=========================================="
echo ""
echo "Deleted Staff ID: $STAFF_ID"
echo "Deletion Date: 2026-02-01 19:05:49"
echo ""

# Export password
export PGPASSWORD='Lost2409'

# Step 1: Verify current state
echo "Step 1: Checking current database state..."
echo "----------------------------------------"
ORPHANED_COUNT=$(psql -U $DB_USER -h $DB_HOST -d $DB_NAME -t -c "SELECT COUNT(*) FROM bookings WHERE created_by IS NULL;")
echo "Orphaned bookings (NULL created_by): $ORPHANED_COUNT"

STAFF_EXISTS=$(psql -U $DB_USER -h $DB_HOST -d $DB_NAME -t -c "SELECT COUNT(*) FROM staff WHERE id = '$STAFF_ID';")
if [ "$STAFF_EXISTS" -eq 0 ]; then
    echo "✓ Staff account does not exist (as expected)"
else
    echo "⚠ Staff account still exists!"
fi

echo ""
echo "Step 2: Checking for backup files..."
echo "----------------------------------------"
if [ -f "$BACKUP_DIR/backup_20260202_012947.sql" ]; then
    echo "✓ Current backup found: backup_20260202_012947.sql"
else
    echo "⚠ Current backup not found"
fi

if [ -f "$BACKUP_DIR/louaj_node_backup.sql" ]; then
    echo "✓ Old backup found: louaj_node_backup.sql"
else
    echo "⚠ Old backup not found"
fi

echo ""
echo "Step 3: Attempting to extract staff info from old backup..."
echo "----------------------------------------"
if [ -f "$BACKUP_DIR/louaj_node_backup.sql" ]; then
    # Try to find the staff member in the old backup
    grep -i "staff_1764529704354997788" "$BACKUP_DIR/louaj_node_backup.sql" || echo "Staff member not found in old backup"
fi

echo ""
echo "=========================================="
echo "RECOVERY INSTRUCTIONS:"
echo "=========================================="
echo ""
echo "To recover the worker account, you need to:"
echo ""
echo "1. Get the worker's information:"
echo "   - CIN (8 digits)"
echo "   - First Name"
echo "   - Last Name"
echo "   - Phone Number (optional)"
echo "   - Role (WORKER or SUPERVISOR)"
echo ""
echo "2. Run the SQL recovery script:"
echo "   psql -U $DB_USER -h $DB_HOST -d $DB_NAME -f recover_deleted_worker.sql"
echo ""
echo "3. Or use this interactive recovery:"
echo ""
read -p "Do you want to proceed with interactive recovery? (yes/no): " proceed

if [ "$proceed" != "yes" ]; then
    echo "Recovery cancelled. You can run this script again when ready."
    exit 0
fi

echo ""
echo "Please provide the worker's information:"
read -p "CIN (8 digits): " cin
read -p "First Name: " first_name
read -p "Last Name: " last_name
read -p "Phone Number (optional, press Enter to skip): " phone_number
read -p "Role (WORKER/SUPERVISOR) [default: WORKER]: " role

if [ -z "$role" ]; then
    role="WORKER"
fi

if [ -z "$phone_number" ]; then
    phone_number="NULL"
else
    phone_number="'$phone_number'"
fi

# Validate CIN
if [ ${#cin} -ne 8 ]; then
    echo "Error: CIN must be 8 digits"
    exit 1
fi

echo ""
echo "=========================================="
echo "RECOVERY SUMMARY:"
echo "=========================================="
echo "Staff ID: $STAFF_ID"
echo "CIN: $cin"
echo "Name: $first_name $last_name"
echo "Phone: $phone_number"
echo "Role: $role"
echo "Orphaned Bookings: $ORPHANED_COUNT"
echo ""
read -p "Confirm recovery? (yes/no): " confirm

if [ "$confirm" != "yes" ]; then
    echo "Recovery cancelled."
    exit 0
fi

echo ""
echo "Step 4: Restoring staff account..."
echo "----------------------------------------"

# Get the first booking date for created_at
FIRST_BOOKING_DATE=$(psql -U $DB_USER -h $DB_HOST -d $DB_NAME -t -c "SELECT MIN(created_at) FROM bookings WHERE created_by IS NULL;")

# Insert the staff account
psql -U $DB_USER -h $DB_HOST -d $DB_NAME <<EOF
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
    '$STAFF_ID',
    '$cin',
    $phone_number,
    '$first_name',
    '$last_name',
    '$role',
    true,
    '$FIRST_BOOKING_DATE',
    NOW()
) ON CONFLICT (id) DO UPDATE SET
    cin = EXCLUDED.cin,
    phone_number = EXCLUDED.phone_number,
    first_name = EXCLUDED.first_name,
    last_name = EXCLUDED.last_name,
    role = EXCLUDED.role,
    is_active = true,
    updated_at = NOW();
EOF

if [ $? -eq 0 ]; then
    echo "✓ Staff account restored successfully"
else
    echo "✗ Failed to restore staff account"
    exit 1
fi

echo ""
echo "Step 5: Restoring bookings..."
echo "----------------------------------------"

# Update orphaned bookings
psql -U $DB_USER -h $DB_HOST -d $DB_NAME <<EOF
UPDATE bookings
SET created_by = '$STAFF_ID',
    updated_at = NOW()
WHERE created_by IS NULL
  AND created_at >= '$FIRST_BOOKING_DATE'
  AND created_at <= '2026-02-01 19:05:49';
EOF

if [ $? -eq 0 ]; then
    echo "✓ Bookings restored successfully"
else
    echo "✗ Failed to restore bookings"
    exit 1
fi

echo ""
echo "Step 6: Verifying recovery..."
echo "----------------------------------------"

psql -U $DB_USER -h $DB_HOST -d $DB_NAME <<EOF
SELECT 
    s.id,
    s.cin,
    s.first_name,
    s.last_name,
    s.role,
    COUNT(b.id) as total_bookings_restored
FROM staff s
LEFT JOIN bookings b ON b.created_by = s.id
WHERE s.id = '$STAFF_ID'
GROUP BY s.id, s.cin, s.first_name, s.last_name, s.role;
EOF

echo ""
echo "=========================================="
echo "RECOVERY COMPLETE!"
echo "=========================================="
echo ""
echo "The worker account and their bookings have been restored."
echo "Please verify the data and test the system."

