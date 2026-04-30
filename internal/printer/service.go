package printer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"strconv"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"station-backend/internal/pricing"
)

const defaultCompanyName = "FATMA ZAHRA Services Transport"

var (
	defaultLogoOnce sync.Once
	defaultLogoData []byte
	defaultLogoErr  error
)

// ClientPrinterPrefix marks print_jobs.printer_id rows that are delivered to
// the client (POS device) instead of being TCP-dialed by the worker. Format:
//
//	"client:<machine-id>"   e.g. "client:pos-7e1c…" or "client:default"
//
// The TCP worker MUST refuse to handle any job whose printer_id starts with
// this prefix as a defense-in-depth check (rendered jobs never enter the
// pending queue, but we still guard the code path).
const ClientPrinterPrefix = "client:"

// Service handles printer business logic
type Service struct {
	repo      *Repository
	auditRepo *AuditRepository
	jobsRepo  *PrintJobsRepository
}

// NewService creates a new printer service
func NewService(repo *Repository, auditRepo *AuditRepository, jobsRepo *PrintJobsRepository) *Service {
	return &Service{
		repo:      repo,
		auditRepo: auditRepo,
		jobsRepo:  jobsRepo,
	}
}

// EnqueueTicket creates a durable FIFO print job and returns its job ID.
// If ticketData.IdempotencyKey is set, retries will return the same job ID (no duplicate tickets).
func (s *Service) EnqueueTicket(ctx context.Context, ticketData *TicketData, jobType PrintJobType) (string, error) {
	if s == nil || s.jobsRepo == nil {
		return "", fmt.Errorf("print jobs repository is not configured")
	}

	printerIDForJob := strings.TrimSpace(ticketData.PrinterID)
	if printerIDForJob == "" {
		if ticketData.PrinterConfig != nil && ticketData.PrinterConfig.IP != "" && ticketData.PrinterConfig.Port != 0 {
			printerIDForJob = fmt.Sprintf("%s:%d", ticketData.PrinterConfig.IP, ticketData.PrinterConfig.Port)
		} else {
			printerIDForJob = "default"
		}
	}

	var bookingIDPtr *string
	if strings.TrimSpace(ticketData.BookingID) != "" {
		bookingIDPtr = &ticketData.BookingID
	}

	return s.jobsRepo.CreateOrGetJob(ctx, printerIDForJob, ticketData.IdempotencyKey, bookingIDPtr, jobType, ticketData)
}

// StartPrintWorker runs a single FIFO worker that processes pending print_jobs in order.
func (s *Service) StartPrintWorker(ctx context.Context) {
	if s == nil || s.jobsRepo == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				job, err := s.jobsRepo.ClaimNextPending(ctx)
				if err != nil {
					continue
				}
				_ = s.processClaimedJob(ctx, job)
			}
		}
	}()
}

func (s *Service) processClaimedJob(ctx context.Context, job *ClaimedJob) error {
	if job == nil {
		return nil
	}

	// Defense in depth: client_local jobs (printer_id "client:<id>") are delivered
	// over HTTP to the POS device's local agent; the TCP worker must never dial
	// them. They are normally inserted in 'rendered' status (not 'pending') so
	// they don't even reach this code path, but if one ever leaks in we mark it
	// failed instead of attempting a TCP write to a non-existent printer.
	if strings.HasPrefix(job.PrinterID, ClientPrinterPrefix) {
		_ = s.jobsRepo.MarkFailed(ctx, job.ID, "client_local job mistakenly queued for backend_tcp worker; ignoring")
		return nil
	}

	var ticketData TicketData
	if err := json.Unmarshal(job.Payload, &ticketData); err != nil {
		_ = s.jobsRepo.MarkFailed(ctx, job.ID, "invalid payload_json")
		return err
	}

	// Use printer config from request, or fallback to default
	var config *PrinterConfig
	if ticketData.PrinterConfig != nil {
		config = &PrinterConfig{
			IP:      ticketData.PrinterConfig.IP,
			Port:    ticketData.PrinterConfig.Port,
			Width:   32,
			Timeout: 5000,
			Model:   "ESC/POS",
			Enabled: true,
		}
	} else {
		defaultConfig, err := s.repo.GetPrinterConfig("default")
		if err != nil {
			_ = s.jobsRepo.MarkFailed(ctx, job.ID, fmt.Sprintf("default printer config not found: %v", err))
			return err
		}
		config = defaultConfig
	}

	content := s.generateContentForJobType(&ticketData, job.JobType)
	escPosData := s.convertToESCPOS(content, config)

	if err := s.repo.SendPrintData(config, escPosData); err != nil {
		_ = s.jobsRepo.MarkFailed(ctx, job.ID, err.Error())
		return err
	}

	// ESC/POS over raw TCP is "fire-and-forget": Write() success doesn't guarantee the printer
	// has finished rendering/cutting. Under bursts we observed missing physical prints, while the
	// app already marked jobs as printed. Give the device a short moment to drain its buffer
	// before we move on / mark printed.
	time.Sleep(350 * time.Millisecond)

	// Best-effort audit log
	if s.auditRepo != nil {
		printedAt := time.Now()
		var bookingID *string
		if strings.TrimSpace(ticketData.BookingID) != "" {
			bookingID = &ticketData.BookingID
		}
		_ = s.auditRepo.LogPrinted(context.Background(), bookingID, job.PrinterID, job.JobType, printedAt)
	}

	_ = s.jobsRepo.MarkPrinted(ctx, job.ID, time.Now())
	return nil
}

