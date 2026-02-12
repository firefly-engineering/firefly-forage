// Package proxy provides an HTTP proxy for API key injection and rate limiting
package proxy

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

// Config holds proxy configuration
type Config struct {
	// ListenAddr is the address to listen on (e.g., ":8080")
	ListenAddr string

	// SecretsDir is the directory containing API key files
	SecretsDir string

	// TargetURL is the upstream API URL (e.g., "https://api.anthropic.com")
	TargetURL string

	// RateLimitRequests is the max requests per window (0 = unlimited)
	RateLimitRequests int

	// RateLimitWindow is the rate limit window duration
	RateLimitWindow time.Duration

	// AuditLogPath is the path to write audit logs (empty = no logging)
	AuditLogPath string

	// APIKeyFilename is the name of the file within each sandbox's secrets
	// directory that contains the API key. Defaults to "anthropic-api-key".
	APIKeyFilename string

	// SandboxesDir is the directory containing sandbox metadata files.
	// When set, enables IP-based sandbox identity verification.
	SandboxesDir string

	// Logger for proxy operations
	Logger *slog.Logger

	// Transport is an optional HTTP transport for the reverse proxy.
	// Used in tests to supply a TLS-aware transport for test servers.
	Transport http.RoundTripper
}

// Proxy is an HTTP reverse proxy with auth injection
type Proxy struct {
	config       *Config
	reverseProxy *httputil.ReverseProxy
	rateLimiter  *rateLimiter
	auditLog     *auditLogger
	apiKeys      map[string]string // sandbox name -> API key
	ipToSandbox  map[string]string // container IP -> sandbox name
	keysMu       sync.RWMutex
}

// New creates a new proxy instance
func New(cfg *Config) (*Proxy, error) {
	target, err := url.Parse(cfg.TargetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	// Validate the target URL scheme to prevent plaintext key transmission
	if target.Scheme != "https" {
		return nil, fmt.Errorf("proxy target must use HTTPS (got %q) to protect API keys in transit", target.Scheme)
	}

	// Reject targets that could be used for SSRF against internal services.
	// Skip this check when a custom Transport is provided (used in tests
	// with httptest.NewTLSServer which binds to 127.0.0.1).
	targetHost := target.Hostname()
	if cfg.Transport == nil && isInternalHost(targetHost) {
		return nil, fmt.Errorf("proxy target must not point to internal/link-local addresses: %s", targetHost)
	}

	if cfg.APIKeyFilename == "" {
		cfg.APIKeyFilename = "anthropic-api-key"
	}

	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	p := &Proxy{
		config:      cfg,
		apiKeys:     make(map[string]string),
		ipToSandbox: make(map[string]string),
	}

	// Create reverse proxy
	p.reverseProxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host

			// Remove hop-by-hop headers
			req.Header.Del("Connection")
			req.Header.Del("Proxy-Connection")
			req.Header.Del("Proxy-Authenticate")
			req.Header.Del("Proxy-Authorization")
		},
		ModifyResponse: p.modifyResponse,
		ErrorHandler:   p.errorHandler,
	}

	// Use custom transport if provided (e.g., for TLS test servers)
	if cfg.Transport != nil {
		p.reverseProxy.Transport = cfg.Transport
	}

	// Create rate limiter if configured
	if cfg.RateLimitRequests > 0 {
		p.rateLimiter = newRateLimiter(cfg.RateLimitRequests, cfg.RateLimitWindow)
	}

	// Create audit logger if configured
	if cfg.AuditLogPath != "" {
		al, err := newAuditLogger(cfg.AuditLogPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create audit logger: %w", err)
		}
		p.auditLog = al
	}

	return p, nil
}

// ServeHTTP implements http.Handler
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// Extract and verify sandbox identity.
	// Prefer X-Forage-Sandbox header but verify it matches source IP.
	sandboxName := r.Header.Get("X-Forage-Sandbox")
	if sandboxName != "" {
		sandboxName = p.verifySandboxIdentity(sandboxName, r.RemoteAddr)
	} else {
		// Fall back to checking source IP against known sandbox IPs
		sandboxName = p.identifySandbox(r.RemoteAddr)
	}

	p.config.Logger.Debug("proxy request",
		"method", r.Method,
		"path", r.URL.Path,
		"sandbox", sandboxName,
		"remote", r.RemoteAddr)

	// Check rate limit
	if p.rateLimiter != nil && sandboxName != "" {
		if !p.rateLimiter.allow(sandboxName) {
			p.config.Logger.Warn("rate limit exceeded", "sandbox", sandboxName)
			http.Error(w, `{"error": {"type": "rate_limit_error", "message": "Rate limit exceeded"}}`,
				http.StatusTooManyRequests)
			return
		}
	}

	// Inject API key if available
	if sandboxName != "" {
		apiKey := p.getAPIKey(sandboxName)
		if apiKey != "" {
			r.Header.Set("X-Api-Key", apiKey)
			// Only set X-Api-Key (used by Anthropic API). Do not also
			// set Authorization: Bearer as it doubles leakage surface.
			// Remove the sandbox header before forwarding
			r.Header.Del("X-Forage-Sandbox")
		}
	}

	// Wrap response writer for logging
	lw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	// Forward request
	p.reverseProxy.ServeHTTP(lw, r)

	// Audit log
	if p.auditLog != nil {
		p.auditLog.log(auditEntry{
			Timestamp:   startTime,
			Duration:    time.Since(startTime),
			Sandbox:     sandboxName,
			Method:      r.Method,
			Path:        r.URL.Path,
			StatusCode:  lw.statusCode,
			RequestSize: r.ContentLength,
			RemoteAddr:  r.RemoteAddr,
		})
	}
}

