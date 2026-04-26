package printer

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditRepository struct {
	db *pgxpool.Pool
}

func NewAuditRepository(db *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) LogPrinted(ctx context.Context, bookingID *string, printerID string, jobType PrintJobType, printedAt time.Time) error {
	var bid interface{} = nil
	if bookingID != nil && *bookingID != "" {
		bid = *bookingID
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO print_audit_logs (booking_id, printer_id, job_type, printed_at)
		VALUES ($1, $2, $3, $4)
	`, bid, printerID, string(jobType), printedAt)
	return err
}