// GetPrinterConfig retrieves printer configuration
func (s *Service) GetPrinterConfig(printerID string) (*PrinterConfig, error) {
	return s.repo.GetPrinterConfig(printerID)
}

// UpdatePrinterConfig updates printer configuration
func (s *Service) UpdatePrinterConfig(config *PrinterConfig) error {
	return s.repo.SavePrinterConfig(config)
}

// TestPrinterConnection tests the connection to a printer
func (s *Service) TestPrinterConnection(printerID string) error {
	config, err := s.repo.GetPrinterConfig(printerID)
	if err != nil {
		return err
	}

	return s.repo.TestPrinterConnection(config)
}

// GetPrintQueue retrieves the current print queue
func (s *Service) GetPrintQueue() ([]QueuedPrintJob, error) {
	return s.repo.GetPrintQueue()
}

// GetPrintQueueStatus retrieves the print queue status
func (s *Service) GetPrintQueueStatus() (*PrintQueueStatus, error) {
	return s.repo.GetPrintQueueStatus()
}

// AddPrintJob adds a job to the print queue
func (s *Service) AddPrintJob(jobType PrintJobType, content string, staffName string, priority int) (*QueuedPrintJob, error) {
	job := &QueuedPrintJob{
		ID:         generateJobID(),
		JobType:    jobType,
		Content:    content,
		StaffName:  staffName,
		Priority:   priority,
		CreatedAt:  time.Now(),
		RetryCount: 0,
	}

	err := s.repo.AddPrintJob(job)
	if err != nil {
		return nil, err
	}

	return job, nil
}

// PrintTicket prints a ticket directly using printer config from request
func (s *Service) PrintTicket(ticketData *TicketData, jobType PrintJobType) error {
	// Use printer config from request, or fallback to default
	var config *PrinterConfig
	if ticketData.PrinterConfig != nil {
		// Convert frontend config to internal config
		config = &PrinterConfig{
			IP:      ticketData.PrinterConfig.IP,
			Port:    ticketData.PrinterConfig.Port,
			Width:   32,        // Default width
			Timeout: 5000,      // Default timeout
			Model:   "ESC/POS", // Default model
			Enabled: true,      // Assume enabled if config provided
		}
	} else {
		// Fallback to default printer config
		defaultConfig, err := s.repo.GetPrinterConfig("default")
		if err != nil {
			return fmt.Errorf("no printer configuration provided and default config not found: %v", err)
		}
		config = defaultConfig
	}

	content := s.generateContentForJobType(ticketData, jobType)
	escPosData := s.convertToESCPOS(content, config)

	// Legacy path: immediate printing (non-queued). Prefer EnqueueTicket + worker in production.

	// Send to printer
	if err := s.repo.SendPrintData(config, escPosData); err != nil {
		return err
	}

	// Best-effort audit log (do not fail printing if audit insert fails).
	if s.auditRepo != nil {
		printerID := strings.TrimSpace(ticketData.PrinterID)
		if printerID == "" {
			if ticketData.PrinterConfig != nil && ticketData.PrinterConfig.IP != "" && ticketData.PrinterConfig.Port != 0 {
				printerID = fmt.Sprintf("%s:%d", ticketData.PrinterConfig.IP, ticketData.PrinterConfig.Port)
			} else if config != nil && config.ID != "" {
				printerID = config.ID
			} else {
				printerID = "default"
			}
		}

		printedAt := time.Now()
		var bookingID *string
		if ticketData.BookingID != "" {
			bookingID = &ticketData.BookingID
		}
		_ = s.auditRepo.LogPrinted(context.Background(), bookingID, printerID, jobType, printedAt)
	}

	return nil
}

// PrintBookingTicket prints a booking ticket
func (s *Service) PrintBookingTicket(ticketData *TicketData) error {
	return s.PrintTicket(ticketData, PrintJobTypeBookingTicket)
}

// PrintEntryTicket prints an entry ticket
func (s *Service) PrintEntryTicket(ticketData *TicketData) error {
	return s.PrintTicket(ticketData, PrintJobTypeEntryTicket)
}

