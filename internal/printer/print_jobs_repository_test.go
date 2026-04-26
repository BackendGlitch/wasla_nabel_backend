package printer

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func mustTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL or DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pg connect: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestPrintJobs_CreateOrGetJob_Idempotent(t *testing.T) {
	db := mustTestDB(t)
	repo := NewPrintJobsRepository(db)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	printerID := "test-printer:9100"
	key := "test-idem-123"

	// Clean previous runs
	_, _ = db.Exec(ctx, `DELETE FROM print_jobs WHERE printer_id=$1 AND idempotency_key=$2`, printerID, key)

	payload := &TicketData{
		PrinterID:       printerID,
		IdempotencyKey:  key,
		LicensePlate:    "TEST",
		DestinationName: "station-ksar-hlel",
		SeatNumber:      1,
		TotalAmount:     1.0,
		CreatedBy:       "Agent",
		CreatedAt:       time.Now(),
		StationName:     "Station",
		RouteName:       "Route",
	}

	id1, err := repo.CreateOrGetJob(ctx, printerID, key, nil, PrintJobTypeTalon, payload)
	if err != nil {
		t.Fatalf("CreateOrGetJob #1: %v", err)
	}
	id2, err := repo.CreateOrGetJob(ctx, printerID, key, nil, PrintJobTypeTalon, payload)
	if err != nil {
		t.Fatalf("CreateOrGetJob #2: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("expected same job id, got %q vs %q", id1, id2)
	}

	var count int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM print_jobs WHERE printer_id=$1 AND idempotency_key=$2`, printerID, key).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}
}

func TestPrintJobs_ClaimNextPending_FIFOOrder(t *testing.T) {
	db := mustTestDB(t)
	repo := NewPrintJobsRepository(db)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	printerID := "test-printer:9100"

	// Clean previous runs
	_, _ = db.Exec(ctx, `DELETE FROM print_jobs WHERE printer_id=$1 AND id LIKE 'job_test_fifo_%'`, printerID)

	// Insert 3 pending jobs with controlled created_at
	base := time.Now().Add(-2 * time.Minute).UTC()
	ids := []string{"job_test_fifo_1", "job_test_fifo_2", "job_test_fifo_3"}
	for i, id := range ids {
		createdAt := base.Add(time.Duration(i) * time.Second)
		_, err := db.Exec(ctx, `
			INSERT INTO print_jobs (id, booking_id, printer_id, job_type, payload_json, status, attempts, created_at, updated_at, idempotency_key)
			VALUES ($1, NULL, $2, $3, '{}'::jsonb, $4, 0, $5, $5, $6)
		`, id, printerID, string(PrintJobTypeTalon), string(PrintJobStatusPending), createdAt, "fifo-"+id)
		if err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}

	claimed1, err := repo.ClaimNextPending(ctx)
	if err != nil {
		t.Fatalf("claim #1: %v", err)
	}
	claimed2, err := repo.ClaimNextPending(ctx)
	if err != nil {
		t.Fatalf("claim #2: %v", err)
	}
	claimed3, err := repo.ClaimNextPending(ctx)
	if err != nil {
		t.Fatalf("claim #3: %v", err)
	}

	if claimed1.ID != ids[0] || claimed2.ID != ids[1] || claimed3.ID != ids[2] {
		t.Fatalf("expected FIFO %v, got %q %q %q", ids, claimed1.ID, claimed2.ID, claimed3.ID)
	}
}

