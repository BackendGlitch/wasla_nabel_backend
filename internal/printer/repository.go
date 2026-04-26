package printer

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Repository handles printer data operations
type Repository struct {
	redis *redis.Client
}

// NewRepository creates a new printer repository
func NewRepository(redis *redis.Client) *Repository {
	return &Repository{
		redis: redis,
	}
}

// GetPrinterConfig retrieves printer configuration from Redis
func (r *Repository) GetPrinterConfig(printerID string) (*PrinterConfig, error) {
	ctx := context.Background()

	// Try to get from Redis first
	configKey := fmt.Sprintf("printer:config:%s", printerID)
	configData, err := r.redis.HGetAll(ctx, configKey).Result()
	if err != nil {
		return nil, err
	}

	// If not found in Redis, return default config
	if len(configData) == 0 {
		return r.getDefaultConfig(printerID), nil
	}

	// Parse config from Redis
	config := &PrinterConfig{
		ID:        configData["id"],
		Name:      configData["name"],
		IP:        configData["ip"],
		Port:      parseInt(configData["port"], 9100),
		Width:     parseInt(configData["width"], 48),
		Timeout:   parseInt(configData["timeout"], 10000),
		Model:     configData["model"],
		Enabled:   parseBool(configData["enabled"], true),
		IsDefault: parseBool(configData["isDefault"], false),
	}

	return config, nil
}

// SavePrinterConfig saves printer configuration to Redis
func (r *Repository) SavePrinterConfig(config *PrinterConfig) error {
	ctx := context.Background()

	configKey := fmt.Sprintf("printer:config:%s", config.ID)

	configData := map[string]interface{}{
		"id":        config.ID,
		"name":      config.Name,
		"ip":        config.IP,
		"port":      strconv.Itoa(config.Port),
		"width":     strconv.Itoa(config.Width),
		"timeout":   strconv.Itoa(config.Timeout),
		"model":     config.Model,
		"enabled":   strconv.FormatBool(config.Enabled),
		"isDefault": strconv.FormatBool(config.IsDefault),
	}

	return r.redis.HMSet(ctx, configKey, configData).Err()
}

// GetPrintQueue retrieves the current print queue
func (r *Repository) GetPrintQueue() ([]QueuedPrintJob, error) {
	ctx := context.Background()

	queueKey := "printer:queue"
	jobIDs, err := r.redis.LRange(ctx, queueKey, 0, -1).Result()
	if err != nil {
		return nil, err
	}

	var jobs []QueuedPrintJob
	for _, jobID := range jobIDs {
		jobKey := fmt.Sprintf("printer:job:%s", jobID)
		jobData, err := r.redis.HGetAll(ctx, jobKey).Result()
		if err != nil {
			continue
		}

		job := QueuedPrintJob{
			ID:         jobData["id"],
			JobType:    PrintJobType(jobData["jobType"]),
			Content:    jobData["content"],
			StaffName:  jobData["staffName"],
			Priority:   parseInt(jobData["priority"], 0),
			CreatedAt:  parseTime(jobData["createdAt"]),
			RetryCount: parseInt(jobData["retryCount"], 0),
		}

		jobs = append(jobs, job)
	}

	return jobs, nil
}

// AddPrintJob adds a job to the print queue
func (r *Repository) AddPrintJob(job *QueuedPrintJob) error {
	ctx := context.Background()

	jobKey := fmt.Sprintf("printer:job:%s", job.ID)
	queueKey := "printer:queue"

	// Save job data
	jobData := map[string]interface{}{
		"id":         job.ID,
		"jobType":    string(job.JobType),
		"content":    job.Content,
		"staffName":  job.StaffName,
		"priority":   strconv.Itoa(job.Priority),
		"createdAt":  job.CreatedAt.Format(time.RFC3339),
		"retryCount": strconv.Itoa(job.RetryCount),
	}

	if err := r.redis.HMSet(ctx, jobKey, jobData).Err(); err != nil {
		return err
	}

	// Add to queue (higher priority = lower number, so we use negative priority for sorting)
	score := float64(255 - job.Priority)
	return r.redis.ZAdd(ctx, queueKey, redis.Z{Score: score, Member: job.ID}).Err()
}

// RemovePrintJob removes a job from the print queue
func (r *Repository) RemovePrintJob(jobID string) error {
	ctx := context.Background()

	jobKey := fmt.Sprintf("printer:job:%s", jobID)
	queueKey := "printer:queue"

	// Remove from queue
	if err := r.redis.ZRem(ctx, queueKey, jobID).Err(); err != nil {
		return err
	}

	// Remove job data
	return r.redis.Del(ctx, jobKey).Err()
}