// PrintExitTicket prints an exit ticket
func (s *Service) PrintExitTicket(ticketData *TicketData) error {
	return s.PrintTicket(ticketData, PrintJobTypeExitTicket)
}

// PrintDayPassTicket prints a day pass ticket
func (s *Service) PrintDayPassTicket(ticketData *TicketData) error {
	return s.PrintTicket(ticketData, PrintJobTypeDayPassTicket)
}

// PrintExitPassTicket prints an exit pass ticket
func (s *Service) PrintExitPassTicket(ticketData *TicketData) error {
	return s.PrintTicket(ticketData, PrintJobTypeExitPassTicket)
}

// PrintTalon prints a talon
func (s *Service) PrintTalon(ticketData *TicketData) error {
	return s.PrintTicket(ticketData, PrintJobTypeTalon)
}

// generateContentForJobType is the single switch over job types used by both
// the TCP worker and the client_local render flow, so the layout cannot drift
// between modes.
func (s *Service) generateContentForJobType(data *TicketData, jobType PrintJobType) string {
	switch jobType {
	case PrintJobTypeBookingTicket:
		return s.generateBookingTicketContent(data)
	case PrintJobTypeEntryTicket:
		return s.generateEntryTicketContent(data)
	case PrintJobTypeExitTicket:
		return s.generateExitTicketContent(data)
	case PrintJobTypeDayPassTicket:
		return s.generateDayPassTicketContent(data)
	case PrintJobTypeExitPassTicket:
		return s.generateExitPassTicketContent(data)
	case PrintJobTypeTalon:
		return s.generateTalonContent(data)
	default:
		return s.generateStandardTicketContent(data)
	}
}

// RenderResult is returned by RenderTicket. ContentBase64 contains the ESC/POS
// bytes the POS device should write to its USB printer.
type RenderResult struct {
	JobID          string         `json:"jobId"`
	Status         PrintJobStatus `json:"status"`
	ContentBase64  string         `json:"contentBase64"`
	AlreadyPrinted bool           `json:"alreadyPrinted"`
	DeliveryMode   string         `json:"deliveryMode"`
}

// RenderTicket is the client_local counterpart to EnqueueTicket. It produces the
// same ESC/POS bytes the TCP worker would have produced (single source of truth)
// and records a print_jobs row in 'rendered' status with delivery_mode='client_local'.
//
// The caller must POST /api/printer/jobs/:id/ack with ok=true after the bytes
// have been successfully written to the local USB printer (or ok=false on
// failure). The TCP worker will never touch these rows.
//
// machineID is the optional X-Wasla-Machine-Id header set by the staff Electron
// main; it becomes the suffix of printer_id ("client:<machineID>"). When empty,
// the printerId from the payload is used; otherwise a "client:default" sentinel.
func (s *Service) RenderTicket(
	ctx context.Context,
	ticketData *TicketData,
	jobType PrintJobType,
	machineID string,
) (*RenderResult, error) {
	if s == nil || s.jobsRepo == nil {
		return nil, fmt.Errorf("print jobs repository is not configured")
	}

	printerID := strings.TrimSpace(ticketData.PrinterID)
	if printerID == "" {
		printerID = strings.TrimSpace(machineID)
	}
	if printerID == "" {
		printerID = "default"
	}
	if !strings.HasPrefix(printerID, ClientPrinterPrefix) {
		printerID = ClientPrinterPrefix + printerID
	}

	var bookingID *string
	if strings.TrimSpace(ticketData.BookingID) != "" {
		bookingID = &ticketData.BookingID
	}

	jobID, status, err := s.jobsRepo.CreateOrGetRenderedJob(
		ctx, printerID, ticketData.IdempotencyKey, bookingID, jobType, ticketData,
	)
	if err != nil {
		return nil, err
	}

	// Re-render bytes from the *stored* payload when an idempotent row already exists,
	// so concurrent retries always see the exact bytes that match the persisted job.
	// For brand-new rows this is the same as rendering from ticketData directly.
	renderData := ticketData
	if status != PrintJobStatusRendered {
		if existingType, payload, _, gerr := s.jobsRepo.GetJobPayload(ctx, jobID); gerr == nil && len(payload) > 0 {
			var stored TicketData
			if uerr := json.Unmarshal(payload, &stored); uerr == nil {
				renderData = &stored
				jobType = existingType
			}
		}
	}

	// No printer IP/port for USB delivery; use a logical config for ESC/POS encoding.
	encConfig := &PrinterConfig{Width: 32, Timeout: 5000, Model: "ESC/POS", Enabled: true}
	escPosData := s.convertToESCPOS(s.generateContentForJobType(renderData, jobType), encConfig)

	return &RenderResult{
		JobID:          jobID,
		Status:         status,
		ContentBase64:  base64.StdEncoding.EncodeToString(escPosData),
		AlreadyPrinted: status == PrintJobStatusPrinted,
		DeliveryMode:   DeliveryModeClientLocal,
	}, nil
}

