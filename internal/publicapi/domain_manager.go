package publicapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type StationDomainInfo struct {
	Slug      string    `json:"slug"`
	FQDN      string    `json:"fqdn"`
	URL       string    `json:"url"`
	TunnelID  string    `json:"tunnel_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DomainManager interface {
	EnsureStationDomain(ctx context.Context, stationID string, stationName string) (*StationDomainInfo, error)
}

type CloudflareDomainManager struct {
	cfg    domainConfig
	client *http.Client
	mu     sync.Mutex
}

type domainConfig struct {
	Enabled           bool
	APIToken          string
	AccountID         string
	ZoneID            string
	BaseDomain        string
	SubdomainRoot     string
	OriginURL         string
	StatePath         string
	PIDPath           string
	LogPath           string
	CloudflaredBin    string
	CloudflaredProto  string
	StartTunnel       bool
	RequestTimeout    time.Duration
	SlugMaxCollisions int
}

type domainState struct {
	StationID   string    `json:"station_id"`
	StationName string    `json:"station_name"`
	Slug        string    `json:"slug"`
	FQDN        string    `json:"fqdn"`
	TunnelID    string    `json:"tunnel_id"`
	TunnelToken string    `json:"tunnel_token"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type cfEnvelope struct {
	Success bool            `json:"success"`
	Errors  []cfError       `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfDNSRecord struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
}

type cfTunnelResult struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

func NewCloudflareDomainManagerFromEnv() *CloudflareDomainManager {
	subdomainRoot, hasSubdomainRoot := lookupEnvTrimmed("PUBLIC_DOMAIN_SUBDOMAIN_ROOT")
	cfg := domainConfig{
		APIToken:          strings.TrimSpace(os.Getenv("CF_API_TOKEN")),
		AccountID:         strings.TrimSpace(os.Getenv("CF_ACCOUNT_ID")),
		ZoneID:            strings.TrimSpace(os.Getenv("CF_ZONE_ID")),
		BaseDomain:        strings.TrimSpace(os.Getenv("PUBLIC_DOMAIN_BASE")),
		SubdomainRoot:     normalizeDomainPart(subdomainRoot),
		OriginURL:         strings.TrimSpace(os.Getenv("PUBLIC_DOMAIN_TUNNEL_ORIGIN_URL")),
		StatePath:         strings.TrimSpace(os.Getenv("PUBLIC_DOMAIN_STATE_FILE")),
		PIDPath:           strings.TrimSpace(os.Getenv("PUBLIC_DOMAIN_PID_FILE")),
		LogPath:           strings.TrimSpace(os.Getenv("PUBLIC_DOMAIN_LOG_FILE")),
		CloudflaredBin:    strings.TrimSpace(os.Getenv("PUBLIC_DOMAIN_CLOUDFLARED_BIN")),
		CloudflaredProto:  parseCloudflaredProtocol(os.Getenv("PUBLIC_DOMAIN_CLOUDFLARED_PROTOCOL"), "quic"),
		StartTunnel:       parseBoolDefault(os.Getenv("PUBLIC_DOMAIN_START_TUNNEL"), true),
		RequestTimeout:    parseDurationSecondsDefault(os.Getenv("PUBLIC_DOMAIN_REQUEST_TIMEOUT_SECONDS"), 15*time.Second),
		SlugMaxCollisions: parseIntDefault(os.Getenv("PUBLIC_DOMAIN_MAX_SLUG_COLLISIONS"), 500),
	}

	if cfg.BaseDomain == "" {
		cfg.BaseDomain = "backendglitch.com"
	}
	if !hasSubdomainRoot {
		cfg.SubdomainRoot = "station"
	}
	if cfg.OriginURL == "" {
		cfg.OriginURL = "http://localhost:8007"
	}
	if cfg.StatePath == "" {
		cfg.StatePath = "configs/station-cloudflare-state.json"
	}
	if cfg.PIDPath == "" {
		cfg.PIDPath = "station-cloudflared.pid"
	}
	if cfg.LogPath == "" {
		cfg.LogPath = "station-cloudflared.log"
	}
	if cfg.CloudflaredBin == "" {
		cfg.CloudflaredBin = "cloudflared"
	}
	if cfg.SlugMaxCollisions <= 0 {
		cfg.SlugMaxCollisions = 500
	}

	enabled := parseBoolDefault(os.Getenv("PUBLIC_DOMAIN_AUTOMATION_ENABLED"), false)
	if !enabled && cfg.APIToken != "" && cfg.AccountID != "" && cfg.ZoneID != "" {
		enabled = true
	}
	cfg.Enabled = enabled

	if !cfg.Enabled {
		return nil
	}

	if cfg.APIToken == "" || cfg.AccountID == "" || cfg.ZoneID == "" {
		return nil
	}

	return &CloudflareDomainManager{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
	}
}

func (m *CloudflareDomainManager) EnsureStationDomain(ctx context.Context, stationID string, stationName string) (*StationDomainInfo, error) {
	if m == nil || !m.cfg.Enabled {
		return nil, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, _ := m.loadState()
	if state != nil && m.stateMatchesStation(state, stationID, stationName) {
		if state.Slug == "" {
			state.Slug = slugify(stationName)
		}
		if err := m.ensureStateDomainMapping(ctx, state); err != nil {
			return m.infoFromState(state), err
		}
		if m.cfg.StartTunnel {
			if err := m.ensureCloudflaredRunning(state.TunnelToken, false); err != nil {
				return m.infoFromState(state), err
			}
		}
		return m.infoFromState(state), nil
	}

	baseSlug := slugify(stationName)
	slug, err := m.findUniqueSlug(ctx, baseSlug)
	if err != nil {
		return nil, err
	}
	fqdn := m.buildFQDN(slug)

	tunnelName := fmt.Sprintf("station-%s-%d", slug, time.Now().Unix())
	tunnelID, tunnelToken, err := m.createTunnel(ctx, tunnelName)
	if err != nil {
		return nil, err
	}

	if err := m.configureTunnelIngress(ctx, tunnelID, fqdn); err != nil {
		return nil, err
	}

	target := fmt.Sprintf("%s.cfargotunnel.com", tunnelID)
	if err := m.upsertDNSRecord(ctx, fqdn, target); err != nil {
		return nil, err
	}

	state = &domainState{
		StationID:   stationID,
		StationName: stationName,
		Slug:        slug,
		FQDN:        fqdn,
		TunnelID:    tunnelID,
		TunnelToken: tunnelToken,
		UpdatedAt:   time.Now().UTC(),
	}
	if err := m.saveState(state); err != nil {
		return m.infoFromState(state), err
	}

	if m.cfg.StartTunnel {
		if err := m.ensureCloudflaredRunning(tunnelToken, true); err != nil {
			return m.infoFromState(state), err
		}
	}

	return m.infoFromState(state), nil
}

func (m *CloudflareDomainManager) infoFromState(state *domainState) *StationDomainInfo {
	if state == nil {
		return nil
	}
	return &StationDomainInfo{
		Slug:      state.Slug,
		FQDN:      state.FQDN,
		URL:       "https://" + state.FQDN,
		TunnelID:  state.TunnelID,
		UpdatedAt: state.UpdatedAt,
	}
}

func (m *CloudflareDomainManager) stateMatchesStation(state *domainState, stationID string, stationName string) bool {
	if state == nil {
		return false
	}
	if stationID != "" && state.StationID != "" {
		return state.StationID == stationID
	}
	return strings.EqualFold(strings.TrimSpace(state.StationName), strings.TrimSpace(stationName))
}

func (m *CloudflareDomainManager) loadState() (*domainState, error) {
	data, err := os.ReadFile(m.cfg.StatePath)
	if err != nil {
		return nil, err
	}
	var state domainState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.FQDN == "" || state.TunnelID == "" || state.TunnelToken == "" {
		return nil, errors.New("invalid state file")
	}
	return &state, nil
}

func (m *CloudflareDomainManager) saveState(state *domainState) error {
	if state == nil {
		return errors.New("state is nil")
	}
	if err := os.MkdirAll(filepath.Dir(m.cfg.StatePath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(m.cfg.StatePath, data, 0o600); err != nil {
		return err
	}
	return nil
}

func (m *CloudflareDomainManager) buildFQDN(slug string) string {
	if m.cfg.SubdomainRoot == "" {
		return fmt.Sprintf("%s.%s", slug, m.cfg.BaseDomain)
	}
	return fmt.Sprintf("%s.%s.%s", slug, m.cfg.SubdomainRoot, m.cfg.BaseDomain)
}

func (m *CloudflareDomainManager) ensureStateDomainMapping(ctx context.Context, state *domainState) error {
	if state == nil {
		return nil
	}
	if state.Slug == "" {
		return errors.New("state slug is empty")
	}

	target := fmt.Sprintf("%s.cfargotunnel.com", state.TunnelID)
	slug := state.Slug
	fqdn := m.buildFQDN(slug)

	allowed, err := m.fqdnAvailableForTarget(ctx, fqdn, target)
	if err != nil {
		return err
	}
	if !allowed {
		slug, err = m.findUniqueSlug(ctx, slug)
		if err != nil {
			return err
		}
		fqdn = m.buildFQDN(slug)
	}

	if !strings.EqualFold(strings.TrimSpace(state.FQDN), fqdn) {
		if err := m.configureTunnelIngress(ctx, state.TunnelID, fqdn); err != nil {
			return err
		}
	}
	if err := m.upsertDNSRecord(ctx, fqdn, target); err != nil {
		return err
	}

	if !strings.EqualFold(strings.TrimSpace(state.FQDN), fqdn) || state.Slug != slug {
		state.Slug = slug
		state.FQDN = fqdn
		state.UpdatedAt = time.Now().UTC()
		if err := m.saveState(state); err != nil {
			return err
		}
	}
	return nil
}

func (m *CloudflareDomainManager) findUniqueSlug(ctx context.Context, baseSlug string) (string, error) {
	slug := baseSlug
	for i := 0; i <= m.cfg.SlugMaxCollisions; i++ {
		if i > 0 {
			slug = fmt.Sprintf("%s-%d", baseSlug, i+1)
		}
		fqdn := m.buildFQDN(slug)
		exists, err := m.dnsRecordExists(ctx, fqdn)
		if err != nil {
			return "", err
		}
		if !exists {
			return slug, nil
		}
	}
	return "", fmt.Errorf("could not allocate unique slug for base %q", baseSlug)
}

func (m *CloudflareDomainManager) dnsRecordExists(ctx context.Context, fqdn string) (bool, error) {
	query := url.QueryEscape(fqdn)
	records, err := m.getDNSRecordsByQuery(ctx, query)
	if err != nil {
		return false, err
	}
	return len(records) > 0, nil
}

func (m *CloudflareDomainManager) getDNSRecordsByName(ctx context.Context, fqdn string) ([]cfDNSRecord, error) {
	return m.getDNSRecordsByQuery(ctx, url.QueryEscape(fqdn))
}

func (m *CloudflareDomainManager) getDNSRecordsByQuery(ctx context.Context, query string) ([]cfDNSRecord, error) {
	var records []cfDNSRecord
	if err := m.cfRequest(ctx, http.MethodGet, "/zones/"+m.cfg.ZoneID+"/dns_records?type=CNAME&name="+query, nil, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func (m *CloudflareDomainManager) fqdnAvailableForTarget(ctx context.Context, fqdn string, target string) (bool, error) {
	records, err := m.getDNSRecordsByName(ctx, fqdn)
	if err != nil {
		return false, err
	}
	if len(records) == 0 {
		return true, nil
	}

	expected := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(target)), ".")
	for _, record := range records {
		content := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(record.Content)), ".")
		if content == expected {
			return true, nil
		}
	}
	return false, nil
}

func (m *CloudflareDomainManager) createTunnel(ctx context.Context, name string) (string, string, error) {
	secretRaw := make([]byte, 32)
	if _, err := rand.Read(secretRaw); err != nil {
		return "", "", err
	}
	secret := base64.StdEncoding.EncodeToString(secretRaw)
	payload := map[string]string{
		"name":          name,
		"tunnel_secret": secret,
		"config_src":    "cloudflare",
	}
	var res cfTunnelResult
	if err := m.cfRequest(ctx, http.MethodPost, "/accounts/"+m.cfg.AccountID+"/cfd_tunnel", payload, &res); err != nil {
		return "", "", err
	}
	if res.ID == "" {
		return "", "", errors.New("cloudflare tunnel id missing")
	}
	if res.Token != "" {
		return res.ID, res.Token, nil
	}

	var token string
	if err := m.cfRequest(ctx, http.MethodGet, "/accounts/"+m.cfg.AccountID+"/cfd_tunnel/"+res.ID+"/token", nil, &token); err != nil {
		return "", "", err
	}
	if token == "" {
		return "", "", errors.New("cloudflare tunnel token missing")
	}
	return res.ID, token, nil
}

func (m *CloudflareDomainManager) configureTunnelIngress(ctx context.Context, tunnelID string, fqdn string) error {
	payload := map[string]interface{}{
		"config": map[string]interface{}{
			"ingress": []map[string]string{
				{
					"hostname": fqdn,
					"service":  m.cfg.OriginURL,
				},
				{
					"service": "http_status:404",
				},
			},
		},
	}
	return m.cfRequest(ctx, http.MethodPut, "/accounts/"+m.cfg.AccountID+"/cfd_tunnel/"+tunnelID+"/configurations", payload, nil)
}

func (m *CloudflareDomainManager) upsertDNSRecord(ctx context.Context, fqdn string, target string) error {
	existing, err := m.getDNSRecordsByName(ctx, fqdn)
	if err != nil {
		return err
	}

	payload := map[string]interface{}{
		"type":    "CNAME",
		"name":    fqdn,
		"content": target,
		"proxied": true,
	}

	if len(existing) == 0 {
		return m.cfRequest(ctx, http.MethodPost, "/zones/"+m.cfg.ZoneID+"/dns_records", payload, nil)
	}
	return m.cfRequest(ctx, http.MethodPut, "/zones/"+m.cfg.ZoneID+"/dns_records/"+existing[0].ID, payload, nil)
}

func (m *CloudflareDomainManager) cfRequest(ctx context.Context, method string, path string, payload interface{}, out interface{}) error {
	reqCtx := ctx
	var cancel context.CancelFunc
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		reqCtx, cancel = context.WithTimeout(ctx, m.cfg.RequestTimeout)
		defer cancel()
	}

	var bodyReader *bytes.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(reqCtx, method, "https://api.cloudflare.com/client/v4"+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.cfg.APIToken)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var envelope cfEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	if !envelope.Success {
		if len(envelope.Errors) == 0 {
			return fmt.Errorf("cloudflare request failed: %s %s", method, path)
		}
		return fmt.Errorf("cloudflare request failed: %s (code=%d)", envelope.Errors[0].Message, envelope.Errors[0].Code)
	}

	if out == nil || len(envelope.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, out); err != nil {
		return err
	}
	return nil
}

func (m *CloudflareDomainManager) ensureCloudflaredRunning(token string, forceRestart bool) error {
	if token == "" {
		return errors.New("missing cloudflared token")
	}

	bin := m.cfg.CloudflaredBin
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("cloudflared binary not found: %w", err)
	}

	if forceRestart {
		_ = m.stopPIDProcess()
	} else if m.isPIDProcessRunning() {
		return nil
	}

	logFile, err := os.OpenFile(m.cfg.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := exec.Command(bin, "tunnel", "--no-autoupdate", "--protocol", m.cfg.CloudflaredProto, "run", "--token", token)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return err
	}

	if err := os.WriteFile(m.cfg.PIDPath, []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		return err
	}

	go func() {
		_ = cmd.Wait()
	}()

	time.Sleep(500 * time.Millisecond)
	if err := syscall.Kill(cmd.Process.Pid, 0); err != nil {
		return fmt.Errorf("cloudflared process failed to stay alive")
	}
	return nil
}

func (m *CloudflareDomainManager) isPIDProcessRunning() bool {
	pid, err := m.readPID()
	if err != nil {
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil {
		_ = os.Remove(m.cfg.PIDPath)
		return false
	}
	return true
}

func (m *CloudflareDomainManager) stopPIDProcess() error {
	pid, err := m.readPID()
	if err != nil {
		return nil
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
	time.Sleep(500 * time.Millisecond)
	_ = syscall.Kill(pid, syscall.SIGKILL)
	_ = os.Remove(m.cfg.PIDPath)
	return nil
}

func (m *CloudflareDomainManager) readPID() (int, error) {
	data, err := os.ReadFile(m.cfg.PIDPath)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, errors.New("invalid pid file")
	}
	return pid, nil
}

func parseBoolDefault(raw string, fallback bool) bool {
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func parseDurationSecondsDefault(raw string, fallback time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return fallback
	}
	return time.Duration(v) * time.Second
}

func parseIntDefault(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return v
}

func parseCloudflaredProtocol(raw string, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "auto", "quic", "http2":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return fallback
	}
}

func lookupEnvTrimmed(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func normalizeDomainPart(raw string) string {
	return strings.Trim(strings.TrimSpace(raw), ".")
}

func slugify(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return "station"
	}

	var b strings.Builder
	lastDash := false
	for _, ch := range raw {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			b.WriteRune(ch)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}

	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "station"
	}
	return slug
}