// LoadAPIKeys loads API keys from the secrets directory and builds the
// IP-to-sandbox mapping from sandbox metadata (if SandboxesDir is configured).
func (p *Proxy) LoadAPIKeys() error {
	p.keysMu.Lock()
	defer p.keysMu.Unlock()

	entries, err := os.ReadDir(p.config.SecretsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read secrets directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Each sandbox has a subdirectory with its secrets
			sandboxName := entry.Name()
			keyPath := filepath.Join(p.config.SecretsDir, sandboxName, p.config.APIKeyFilename)
			data, err := os.ReadFile(keyPath)
			if err != nil {
				if !os.IsNotExist(err) {
					p.config.Logger.Warn("failed to read API key", "sandbox", sandboxName, "error", err)
				}
				continue
			}
			p.apiKeys[sandboxName] = strings.TrimSpace(string(data))
			p.config.Logger.Debug("loaded API key", "sandbox", sandboxName)
		}
	}

	// Build IP-to-sandbox mapping from metadata
	if p.config.SandboxesDir != "" {
		p.loadIPMapping()
	}

	return nil
}

// loadIPMapping builds the IP-to-sandbox mapping from sandbox metadata.
// Must be called with keysMu held.
func (p *Proxy) loadIPMapping() {
	metadatas, err := config.ListSandboxes(p.config.SandboxesDir)
	if err != nil {
		p.config.Logger.Warn("failed to list sandbox metadata for IP mapping", "error", err)
		return
	}
	p.ipToSandbox = make(map[string]string, len(metadatas))
	for _, meta := range metadatas {
		ip := meta.ContainerIP()
		p.ipToSandbox[ip] = meta.Name
		p.config.Logger.Debug("mapped sandbox IP", "sandbox", meta.Name, "ip", ip)
	}
}

// getAPIKey returns the API key for a sandbox
func (p *Proxy) getAPIKey(sandboxName string) string {
	p.keysMu.RLock()
	defer p.keysMu.RUnlock()
	return p.apiKeys[sandboxName]
}

// identifySandbox identifies the sandbox from the remote address using the
// IP-to-sandbox mapping built from sandbox metadata.
func (p *Proxy) identifySandbox(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return ""
	}
	p.keysMu.RLock()
	defer p.keysMu.RUnlock()
	return p.ipToSandbox[host]
}

// verifySandboxIdentity checks that the X-Forage-Sandbox header matches
// the source IP. Returns the verified sandbox name or empty string.
func (p *Proxy) verifySandboxIdentity(headerName, remoteAddr string) string {
	if headerName == "" {
		return ""
	}
	// If we have IP mapping, verify the header matches the source
	if len(p.ipToSandbox) > 0 {
		host, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			return headerName // can't verify, trust header
		}
		p.keysMu.RLock()
		ipSandbox := p.ipToSandbox[host]
		p.keysMu.RUnlock()
		if ipSandbox != "" && ipSandbox != headerName {
			p.config.Logger.Warn("sandbox identity mismatch",
				"header", headerName,
				"ip_sandbox", ipSandbox,
				"remote", remoteAddr)
			return "" // reject mismatched identity
		}
	}
	return headerName
}

// isInternalHost returns true if the host resolves to a loopback, link-local,
// or private address that could be used for SSRF attacks.
func isInternalHost(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		// Try resolving hostname
		ips, err := net.LookupIP(host)
		if err != nil || len(ips) == 0 {
			return false
		}
		ip = ips[0]
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func (p *Proxy) modifyResponse(resp *http.Response) error {
	// No CORS headers - the proxy should only be accessed by sandboxes
	// directly, not from browsers. Adding Access-Control-Allow-Origin: *
	// would allow any website to make API calls through the proxy.
	return nil
}

func (p *Proxy) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	p.config.Logger.Error("proxy error", "error", err, "path", r.URL.Path)
	http.Error(w, `{"error": {"type": "proxy_error", "message": "Proxy error"}}`,
		http.StatusBadGateway)
}

// Close closes the proxy and releases resources
func (p *Proxy) Close() error {
	if p.rateLimiter != nil {
		p.rateLimiter.stop()
	}
	if p.auditLog != nil {
		return p.auditLog.close()
	}
	return nil
}

// loggingResponseWriter wraps http.ResponseWriter to capture status code
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lw *loggingResponseWriter) WriteHeader(code int) {
	lw.statusCode = code
	lw.ResponseWriter.WriteHeader(code)
}

