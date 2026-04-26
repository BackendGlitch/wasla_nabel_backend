package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

type loginResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Token string `json:"token"`
	} `json:"data"`
}

func baseURL(env string, def string) string {
	v := os.Getenv(env)
	if v == "" {
		return def
	}
	return v
}

func login(t *testing.T) string {
	t.Helper()
	authBase := baseURL("E2E_AUTH_BASE", "http://localhost:8001")
	body := map[string]any{"cin": "12345678"}
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", authBase+"/api/v1/auth/login", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("login status=%d", resp.StatusCode)
	}
	var out loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if out.Data.Token == "" {
		t.Fatalf("empty token in login response")
	}
	return out.Data.Token
}

func TestE2E_GhostBookingAndPrint_Idempotent_10Clicks(t *testing.T) {
	// Requires services running locally.
	token := login(t)

	bookingBase := baseURL("E2E_BOOKING_BASE", "http://localhost:8003")
	printerBase := baseURL("E2E_PRINTER_BASE", "http://localhost:8005")

	idem := fmt.Sprintf("e2e-%d", time.Now().UnixNano())

	type ghostResp struct {
		Success bool `json:"success"`
		Data    struct {
			ID         string `json:"id"`
			SeatNumber int    `json:"seatNumber"`
			// DestinationID string `json:"destinationId"`
		} `json:"data"`
	}

	createGhost := func() (ghostResp, int, error) {
		var out ghostResp
		body := map[string]any{
			"destinationId":   "station-ksar-hlel",
			"seats":           1,
			"idempotencyKey":  idem,
		}
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", bookingBase+"/api/v1/bookings/ghost", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return out, 0, err
		}
		defer resp.Body.Close()
		status := resp.StatusCode
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out, status, nil
	}

	// Rapid 10 clicks => still 1 booking id.
	var wg sync.WaitGroup
	wg.Add(10)
	type result struct {
		id     string
		status int
		err    error
	}
	results := make(chan result, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			r, status, err := createGhost()
			if err != nil {
				results <- result{err: err}
				return
			}
			results <- result{id: r.Data.ID, status: status}
		}()
	}
	wg.Wait()
	close(results)

	var bookingID string
	for r := range results {
		if r.err != nil {
			t.Fatalf("ghost request error: %v", r.err)
		}
			if r.status != 200 && r.status != 201 {
			t.Fatalf("ghost request status=%d", r.status)
		}
		if bookingID == "" {
			bookingID = r.id
		} else if r.id != bookingID {
			t.Fatalf("expected same bookingId, got %q vs %q", bookingID, r.id)
		}
	}
	if bookingID == "" {
		t.Fatalf("empty bookingId")
	}

	// Print: enqueue twice with same key => same jobId.
	printIdem := "print-" + idem
	printBody := map[string]any{
		"bookingId":       bookingID,
		"printerId":       "192.168.192.12:9100",
		"idempotencyKey":  printIdem,
		"licensePlate":    "TEST",
		"destinationName": "station-ksar-hlel",
		"seatNumber":      1,
		"totalAmount":     1.0,
		"createdBy":       "Agent",
		"createdAt":       time.Now().UTC().Format(time.RFC3339),
		"stationName":     "Station",
		"routeName":       "Route",
		"printerConfig": map[string]any{
			"ip":   "192.168.192.12",
			"port": 9100,
		},
	}
	raw, _ := json.Marshal(printBody)
	doPrint := func() (string, int, error) {
		req, _ := http.NewRequest("POST", printerBase+"/api/printer/print/booking", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", 0, err
		}
		defer resp.Body.Close()
		var out struct {
			JobID string `json:"jobId"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out.JobID, resp.StatusCode, nil
	}

	job1, s1, err := doPrint()
	if err != nil {
		t.Fatalf("print #1: %v", err)
	}
	job2, s2, err := doPrint()
	if err != nil {
		t.Fatalf("print #2: %v", err)
	}
	if s1 != 202 || s2 != 202 {
		t.Fatalf("expected 202 accepted, got %d and %d", s1, s2)
	}
	if job1 == "" || job2 == "" || job1 != job2 {
		t.Fatalf("expected same jobId, got %q and %q", job1, job2)
	}

	// Wait until printed (best-effort; worker should mark it printed quickly).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("job did not complete in time: %s", job1)
		default:
			req, _ := http.NewRequest("GET", printerBase+"/api/printer/jobs/"+job1, nil)
			resp, err := http.DefaultClient.Do(req)
			if err == nil && resp.StatusCode == 200 {
				var rec struct {
					Status string `json:"status"`
				}
				_ = json.NewDecoder(resp.Body).Decode(&rec)
				resp.Body.Close()
				if rec.Status == "printed" {
					return
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
}

