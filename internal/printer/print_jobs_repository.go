package printer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PrintJobStatus string

const (
	PrintJobStatusPending  PrintJobStatus = "pending"
	PrintJobStatusPrinted  PrintJobStatus = "printed"
	PrintJobStatusFailed   PrintJobStatus = "failed"
	PrintJobStatusPrinting PrintJobStatus = "printing"
	// PrintJobStatusRendered marks jobs whose ESC/POS bytes have been generated
	// by the backend and handed to a client (client_local delivery). They are
	// never picked up by the TCP worker; they wait for an /ack call.
	PrintJobStatusRendered PrintJobStatus = "rendered"
)

// Delivery modes for print_jobs (see migration 019).
const (
	DeliveryModeBackendTCP  = "backend_tcp"
	DeliveryModeClientLocal = "client_local"
)

type PrintJobRecord struct {
	ID           string         `json:"id"`
	BookingID    *string        `json:"bookingId,omitempty"`
	PrinterID    string         `json:"printerId"`
	JobType      PrintJobType   `json:"jobType"`
	Status       PrintJobStatus `json:"status"`
	Attempts     int            `json:"attempts"`
	LastError    *string        `json:"lastError,omitempty"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	PrintedAt    *time.Time     `json:"printedAt,omitempty"`
	DeliveryMode string         `json:"deliveryMode,omitempty"`
}

type PrintJobsRepository struct {
	db *pgxpool.Pool
}

func NewPrintJobsRepository(db *pgxpool.Pool) *PrintJobsRepository {
	return &PrintJobsRepository{db: db}
}

func (r *PrintJobsRepository) CreateJob(ctx context.Context, id string, bookingID *string, printerID string, jobType PrintJobType, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var bid interface{} = nil
	if bookingID != nil && *bookingID != "" {
		bid = *bookingID
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO print_jobs (
			id, booking_id, printer_id, job_type, payload_json, status, attempts, idempotency_key
		) VALUES (
			$1, $2, $3, $4, $5::jsonb, $6, 0, NULL
		)
	`, id, bid, printerID, string(jobType), string(raw), string(PrintJobStatusPending))
	return err
}