// UpdatePrintJobRetryCount updates the retry count for a print job
func (r *Repository) UpdatePrintJobRetryCount(jobID string, retryCount int) error {
	ctx := context.Background()

	jobKey := fmt.Sprintf("printer:job:%s", jobID)
	return r.redis.HSet(ctx, jobKey, "retryCount", retryCount).Err()
}

// GetPrintQueueStatus gets the current status of the print queue
func (r *Repository) GetPrintQueueStatus() (*PrintQueueStatus, error) {
	ctx := context.Background()

	queueKey := "printer:queue"
	queueLength, err := r.redis.ZCard(ctx, queueKey).Result()
	if err != nil {
		return nil, err
	}

	statusKey := "printer:status"
	statusData, err := r.redis.HGetAll(ctx, statusKey).Result()
	if err != nil {
		return nil, err
	}

	status := &PrintQueueStatus{
		QueueLength:  int(queueLength),
		IsProcessing: parseBool(statusData["isProcessing"], false),
		FailedJobs:   parseInt(statusData["failedJobs"], 0),
	}

	if lastPrintedStr := statusData["lastPrintedAt"]; lastPrintedStr != "" {
		if lastPrinted, err := time.Parse(time.RFC3339, lastPrintedStr); err == nil {
			status.LastPrintedAt = &lastPrinted
		}
	}

	return status, nil
}

// UpdatePrintQueueStatus updates the print queue status
func (r *Repository) UpdatePrintQueueStatus(status *PrintQueueStatus) error {
	ctx := context.Background()

	statusKey := "printer:status"
	statusData := map[string]interface{}{
		"isProcessing": strconv.FormatBool(status.IsProcessing),
		"failedJobs":   strconv.Itoa(status.FailedJobs),
	}

	if status.LastPrintedAt != nil {
		statusData["lastPrintedAt"] = status.LastPrintedAt.Format(time.RFC3339)
	}

	return r.redis.HMSet(ctx, statusKey, statusData).Err()
}

// TestPrinterConnection tests the connection to a printer
func (r *Repository) TestPrinterConnection(config *PrinterConfig) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", config.IP, config.Port), time.Duration(config.Timeout)*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to connect to printer %s (%s:%d): %v", config.Name, config.IP, config.Port, err)
	}
	defer conn.Close()

	// Send a simple test command
	testCommand := []byte{0x1B, 0x40} // ESC @ - Initialize printer
	_, err = conn.Write(testCommand)
	if err != nil {
		return fmt.Errorf("failed to send test command to printer: %v", err)
	}

	return nil
}

// SendPrintData sends raw print data to the printer
func (r *Repository) SendPrintData(config *PrinterConfig, data []byte) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", config.IP, config.Port), time.Duration(config.Timeout)*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to connect to printer %s (%s:%d): %v", config.Name, config.IP, config.Port, err)
	}
	defer conn.Close()

	// Send the data
	_, err = conn.Write(data)
	if err != nil {
		return fmt.Errorf("failed to send data to printer: %v", err)
	}

	return nil
}

// All printing now requires a reachable physical/network printer. No virtual mode.

// Helper functions
func (r *Repository) getDefaultConfig(printerID string) *PrinterConfig {
	// Get IP from environment variable or use default
	printerIP := os.Getenv("PRINTER_IP")
	if printerIP == "" {
		printerIP = "192.168.192.12" // Default IP
	}

	printerPort := 9100
	if portStr := os.Getenv("PRINTER_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			printerPort = port
		}
	}

	return &PrinterConfig{
		ID:        printerID,
		Name:      "TM-T20X Thermal Printer",
		IP:        printerIP,
		Port:      printerPort,
		Width:     48,
		Timeout:   10000,
		Model:     "TM-T20X",
		Enabled:   true,
		IsDefault: true,
	}
}

func parseInt(s string, defaultValue int) int {
	if s == "" {
		return defaultValue
	}
	if val, err := strconv.Atoi(s); err == nil {
		return val
	}
	return defaultValue
}

func parseBool(s string, defaultValue bool) bool {
	if s == "" {
		return defaultValue
	}
	if val, err := strconv.ParseBool(s); err == nil {
		return val
	}
	return defaultValue
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Now()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Now()
}
