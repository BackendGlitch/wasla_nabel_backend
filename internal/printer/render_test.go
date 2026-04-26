package printer

import (
	"context"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// --- pure unit tests (no DB) ---

func TestMapTicketType_KnownValues(t *testing.T) {
	cases := map[string]PrintJobType{
		"booking":  PrintJobTypeBookingTicket,
		"entry":    PrintJobTypeEntryTicket,
		"exit":     PrintJobTypeExitTicket,
		"daypass":  PrintJobTypeDayPassTicket,
		"exitpass": PrintJobTypeExitPassTicket,
		"talon":    PrintJobTypeTalon,
		// case + whitespace tolerance
		" Booking ": PrintJobTypeBookingTicket,
		"TALON":     PrintJobTypeTalon,
	}
	for input, want := range cases {
		got, ok := mapTicketType(input)
		if !ok {
			t.Fatalf("mapTicketType(%q): expected ok=true", input)
		}
		if got != want {
			t.Fatalf("mapTicketType(%q): expected %q, got %q", input, want, got)
		}
	}
}

func TestMapTicketType_Unknown(t *testing.T) {
	for _, input := range []string{"", "unknown", "ticket", "booking_ticket"} {
		if _, ok := mapTicketType(input); ok {
			t.Fatalf("mapTicketType(%q): expected ok=false", input)
		}
	}
}

// --- DB-backed tests (skipped when TEST_DATABASE_URL/DATABASE_URL is unset) ---

func TestService_RenderTicket_PersistsRenderedClientLocalJob(t *testing.T) {
	db := mustTestDB(t)
	jobsRepo := NewPrintJobsRepository(db)
	auditRepo := NewAuditRepository(db)
	// nil printer.Repository is fine: RenderTicket never reads/writes Redis.
	svc := NewService(nil, auditRepo, jobsRepo)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Cleanup any leftovers from previous runs.
	_, _ = db.Exec(ctx, `DELETE FROM print_jobs WHERE printer_id LIKE 'client:render-test-%'`)

	machineID := "render-test-pos-1"
	idem := "render-test-talon-1"
	ticket := &TicketData{
		IdempotencyKey:  idem,
		LicensePlate:    "TUN-123-AB",
		DestinationName: "Sousse",
		SeatNumber:      3,
		TotalAmount:     5.15,
		BasePrice:       5.0,
		CreatedBy:       "Agent",
		CreatedAt:       time.Now(),
	}

	res, err := svc.RenderTicket(ctx, ticket, PrintJobTypeTalon, machineID)
	if err != nil {
		t.Fatalf("RenderTicket: %v", err)
	}
	if res.JobID == "" {
		t.Fatal("expected non-empty jobId")
	}
	if res.DeliveryMode != DeliveryModeClientLocal {
		t.Fatalf("expected deliveryMode=%q, got %q", DeliveryModeClientLocal, res.DeliveryMode)
	}
	if res.Status != PrintJobStatusRendered {
		t.Fatalf("expected status=%q, got %q", PrintJobStatusRendered, res.Status)
	}
	if res.AlreadyPrinted {
		t.Fatal("AlreadyPrinted must be false on a fresh render")
	}
	bytesOut, err := base64.StdEncoding.DecodeString(res.ContentBase64)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if len(bytesOut) == 0 {
		t.Fatal("expected non-empty ESC/POS bytes")
	}
	// Sanity: ESC/POS init sequence is the first two bytes (ESC @).
	if bytesOut[0] != 0x1B || bytesOut[1] != 0x40 {
		t.Fatalf("expected ESC/POS init prefix 0x1B 0x40, got %x %x", bytesOut[0], bytesOut[1])
	}

	// DB row should reflect the new client_local job.
	var (
		printerID    string
		status       string
		deliveryMode string
	)
	if err := db.QueryRow(ctx,
		`SELECT printer_id, status, delivery_mode FROM print_jobs WHERE id=$1`,
		res.JobID,
	).Scan(&printerID, &status, &deliveryMode); err != nil {
		t.Fatalf("query inserted row: %v", err)
	}
	if !strings.HasPrefix(printerID, ClientPrinterPrefix) {
		t.Fatalf("expected printer_id to start with %q, got %q", ClientPrinterPrefix, printerID)
	}
	if !strings.HasSuffix(printerID, machineID) {
		t.Fatalf("expected printer_id to end with machineID %q, got %q", machineID, printerID)
	}
	if status != string(PrintJobStatusRendered) {
		t.Fatalf("expected status=%q, got %q", PrintJobStatusRendered, status)
	}
	if deliveryMode != DeliveryModeClientLocal {
		t.Fatalf("expected delivery_mode=%q, got %q", DeliveryModeClientLocal, deliveryMode)
	}

	// Idempotent re-render: same (printer_id, idempotency_key) → same jobId.
	res2, err := svc.RenderTicket(ctx, ticket, PrintJobTypeTalon, machineID)
	if err != nil {
		t.Fatalf("RenderTicket #2: %v", err)
	}
	if res2.JobID != res.JobID {
		t.Fatalf("expected idempotent jobId, got %q vs %q", res.JobID, res2.JobID)
	}

	// Ack success transitions status to 'printed'.
	if err := svc.AckPrintJob(ctx, res.JobID, true, "", time.Now()); err != nil {
		t.Fatalf("AckPrintJob ok=true: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT status FROM print_jobs WHERE id=$1`, res.JobID).Scan(&status); err != nil {
		t.Fatalf("query post-ack row: %v", err)
	}
	if status != string(PrintJobStatusPrinted) {
		t.Fatalf("expected status=%q after ok ack, got %q", PrintJobStatusPrinted, status)
	}

	// Subsequent render returns AlreadyPrinted=true.
	res3, err := svc.RenderTicket(ctx, ticket, PrintJobTypeTalon, machineID)
	if err != nil {
		t.Fatalf("RenderTicket #3: %v", err)
	}
	if res3.JobID != res.JobID {
		t.Fatalf("expected idempotent jobId, got %q vs %q", res.JobID, res3.JobID)
	}
	if !res3.AlreadyPrinted {
		t.Fatal("expected AlreadyPrinted=true after ack ok")
	}
}

func TestService_AckPrintJob_RejectsBackendTcpJob(t *testing.T) {
	db := mustTestDB(t)
	jobsRepo := NewPrintJobsRepository(db)
	svc := NewService(nil, nil, jobsRepo)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Insert a backend_tcp pending job (legacy shape).
	id := "job_test_ack_reject_1"
	_, _ = db.Exec(ctx, `DELETE FROM print_jobs WHERE id=$1`, id)
	if _, err := db.Exec(ctx, `
		INSERT INTO print_jobs (id, booking_id, printer_id, job_type, payload_json, status, attempts, idempotency_key, delivery_mode)
		VALUES ($1, NULL, $2, $3, '{}'::jsonb, $4, 0, NULL, $5)
	`, id, "192.168.1.10:9100", string(PrintJobTypeTalon), string(PrintJobStatusPending), DeliveryModeBackendTCP); err != nil {
		t.Fatalf("seed backend_tcp row: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(context.Background(), `DELETE FROM print_jobs WHERE id=$1`, id) })

	err := svc.AckPrintJob(ctx, id, true, "", time.Now())
	if err == nil {
		t.Fatal("expected AckPrintJob to refuse a backend_tcp job, got nil error")
	}
	if !strings.Contains(err.Error(), "not a client_local job") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// Status must remain 'pending' (not flipped).
	var status string
	if err := db.QueryRow(ctx, `SELECT status FROM print_jobs WHERE id=$1`, id).Scan(&status); err != nil {
		t.Fatalf("query row: %v", err)
	}
	if status != string(PrintJobStatusPending) {
		t.Fatalf("expected status to remain %q, got %q", PrintJobStatusPending, status)
	}
}

func TestService_ProcessClaimedJob_BypassesClientLocal(t *testing.T) {
	db := mustTestDB(t)
	jobsRepo := NewPrintJobsRepository(db)
	// nil repo + nil audit: with the bypass we never reach the TCP path that
	// would dereference s.repo, so this is safe.
	svc := NewService(nil, nil, jobsRepo)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	job := &ClaimedJob{
		ID:        "job_test_bypass_1",
		PrinterID: "client:bypass-test",
		JobType:   PrintJobTypeTalon,
		Payload:   []byte(`{}`),
	}

	// Seed a pending row with the same id so MarkFailed has a target.
	_, _ = db.Exec(ctx, `DELETE FROM print_jobs WHERE id=$1`, job.ID)
	if _, err := db.Exec(ctx, `
		INSERT INTO print_jobs (id, booking_id, printer_id, job_type, payload_json, status, attempts, idempotency_key, delivery_mode)
		VALUES ($1, NULL, $2, $3, '{}'::jsonb, $4, 0, NULL, $5)
	`, job.ID, job.PrinterID, string(job.JobType), string(PrintJobStatusPending), DeliveryModeClientLocal); err != nil {
		t.Fatalf("seed row: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(context.Background(), `DELETE FROM print_jobs WHERE id=$1`, job.ID) })

	if err := svc.processClaimedJob(ctx, job); err != nil {
		t.Fatalf("processClaimedJob bypass returned error: %v", err)
	}

	var status string
	var lastErr *string
	if err := db.QueryRow(ctx,
		`SELECT status, last_error FROM print_jobs WHERE id=$1`, job.ID,
	).Scan(&status, &lastErr); err != nil {
		t.Fatalf("query row: %v", err)
	}
	if status != string(PrintJobStatusFailed) {
		t.Fatalf("expected status=%q after bypass, got %q", PrintJobStatusFailed, status)
	}
	if lastErr == nil || !strings.Contains(*lastErr, "client_local") {
		t.Fatalf("expected last_error to mention client_local, got %v", lastErr)
	}
}

// --- handler-level smoke test (no DB; just verifies routing/validation) ---

func TestRenderTicketHandler_RejectsUnknownType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(nil, nil)
	r := gin.New()
	r.POST("/api/printer/render/:type", h.RenderTicket)

	req := httptest.NewRequest("POST", "/api/printer/render/totally-unknown", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 for unknown type, got %d body=%s", w.Code, w.Body.String())
	}
}