func (r *PrintJobsRepository) CreateOrGetJob(ctx context.Context, printerID string, idempotencyKey string, bookingID *string, jobType PrintJobType, payload any) (string, error) {
	if strings.TrimSpace(printerID) == "" {
		return "", fmt.Errorf("printerID is required")
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		// no dedupe requested; create a fresh job id
		id := generateJobID()
		return id, r.CreateJob(ctx, id, bookingID, printerID, jobType, payload)
	}

	// If already exists, return existing job id
	var existingID string
	err := r.db.QueryRow(ctx, `
		SELECT id
		FROM print_jobs
		WHERE printer_id=$1 AND idempotency_key=$2
		LIMIT 1
	`, printerID, idempotencyKey).Scan(&existingID)
	if err == nil && existingID != "" {
		return existingID, nil
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	id := generateJobID()
	var bid interface{} = nil
	if bookingID != nil && *bookingID != "" {
		bid = *bookingID
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO print_jobs (
			id, booking_id, printer_id, job_type, payload_json, status, attempts, idempotency_key
		) VALUES (
			$1, $2, $3, $4, $5::jsonb, $6, 0, $7
		)
		ON CONFLICT (printer_id, idempotency_key) DO NOTHING
	`, id, bid, printerID, string(jobType), string(raw), string(PrintJobStatusPending), idempotencyKey)
	if err != nil {
		return "", err
	}

	// If we lost the race, fetch the existing id
	err = r.db.QueryRow(ctx, `
		SELECT id
		FROM print_jobs
		WHERE printer_id=$1 AND idempotency_key=$2
		LIMIT 1
	`, printerID, idempotencyKey).Scan(&existingID)
	if err != nil {
		return "", err
	}
	return existingID, nil
}

// CreateOrGetRenderedJob inserts a row in 'rendered' status with delivery_mode='client_local'.
//
// Idempotency: if (printer_id, idempotency_key) already exists, returns the existing
// id and its current status (could be 'rendered', 'printed', or 'failed' from a prior
// ack). The caller is expected to re-render bytes from the request payload because the
// rendered ESC/POS bytes are not stored on disk (they are derived from payload_json).
//
// printerID MUST be prefixed with ClientPrinterPrefix ("client:") so the TCP worker can
// safely skip these rows even if one ever leaks into the pending queue.
func (r *PrintJobsRepository) CreateOrGetRenderedJob(
	ctx context.Context,
	printerID string,
	idempotencyKey string,
	bookingID *string,
	jobType PrintJobType,
	payload any,
) (string, PrintJobStatus, error) {
	if strings.TrimSpace(printerID) == "" {
		return "", "", fmt.Errorf("printerID is required")
	}

	idempotencyKey = strings.TrimSpace(idempotencyKey)

	// Fast-path: existing row by (printer_id, idempotency_key).
	if idempotencyKey != "" {
		var existingID string
		var existingStatus string
		err := r.db.QueryRow(ctx, `
			SELECT id, status
			FROM print_jobs
			WHERE printer_id=$1 AND idempotency_key=$2
			LIMIT 1
		`, printerID, idempotencyKey).Scan(&existingID, &existingStatus)
		if err == nil && existingID != "" {
			return existingID, PrintJobStatus(existingStatus), nil
		}
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}
	id := generateJobID()
	var bid interface{} = nil
	if bookingID != nil && *bookingID != "" {
		bid = *bookingID
	}
	var idem interface{} = nil
	if idempotencyKey != "" {
		idem = idempotencyKey
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO print_jobs (
			id, booking_id, printer_id, job_type, payload_json,
			status, attempts, idempotency_key, delivery_mode
		) VALUES (
			$1, $2, $3, $4, $5::jsonb,
			$6, 0, $7, $8
		)
		ON CONFLICT (printer_id, idempotency_key) DO NOTHING
	`,
		id, bid, printerID, string(jobType), string(raw),
		string(PrintJobStatusRendered), idem, DeliveryModeClientLocal,
	)
	if err != nil {
		return "", "", err
	}

	// If we lost the race or the row already existed, fetch the existing id+status.
	if idempotencyKey != "" {
		var existingID string
		var existingStatus string
		err = r.db.QueryRow(ctx, `
			SELECT id, status
			FROM print_jobs
			WHERE printer_id=$1 AND idempotency_key=$2
			LIMIT 1
		`, printerID, idempotencyKey).Scan(&existingID, &existingStatus)
		if err != nil {
			return "", "", err
		}
		return existingID, PrintJobStatus(existingStatus), nil
	}
	return id, PrintJobStatusRendered, nil
}

// MarkAcked records a client-side print result for a client_local job.
// On ok=true, sets status='printed', printed_at=printedAt, clears last_error.
// On ok=false, sets status='failed', last_error=errMsg, leaves printed_at untouched.
// Both paths increment attempts so the audit trail reflects retries.
func (r *PrintJobsRepository) MarkAcked(
	ctx context.Context,
	id string,
	ok bool,
	errMsg string,
	printedAt time.Time,
) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("id is required")
	}
	if ok {
		_, err := r.db.Exec(ctx, `
			UPDATE print_jobs
			SET status=$2, printed_at=$3, last_error=NULL, attempts=attempts+1
			WHERE id=$1
		`, id, string(PrintJobStatusPrinted), printedAt)
		return err
	}
	_, err := r.db.Exec(ctx, `
		UPDATE print_jobs
		SET status=$2, last_error=$3, attempts=attempts+1
		WHERE id=$1
	`, id, string(PrintJobStatusFailed), errMsg)
	return err
}

func (r *PrintJobsRepository) MarkPrinting(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE print_jobs
		SET status=$2, attempts=attempts+1
		WHERE id=$1
	`, id, string(PrintJobStatusPrinting))
	return err
}

func (r *PrintJobsRepository) MarkPrinted(ctx context.Context, id string, printedAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE print_jobs
		SET status=$2, printed_at=$3, last_error=NULL
		WHERE id=$1
	`, id, string(PrintJobStatusPrinted), printedAt)
	return err
}