// AckPrintJob records the outcome of a client_local print attempt. It refuses
// to ack any job whose printer_id does not have the client_local prefix, so a
// caller cannot use this endpoint to flip the status of a backend_tcp job.
func (s *Service) AckPrintJob(
	ctx context.Context,
	jobID string,
	ok bool,
	errMsg string,
	printedAt time.Time,
) error {
	if s == nil || s.jobsRepo == nil {
		return fmt.Errorf("print jobs repository is not configured")
	}

	job, err := s.jobsRepo.GetJob(ctx, jobID)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(job.PrinterID, ClientPrinterPrefix) {
		return fmt.Errorf("job %s is not a client_local job (printer_id=%q)", jobID, job.PrinterID)
	}

	if ok {
		if printedAt.IsZero() {
			printedAt = time.Now()
		}
		if err := s.jobsRepo.MarkAcked(ctx, jobID, true, "", printedAt); err != nil {
			return err
		}
		if s.auditRepo != nil {
			_ = s.auditRepo.LogPrinted(context.Background(), job.BookingID, job.PrinterID, job.JobType, printedAt)
		}
		return nil
	}
	return s.jobsRepo.MarkAcked(ctx, jobID, false, errMsg, time.Time{})
}

var uuidLikePattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var hex24IDPattern = regexp.MustCompile(`^(?i)[0-9a-f]{24}$`)

func looksLikeInternalID(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if uuidLikePattern.MatchString(s) {
		return true
	}
	// 24-hex booking ids (substr(md5…)) sometimes appear as createdBy
	if hex24IDPattern.MatchString(s) {
		return true
	}
	return false
}

// looksLikeTechnicalStaffLoginLabel matches API/JWT quirks where booking.createdByName
// holds a supervisor username like "staff_supervisor_001" instead of a person's name.
func looksLikeTechnicalStaffLoginLabel(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return false
	}
	if strings.Contains(s, "staff_supervisor") {
		return true
	}
	if strings.HasPrefix(s, "staff_") && !strings.Contains(s, " ") {
		return true
	}
	return false
}

// agentLineForTicket prints one "Agent:" line. Prefer first+last; then createdByName; never show raw ids.
func agentLineForTicket(d *TicketData) string {
	first := strings.TrimSpace(d.StaffFirstName)
	last := strings.TrimSpace(d.StaffLastName)
	fromNames := strings.TrimSpace(first + " " + last)
	if fromNames != "" {
		return fromNames
	}
	if n := strings.TrimSpace(d.CreatedByName); n != "" && !looksLikeInternalID(n) && !looksLikeTechnicalStaffLoginLabel(n) {
		return n
	}
	by := strings.TrimSpace(d.CreatedBy)
	if by != "" && !looksLikeInternalID(by) && !looksLikeTechnicalStaffLoginLabel(by) {
		return by
	}
	if by != "" && looksLikeInternalID(by) {
		return "Agent"
	}
	return "Agent"
}

func companyNameForTicket(data *TicketData) string {
	name := strings.TrimSpace(data.CompanyName)
	if name == "" {
		return defaultCompanyName
	}
	return name
}

func logoMarkerForTicket(data *TicketData) string {
	logo := strings.TrimSpace(data.CompanyLogo)
	if logo != "" {
		return fmt.Sprintf("{{LOGO_PATH:%s}}", logo)
	}
	return "{{COMPANY_LOGO}}"
}

func defaultLogoCandidates() []string {
	paths := []string{}
	if env := strings.TrimSpace(os.Getenv("WASLA_COMPANY_LOGO_PATH")); env != "" {
		paths = append(paths, env)
	}
	paths = append(paths,
		"./assets/company-logo.png",
		"./static/assets/company-logo.png",
		"/app/assets/company-logo.png",
		"/app/static/assets/company-logo.png",
		"/opt/wasla_backend/assets/company-logo.png",
		"/opt/wasla_backend/static/assets/company-logo.png",
	)
	return paths
}

func resolveLogoPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		if u, err := url.Parse(path); err == nil {
			path = strings.TrimSpace(u.Path)
		}
	}
	if filepath.IsAbs(path) {
		if _, err := os.Stat(path); err == nil {
			return path
		}
		if strings.HasPrefix(path, "/assets/") {
			if _, err := os.Stat("." + path); err == nil {
				return "." + path
			}
		}
		return ""
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

func pngFileToEscPosRaster(path string, maxWidth int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}

	b := img.Bounds()
	srcW := b.Dx()
	srcH := b.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil, fmt.Errorf("invalid logo dimensions")
	}

	if maxWidth <= 0 {
		maxWidth = 256
	}
	dstW := srcW
	if dstW > maxWidth {
		dstW = maxWidth
	}
	dstH := srcH * dstW / srcW
	if dstH <= 0 {
		dstH = 1
	}

	bytesPerRow := (dstW + 7) / 8
	raster := make([]byte, bytesPerRow*dstH)
	threshold := uint32(180)

	for y := 0; y < dstH; y++ {
		srcY := b.Min.Y + (y*srcH)/dstH
		for x := 0; x < dstW; x++ {
			srcX := b.Min.X + (x*srcW)/dstW
			r, g, bb, a := img.At(srcX, srcY).RGBA()
			if a == 0 {
				continue
			}
			// Convert to luminance in [0,255] scale.
			luma := ((299*r + 587*g + 114*bb) / 1000) >> 8
			if luma < threshold {
				idx := y*bytesPerRow + (x / 8)
				raster[idx] |= 0x80 >> uint(x%8)
			}
		}
	}

	var out bytes.Buffer
	// GS v 0 m xL xH yL yH d1...dk
	out.Write([]byte{
		0x1D, 0x76, 0x30, 0x00,
		byte(bytesPerRow & 0xFF), byte((bytesPerRow >> 8) & 0xFF),
		byte(dstH & 0xFF), byte((dstH >> 8) & 0xFF),
	})
	out.Write(raster)
	out.WriteByte(0x0A)
	return out.Bytes(), nil
}

func defaultLogoEscPos() ([]byte, error) {
	defaultLogoOnce.Do(func() {
		for _, candidate := range defaultLogoCandidates() {
			p := resolveLogoPath(candidate)
			if p == "" {
				continue
			}
			// Use a larger logo width for 58mm printers while keeping safe margins.
			data, err := pngFileToEscPosRaster(p, 320)
			if err == nil {
				defaultLogoData = data
				return
			}
		}
		defaultLogoErr = fmt.Errorf("default logo not found")
	})
	return defaultLogoData, defaultLogoErr
}

// resolveServiceFeePerSeat picks the most reliable per-seat service fee:
// explicit payload -> total-base (single-seat tickets) -> global fallback.
func resolveServiceFeePerSeat(data *TicketData) float64 {
	if data.ServiceFee > 0 {
		return data.ServiceFee
	}
	if data.BasePrice > 0 && data.TotalAmount > 0 {
		fee := data.TotalAmount - data.BasePrice
		if fee >= 0 && fee < 1 {
			return fee
		}
	}
	return pricing.ServiceFeePerSeatTND
}

func fitRightLabel(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 {
		return ""
	}
	r := []rune(value)
	if len(r) <= max {
		return value
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "."
}

func talonBottomRow(left, right string, width int) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if width <= 0 {
		width = 32
	}
	if len([]rune(left)) >= width {
		return fitRightLabel(left, width)
	}
	remaining := width - len([]rune(left)) - 1
	if remaining < 1 {
		return left
	}
	right = fitRightLabel(right, remaining)
	spaces := width - len([]rune(left)) - len([]rune(right))
	if spaces < 1 {
		spaces = 1
	}
	return left + strings.Repeat(" ", spaces) + right
}

func talonTopRightStar(width int) string {
	const marker = "*"
	if width <= 0 {
		width = 32
	}
	rm := len([]rune(marker))
	if width < rm {
		return marker
	}
	return strings.Repeat(" ", width-rm) + marker
}

// Generate ticket content methods
func (s *Service) generateBookingTicketContent(data *TicketData) string {
	return RenderFrenchBookingTicket(data)
}

func (s *Service) generateEntryTicketContent(data *TicketData) string {
	return RenderFrenchEntryTicket(data)
}

func (s *Service) generateExitTicketContent(data *TicketData) string {
	return RenderFrenchExitTripTicket(data)
}

func tunisLocationForTickets() *time.Location {
	loc, err := time.LoadLocation("Africa/Tunis")
	if err != nil {
		return time.UTC
	}
	return loc
}

// tunisCalendarDayBounds returns [start, end] of the calendar day in Africa/Tunis for `at`.
func tunisCalendarDayBounds(at time.Time) (time.Time, time.Time) {
	loc := tunisLocationForTickets()
	t := at.In(loc)
	y, m, d := t.Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, loc)
	end := start.Add(24*time.Hour - time.Second)
	return start, end
}

func formatTicketDT(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(tunisLocationForTickets()).Format("02/01/2006 15:04")
}

func (s *Service) generateDayPassTicketContent(data *TicketData) string {
	return RenderFrenchDayPassTicket(data)
}