// rateLimiter implements per-sandbox rate limiting
type rateLimiter struct {
	maxRequests int
	window      time.Duration
	requests    map[string][]time.Time
	mu          sync.Mutex
	stopClean   chan struct{}
}

func newRateLimiter(maxRequests int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		maxRequests: maxRequests,
		window:      window,
		requests:    make(map[string][]time.Time),
		stopClean:   make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	// Get existing requests and filter to window
	reqs := rl.requests[key]
	var valid []time.Time
	for _, t := range reqs {
		if t.After(windowStart) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.maxRequests {
		rl.requests[key] = valid
		return false
	}

	rl.requests[key] = append(valid, now)
	return true
}

// cleanupLoop periodically removes stale entries from the requests map
// to prevent unbounded memory growth from inactive sandboxes.
func (rl *rateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.window * 2)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopClean:
			return
		}
	}
}

func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	windowStart := time.Now().Add(-rl.window)
	for key, reqs := range rl.requests {
		var valid []time.Time
		for _, t := range reqs {
			if t.After(windowStart) {
				valid = append(valid, t)
			}
		}
		if len(valid) == 0 {
			delete(rl.requests, key)
		} else {
			rl.requests[key] = valid
		}
	}
}

func (rl *rateLimiter) stop() {
	close(rl.stopClean)
}

// auditLogger logs requests to a file with size-based rotation.
type auditLogger struct {
	path    string
	maxSize int64 // max file size in bytes before rotation (0 = no limit)
	file    *os.File
	enc     *json.Encoder
	size    int64
	mu      sync.Mutex
	logger  *slog.Logger
}

const (
	defaultAuditMaxSize = 50 * 1024 * 1024 // 50 MiB
	auditKeepFiles      = 3                 // keep current + 3 rotated files
)

type auditEntry struct {
	Timestamp   time.Time     `json:"timestamp"`
	Duration    time.Duration `json:"duration_ns"`
	Sandbox     string        `json:"sandbox,omitempty"`
	Method      string        `json:"method"`
	Path        string        `json:"path"`
	StatusCode  int           `json:"status_code"`
	RequestSize int64         `json:"request_size"`
	RemoteAddr  string        `json:"remote_addr"`
}

func newAuditLogger(path string) (*auditLogger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	info, _ := f.Stat()
	var size int64
	if info != nil {
		size = info.Size()
	}
	return &auditLogger{
		path:    path,
		maxSize: defaultAuditMaxSize,
		file:    f,
		enc:     json.NewEncoder(f),
		size:    size,
		logger:  slog.Default(),
	}, nil
}

func (al *auditLogger) log(entry auditEntry) {
	al.mu.Lock()
	defer al.mu.Unlock()

	if err := al.enc.Encode(entry); err != nil {
		al.logger.Warn("audit log write failed", "error", err)
		return
	}

	// Approximate size tracking (exact size not critical)
	al.size += 256 // average entry size estimate
	if al.maxSize > 0 && al.size >= al.maxSize {
		al.rotate()
	}
}

func (al *auditLogger) rotate() {
	al.file.Close()

	// Shift existing rotated files: .3 -> deleted, .2 -> .3, .1 -> .2, current -> .1
	for i := auditKeepFiles; i > 0; i-- {
		old := fmt.Sprintf("%s.%d", al.path, i)
		if i == auditKeepFiles {
			os.Remove(old)
		}
		if i > 1 {
			prev := fmt.Sprintf("%s.%d", al.path, i-1)
			os.Rename(prev, old)
		} else {
			os.Rename(al.path, old)
		}
	}

	f, err := os.OpenFile(al.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		al.logger.Warn("audit log rotation failed", "error", err)
		return
	}
	al.file = f
	al.enc = json.NewEncoder(f)
	al.size = 0
}

func (al *auditLogger) close() error {
	return al.file.Close()
}

// Server wraps the proxy with lifecycle management
type Server struct {
	proxy  *Proxy
	server *http.Server
}

// NewServer creates a new proxy server
func NewServer(cfg *Config) (*Server, error) {
	proxy, err := New(cfg)
	if err != nil {
		return nil, err
	}

	// Load API keys initially
	if err := proxy.LoadAPIKeys(); err != nil {
		cfg.Logger.Warn("failed to load API keys", "error", err)
	}

	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      proxy,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second, // Longer for streaming responses
		IdleTimeout:  60 * time.Second,
	}

	return &Server{
		proxy:  proxy,
		server: server,
	}, nil
}

// Start starts the proxy server
func (s *Server) Start() error {
	s.proxy.config.Logger.Info("starting proxy server", "addr", s.server.Addr)
	return s.server.ListenAndServe()
}

// Stop stops the proxy server
func (s *Server) Stop() error {
	if err := s.server.Close(); err != nil {
		return err
	}
	return s.proxy.Close()
}

// ReloadKeys reloads API keys from disk
func (s *Server) ReloadKeys() error {
	return s.proxy.LoadAPIKeys()
}