func (r *PrintJobsRepository) MarkFailed(ctx context.Context, id string, errMsg string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE print_jobs
		SET status=$2, last_error=$3
		WHERE id=$1
	`, id, string(PrintJobStatusFailed), errMsg)
	return err
}

func (r *PrintJobsRepository) GetJob(ctx context.Context, id string) (*PrintJobRecord, error) {
	var rec PrintJobRecord
	var bookingID *string
	var lastErr *string
	var printedAt *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT id, booking_id, printer_id, job_type, status, attempts, last_error, created_at, updated_at, printed_at, delivery_mode
		FROM print_jobs
		WHERE id=$1
	`, id).Scan(
		&rec.ID, &bookingID, &rec.PrinterID, &rec.JobType, &rec.Status, &rec.Attempts, &lastErr, &rec.CreatedAt, &rec.UpdatedAt, &printedAt, &rec.DeliveryMode,
	)
	if err != nil {
		return nil, err
	}
	rec.BookingID = bookingID
	rec.LastError = lastErr
	rec.PrintedAt = printedAt
	return &rec, nil
}

// GetJobPayload returns the raw payload_json for a job. Used to re-render the
// ESC/POS bytes on idempotent /render calls without storing the bytes themselves.
func (r *PrintJobsRepository) GetJobPayload(ctx context.Context, id string) (PrintJobType, json.RawMessage, *string, error) {
	var jobType string
	var payload []byte
	var bookingID *string
	err := r.db.QueryRow(ctx, `
		SELECT job_type, payload_json, booking_id
		FROM print_jobs
		WHERE id=$1
	`, id).Scan(&jobType, &payload, &bookingID)
	if err != nil {
		return "", nil, nil, err
	}
	return PrintJobType(jobType), json.RawMessage(payload), bookingID, nil
}

func (r *PrintJobsRepository) ListRecent(ctx context.Context, limit int) ([]PrintJobRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, booking_id, printer_id, job_type, status, attempts, last_error, created_at, updated_at, printed_at, delivery_mode
		FROM print_jobs
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PrintJobRecord
	for rows.Next() {
		var rec PrintJobRecord
		var bookingID *string
		var lastErr *string
		var printedAt *time.Time
		if err := rows.Scan(
			&rec.ID, &bookingID, &rec.PrinterID, &rec.JobType, &rec.Status, &rec.Attempts, &lastErr, &rec.CreatedAt, &rec.UpdatedAt, &printedAt, &rec.DeliveryMode,
		); err != nil {
			return nil, err
		}
		rec.BookingID = bookingID
		rec.LastError = lastErr
		rec.PrintedAt = printedAt
		out = append(out, rec)
	}
	return out, nil
}

type ClaimedJob struct {
	ID        string
	PrinterID string
	JobType   PrintJobType
	Payload   json.RawMessage
	BookingID *string
}

// ClaimNextPending claims the oldest pending job (FIFO) and marks it printing.
// Uses SKIP LOCKED so multiple workers could exist (we will run one).
func (r *PrintJobsRepository) ClaimNextPending(ctx context.Context) (*ClaimedJob, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var job ClaimedJob
	var jobType string
	var bookingID *string
	err = tx.QueryRow(ctx, `
		SELECT id, booking_id, printer_id, job_type, payload_json
		FROM print_jobs
		WHERE status = $1
		ORDER BY created_at ASC, id ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`, string(PrintJobStatusPending)).Scan(&job.ID, &bookingID, &job.PrinterID, &jobType, &job.Payload)
	if err != nil {
		return nil, err
	}
	job.BookingID = bookingID
	job.JobType = PrintJobType(jobType)

	if _, err := tx.Exec(ctx, `
		UPDATE print_jobs
		SET status=$2, attempts=attempts+1
		WHERE id=$1
	`, job.ID, string(PrintJobStatusPrinting)); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &job, nil
}