func (s *Service) generateExitPassTicketContent(data *TicketData) string {
	return RenderFrenchExitPassTicket(data)
}

func (s *Service) generateTalonContent(data *TicketData) string {
	return RenderFrenchTalonOnly(data)
}

func (s *Service) generateStandardTicketContent(data *TicketData) string {
	return RenderFrenchStandardTicket(data)
}

func frenchSepDashWidth(width int) int {
	if width <= 0 {
		return 32
	}
	if width > 48 {
		return 48
	}
	return width
}

// Convert text content to ESC/POS commands
func (s *Service) convertToESCPOS(content string, config *PrinterConfig) []byte {
	var buffer bytes.Buffer

	paperWidth := 32
	if config != nil && config.Width > 0 {
		paperWidth = config.Width
	}

	// Initialize printer
	buffer.WriteByte(0x1B) // ESC
	buffer.WriteByte(0x40) // @

	// Helper to set text alignment.
	setAlign := func(mode byte) {
		buffer.WriteByte(0x1B) // ESC
		buffer.WriteByte(0x61) // a
		buffer.WriteByte(mode) // 0=left,1=center,2=right
	}

	// Helper to set text style using ESC !.
	setTextStyle := func(mode byte) {
		buffer.WriteByte(0x1B) // ESC
		buffer.WriteByte(0x21) // !
		buffer.WriteByte(mode) // 0x00 normal, 0x08 emphasized
	}

	// Helper to set character scaling using GS !.
	setTextScale := func(mode byte) {
		buffer.WriteByte(0x1D) // GS
		buffer.WriteByte(0x21) // !
		buffer.WriteByte(mode) // 0x00 normal, 0x11 2x, 0x22 3x
	}

	// Reset print style to a known baseline.
	resetStyle := func() {
		setAlign(0x00)
		setTextStyle(0x00)
		setTextScale(0x00)
	}

	isTitleLine := func(line string) bool {
		return strings.Contains(line, "BILLET") ||
			strings.Contains(line, "TALON") ||
			strings.Contains(line, "AUTORISATION") ||
			strings.Contains(line, "STANDARD") ||
			strings.Contains(line, "RESERVATION")
	}

	resetStyle()
	isCompactTalon := false

	// Print content
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if tryConsumeArabEscPosDirective(&buffer, line, paperWidth, isCompactTalon) {
			continue
		}

		if line == "{{FR_SEP}}" {
			resetStyle()
			w := frenchSepDashWidth(paperWidth)
			buffer.WriteString(strings.Repeat("-", w))
			buffer.WriteByte(0x0A)
			continue
		}
		if strings.HasPrefix(line, "{{FR_CENTER_TITLE:") && strings.HasSuffix(line, "}}") {
			raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{FR_CENTER_TITLE:"), "}}")
			resetStyle()
			setAlign(0x01)
			setTextStyle(0x08)
			setTextScale(0x00)
			buffer.WriteString(raw)
			buffer.WriteByte(0x0A)
			resetStyle()
			if isCompactTalon {
				buffer.Write([]byte{0x1B, 0x4D, 0x01})
				buffer.Write([]byte{0x1B, 0x33, 18})
			}
			continue
		}

		if line == "{{TALON_TOP_RIGHT_STAR}}" {
			resetStyle()
			setAlign(0x00)
			buffer.WriteString(talonTopRightStar(paperWidth))
			buffer.WriteByte(0x0A)
			continue
		}

		// Compact talon markers: smaller font + tighter line spacing for noticeable mini talon.
		if line == "{{TALON_COMPACT_ON}}" {
			isCompactTalon = true
			// ESC M 1 -> Font B (smaller)
			buffer.Write([]byte{0x1B, 0x4D, 0x01})
			// ESC 3 n -> set line spacing to n dots (smaller than default)
			buffer.Write([]byte{0x1B, 0x33, 18})
			continue
		}
		if line == "{{TALON_COMPACT_OFF}}" {
			isCompactTalon = false
			// ESC 2 -> default line spacing
			buffer.Write([]byte{0x1B, 0x32})
			// ESC M 0 -> Font A
			buffer.Write([]byte{0x1B, 0x4D, 0x00})
			resetStyle()
			continue
		}

		// Dedicated marker for printing the ticket index in large, centered text.
		if strings.HasPrefix(line, "{{BIG_INDEX:") && strings.HasSuffix(line, "}}") {
			value := strings.TrimSuffix(strings.TrimPrefix(line, "{{BIG_INDEX:"), "}}")
			setAlign(0x01)
			setTextStyle(0x08)
			setTextScale(0x22)
			buffer.WriteString(value)
			buffer.WriteByte(0x0A) // Line feed
			resetStyle()
			continue
		}

		// Blank feed before cutters so blades never intersect the last inked line / wrap line.
		if line == "{{FEED_BEFORE_CUT}}" {
			// Keep enough paper advance before the inter-ticket cut so the line above
			// (often a long license plate/destination line) is fully out of the cutter zone.
			for range 3 {
				buffer.WriteByte(0x0A)
			}
			resetStyle()
			continue
		}
		if line == "{{SHORT_FEED_BEFORE_CUT}}" {
			buffer.WriteByte(0x0A)
			resetStyle()
			continue
		}
		if line == "{{PASSENGER_PRE_PARTIAL_FEED}}" {
			// Blank line feeds only (no spacer text) — short tail before partial cut after footer.
			for range 4 {
				buffer.WriteByte(0x0A)
			}
			resetStyle()
			continue
		}
		if strings.HasPrefix(line, "{{FR_LF_ONLY:") && strings.HasSuffix(line, "}}") {
			inner := strings.TrimSuffix(strings.TrimPrefix(line, "{{FR_LF_ONLY:"), "}}")
			n, err := strconv.Atoi(strings.TrimSpace(inner))
			if err != nil || n < 1 {
				n = 1
			}
			if n > 16 {
				n = 16
			}
			for range n {
				buffer.WriteByte(0x0A)
			}
			// Preserve compact Font B + tight spacing after naked feeds.
			if isCompactTalon {
				buffer.Write([]byte{0x1B, 0x4D, 0x01})
				buffer.Write([]byte{0x1B, 0x33, 18})
			}
			continue
		}

		recompactFrench := func() {
			if isCompactTalon {
				buffer.Write([]byte{0x1B, 0x4D, 0x01})
				buffer.Write([]byte{0x1B, 0x33, 18})
			}
		}
		useFontABriefly := func() {
			buffer.Write([]byte{0x1B, 0x4D, 0x00})
			buffer.Write([]byte{0x1B, 0x32})
			resetStyle()
		}

		// Passenger + talon: double width × double height centered "SIEGE n" (main visual anchor).
		if strings.HasPrefix(line, "{{FR_SEAT_FOCUS:") && strings.HasSuffix(line, "}}") {
			raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{FR_SEAT_FOCUS:"), "}}")
			n, err := strconv.Atoi(strings.TrimSpace(raw))
			if err != nil || n <= 0 {
				recompactFrench()
				continue
			}
			wasCompact := isCompactTalon
			if wasCompact {
				useFontABriefly()
			} else {
				resetStyle()
			}
			setAlign(0x01)
			setTextStyle(0x08)
			setTextScale(0x22)
			buffer.WriteString(fmt.Sprintf("SIEGE %d", n))
			buffer.WriteByte(0x0A)
			resetStyle()
			if wasCompact {
				useFontABriefly()
				recompactFrench()
			}
			continue
		}

		// Bold medium scale (typically vehicle line on passenger slip).
		if strings.HasPrefix(line, "{{FR_VEH_MEDIUM:") && strings.HasSuffix(line, "}}") {
			raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{FR_VEH_MEDIUM:"), "}}")
			raw = strings.TrimSpace(raw)
			if raw == "" {
				recompactFrench()
				continue
			}
			wasCompact := isCompactTalon
			if wasCompact {
				useFontABriefly()
			} else {
				resetStyle()
			}
			setAlign(0x01)
			setTextStyle(0x08)
			setTextScale(0x11)
			buffer.WriteString(raw)
			buffer.WriteByte(0x0A)
			resetStyle()
			if wasCompact {
				useFontABriefly()
				recompactFrench()
			}
			continue
		}

		// Centered bold normal size (tarif emphasis on passenger ticket).
		if strings.HasPrefix(line, "{{FR_BOLD_LINE:") && strings.HasSuffix(line, "}}") {
			raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{FR_BOLD_LINE:"), "}}")
			raw = strings.TrimSpace(raw)
			if raw == "" {
				recompactFrench()
				continue
			}
			resetStyle()
			setAlign(0x01)
			setTextStyle(0x08)
			setTextScale(0x00)
			buffer.WriteString(raw)
			buffer.WriteByte(0x0A)
			resetStyle()
			recompactFrench()
			continue
		}

		// Cut marker between printed blocks.
		if line == "{{PARTIAL_CUT}}" {
			// One extra safety feed right before cut for printers with delayed paper movement.
			buffer.WriteByte(0x0A)
			buffer.WriteByte(0x1D) // GS
			buffer.WriteByte(0x56) // V
			buffer.WriteByte(0x01) // partial cut
			resetStyle()
			continue
		}
		if line == "{{FULL_CUT}}" {
			buffer.WriteByte(0x1D) // GS
			buffer.WriteByte(0x56) // V
			buffer.WriteByte(0x00) // full cut
			buffer.WriteByte(0x0A)
			resetStyle()
			continue
		}

		if line == "{{COMPANY_LOGO}}" {
			setAlign(0x01)
			if logoData, err := defaultLogoEscPos(); err == nil && len(logoData) > 0 {
				buffer.Write(logoData)
				buffer.WriteByte(0x0A)
			}
			resetStyle()
			continue
		}

		if strings.HasPrefix(line, "{{LOGO_PATH:") && strings.HasSuffix(line, "}}") {
			raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{LOGO_PATH:"), "}}")
			setAlign(0x01)
			path := resolveLogoPath(raw)
			if path != "" {
				if logoData, err := pngFileToEscPosRaster(path, 320); err == nil && len(logoData) > 0 {
					buffer.Write(logoData)
					buffer.WriteByte(0x0A)
				}
			}
			resetStyle()
			continue
		}

		// Compact centered line (Font B) for long company names.
		if strings.HasPrefix(line, "{{CENTER_SMALL:") && strings.HasSuffix(line, "}}") {
			value := strings.TrimSuffix(strings.TrimPrefix(line, "{{CENTER_SMALL:"), "}}")
			setAlign(0x01)
			setTextStyle(0x01) // Font B (smaller)
			setTextScale(0x00)
			buffer.WriteString(value)
			buffer.WriteByte(0x0A) // Line feed
			resetStyle()
			continue
		}
		// Talon-only: prominent seat row above the dashed line (slightly enlarged, centered).
		if strings.HasPrefix(line, "{{TALON_SEAT_PROMINENT:") && strings.HasSuffix(line, "}}") {
			raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{TALON_SEAT_PROMINENT:"), "}}")
			n, err := strconv.Atoi(strings.TrimSpace(raw))
			if err != nil {
				resetStyle()
				continue
			}
			resetStyle()
			setAlign(0x01)
			setTextStyle(0x08) // bold
			setTextScale(0x01) // double width (visibly larger body, still below TALON_LP_BIG)
			buffer.WriteString(fmt.Sprintf("SIEGE %d", n))
			buffer.WriteByte(0x0A)
			resetStyle()
			continue
		}
		if strings.HasPrefix(line, "{{TALON_LP_BIG:") && strings.HasSuffix(line, "}}") {
			value := strings.TrimSuffix(strings.TrimPrefix(line, "{{TALON_LP_BIG:"), "}}")
			setAlign(0x01)
			setTextStyle(0x08) // bold
			setTextScale(0x11) // larger for plate readability
			buffer.WriteString(value)
			buffer.WriteByte(0x0A)
			resetStyle()
			if isCompactTalon {
				buffer.Write([]byte{0x1B, 0x4D, 0x01})
				buffer.Write([]byte{0x1B, 0x33, 18})
			}
			continue
		}
		if strings.HasPrefix(line, "{{TALON_BOTTOM_ROW:") && strings.HasSuffix(line, "}}") {
			raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{TALON_BOTTOM_ROW:"), "}}")
			parts := strings.SplitN(raw, "|", 2)
			left := raw
			right := ""
			if len(parts) == 2 {
				left = strings.TrimSpace(parts[0])
				right = strings.TrimSpace(parts[1])
			}
			setAlign(0x00)
			setTextStyle(0x00)
			setTextScale(0x00)
			buffer.WriteString(talonBottomRow(left, right, paperWidth))
			buffer.WriteByte(0x0A)
			continue
		}

		switch {
		case isTitleLine(line):
			setAlign(0x01)
			setTextStyle(0x08)
			setTextScale(0x00)
		case strings.HasPrefix(line, "INDEX SIEGE:"):
			setAlign(0x01)
			setTextStyle(0x08)
			setTextScale(0x00)
		case strings.HasPrefix(line, "====") || strings.HasPrefix(line, "----"):
			setAlign(0x01)
			setTextStyle(0x00)
			setTextScale(0x00)
		default:
			setAlign(0x00)
			setTextStyle(0x00)
			setTextScale(0x00)
		}
		if isCompactTalon {
			// Keep talon compact even for title/separator lines.
			buffer.Write([]byte{0x1B, 0x4D, 0x01})
		}

		buffer.WriteString(line)
		buffer.WriteByte(0x0A) // Line feed
	}

	resetStyle()

	// Shorter tail feed before final cut — saves paper on compact booking + talon jobs.
	for range 2 {
		buffer.WriteByte(0x0A)
	}

	// Cut paper
	buffer.WriteByte(0x1D) // GS
	buffer.WriteByte(0x56) // V
	buffer.WriteByte(0x00) // Full cut

	return buffer.Bytes()
}

// Helper function to generate unique job ID
func generateJobID() string {
	return fmt.Sprintf("job_%d", time.Now().UnixNano())
}
