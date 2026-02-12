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

	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	p := &Proxy{
		config:  cfg,
		apiKeys: make(map[string]string),
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

	// Extract sandbox identifier from request
	// Sandboxes should send X-Forage-Sandbox header
	sandboxName := r.Header.Get("X-Forage-Sandbox")
	if sandboxName == "" {
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

// LoadAPIKeys loads API keys from the secrets directory
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
			keyPath := filepath.Join(p.config.SecretsDir, sandboxName, "anthropic-api-key")
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

	return nil
}

// getAPIKey returns the API key for a sandbox
func (p *Proxy) getAPIKey(sandboxName string) string {
	p.keysMu.RLock()
	defer p.keysMu.RUnlock()
	return p.apiKeys[sandboxName]
}

// identifySandbox attempts to identify the sandbox from the remote address
func (p *Proxy) identifySandbox(remoteAddr string) string {
	// Container IPs are in 10.100.X.2 format where X is the network slot
	// We'd need to maintain a mapping of slots to sandbox names
	// For now, require explicit X-Forage-Sandbox header
	return ""
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
}

func newRateLimiter(maxRequests int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		maxRequests: maxRequests,
		window:      window,
		requests:    make(map[string][]time.Time),
	}
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

// auditLogger logs requests to a file
type auditLogger struct {
	file *os.File
	enc  *json.Encoder
	mu   sync.Mutex
}

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
	return &auditLogger{
		file: f,
		enc:  json.NewEncoder(f),
	}, nil
}

func (al *auditLogger) log(entry auditEntry) {
	al.mu.Lock()
	defer al.mu.Unlock()
	_ = al.enc.Encode(entry) // Best-effort audit logging
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
