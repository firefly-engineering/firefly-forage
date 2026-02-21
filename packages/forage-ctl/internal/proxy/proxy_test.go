package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProxy_AuthInjection(t *testing.T) {
	// Create a test upstream server that verifies headers
	var receivedAPIKey string
	var receivedAuth string
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAPIKey = r.Header.Get("X-Api-Key")
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok": true}`))
	}))
	defer upstream.Close()

	// Create secrets directory with test API key
	tmpDir := t.TempDir()
	sandboxDir := filepath.Join(tmpDir, "test-sandbox")
	if err := os.MkdirAll(sandboxDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sandboxDir, "anthropic-api-key"), []byte("sk-test-key-123"), 0600); err != nil {
		t.Fatal(err)
	}

	// Create proxy
	cfg := &Config{
		ListenAddr: ":0",
		SecretsDir: tmpDir,
		TargetURL:  upstream.URL,
		Transport:  upstream.Client().Transport,
	}
	proxy, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Load API keys
	if err := proxy.LoadAPIKeys(); err != nil {
		t.Fatal(err)
	}

	// Create test request with sandbox header
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"test": true}`))
	req.Header.Set("X-Forage-Sandbox", "test-sandbox")
	req.Header.Set("Content-Type", "application/json")

	// Execute
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	// Verify
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if receivedAPIKey != "sk-test-key-123" {
		t.Errorf("expected API key 'sk-test-key-123', got %q", receivedAPIKey)
	}
	// Authorization: Bearer should NOT be set (only X-Api-Key is used)
	if receivedAuth != "" {
		t.Errorf("expected no Authorization header, got %q", receivedAuth)
	}
}

func TestProxy_NoAPIKey(t *testing.T) {
	// Create a test upstream server
	var receivedAPIKey string
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAPIKey = r.Header.Get("X-Api-Key")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// Create proxy with empty secrets dir
	tmpDir := t.TempDir()
	cfg := &Config{
		ListenAddr: ":0",
		SecretsDir: tmpDir,
		TargetURL:  upstream.URL,
		Transport:  upstream.Client().Transport,
	}
	proxy, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Request without matching sandbox
	req := httptest.NewRequest("POST", "/v1/messages", nil)
	req.Header.Set("X-Forage-Sandbox", "unknown-sandbox")

	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	// Should forward without injection
	if receivedAPIKey != "" {
		t.Errorf("expected no API key, got %q", receivedAPIKey)
	}
}

func TestProxy_RateLimiting(t *testing.T) {
	// Create a test upstream server
	requestCount := 0
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// Create proxy with rate limiting
	cfg := &Config{
		ListenAddr:        ":0",
		SecretsDir:        t.TempDir(),
		TargetURL:         upstream.URL,
		Transport:         upstream.Client().Transport,
		RateLimitRequests: 3,
		RateLimitWindow:   time.Minute,
	}
	proxy, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Make requests up to limit
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Forage-Sandbox", "test-sandbox")
		w := httptest.NewRecorder()
		proxy.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, w.Code)
		}
	}

	// Next request should be rate limited
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forage-Sandbox", "test-sandbox")
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}

	// Different sandbox should not be rate limited
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forage-Sandbox", "other-sandbox")
	w = httptest.NewRecorder()
	proxy.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for different sandbox, got %d", w.Code)
	}
}

func TestProxy_AuditLogging(t *testing.T) {
	// Create a test upstream server
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// Create proxy with audit logging
	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")
	cfg := &Config{
		ListenAddr:   ":0",
		SecretsDir:   tmpDir,
		TargetURL:    upstream.URL,
		Transport:    upstream.Client().Transport,
		AuditLogPath: auditPath,
	}
	proxy, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Make a request
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader("test body"))
	req.Header.Set("X-Forage-Sandbox", "test-sandbox")
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	// Close proxy to flush logs
	proxy.Close()

	// Read audit log
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}

	var entry auditEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("failed to parse audit log: %v\ndata: %s", err, string(data))
	}

	if entry.Sandbox != "test-sandbox" {
		t.Errorf("expected sandbox 'test-sandbox', got %q", entry.Sandbox)
	}
	if entry.Method != "POST" {
		t.Errorf("expected method POST, got %q", entry.Method)
	}
	if entry.Path != "/v1/messages" {
		t.Errorf("expected path '/v1/messages', got %q", entry.Path)
	}
	if entry.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", entry.StatusCode)
	}
}

func TestProxy_SandboxHeaderRemoved(t *testing.T) {
	// Verify X-Forage-Sandbox header is not forwarded
	var receivedHeaders http.Header
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// Create proxy with API key
	tmpDir := t.TempDir()
	sandboxDir := filepath.Join(tmpDir, "test-sandbox")
	os.MkdirAll(sandboxDir, 0700)
	os.WriteFile(filepath.Join(sandboxDir, "anthropic-api-key"), []byte("sk-test"), 0600)

	cfg := &Config{
		ListenAddr: ":0",
		SecretsDir: tmpDir,
		TargetURL:  upstream.URL,
		Transport:  upstream.Client().Transport,
	}
	proxy, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	proxy.LoadAPIKeys()

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forage-Sandbox", "test-sandbox")
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if receivedHeaders.Get("X-Forage-Sandbox") != "" {
		t.Error("X-Forage-Sandbox header should not be forwarded")
	}
}

func TestRateLimiter_WindowExpiry(t *testing.T) {
	rl := newRateLimiter(2, 50*time.Millisecond)

	// Use up the limit
	if !rl.allow("test") {
		t.Error("first request should be allowed")
	}
	if !rl.allow("test") {
		t.Error("second request should be allowed")
	}
	if rl.allow("test") {
		t.Error("third request should be denied")
	}

	// Wait for window to expire
	time.Sleep(60 * time.Millisecond)

	// Should be allowed again
	if !rl.allow("test") {
		t.Error("request after window expiry should be allowed")
	}
}

func TestProxy_LoadAPIKeys(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple sandbox secrets
	for _, name := range []string{"sandbox-a", "sandbox-b", "sandbox-c"} {
		dir := filepath.Join(tmpDir, name)
		os.MkdirAll(dir, 0700)
		os.WriteFile(filepath.Join(dir, "anthropic-api-key"), []byte("key-"+name), 0600)
	}

	// Create a sandbox without the anthropic key
	os.MkdirAll(filepath.Join(tmpDir, "sandbox-no-key"), 0700)
	os.WriteFile(filepath.Join(tmpDir, "sandbox-no-key", "other-secret"), []byte("other"), 0600)

	cfg := &Config{
		ListenAddr: ":0",
		SecretsDir: tmpDir,
		TargetURL:  "https://api.example.com",
	}
	proxy, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if err := proxy.LoadAPIKeys(); err != nil {
		t.Fatal(err)
	}

	// Verify keys loaded
	tests := []struct {
		sandbox string
		wantKey string
	}{
		{"sandbox-a", "key-sandbox-a"},
		{"sandbox-b", "key-sandbox-b"},
		{"sandbox-c", "key-sandbox-c"},
		{"sandbox-no-key", ""},
		{"nonexistent", ""},
	}

	for _, tt := range tests {
		got := proxy.getAPIKey(tt.sandbox)
		if got != tt.wantKey {
			t.Errorf("getAPIKey(%q) = %q, want %q", tt.sandbox, got, tt.wantKey)
		}
	}
}

func TestProxy_UpstreamError(t *testing.T) {
	// Create an upstream that always errors
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "upstream error"}`))
	}))
	defer upstream.Close()

	cfg := &Config{
		ListenAddr: ":0",
		SecretsDir: t.TempDir(),
		TargetURL:  upstream.URL,
		Transport:  upstream.Client().Transport,
	}
	proxy, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestProxy_StreamingResponse(t *testing.T) {
	// Test that streaming responses work correctly
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("expected Flusher")
			return
		}
		for i := 0; i < 3; i++ {
			w.Write([]byte("data: test\n\n"))
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	cfg := &Config{
		ListenAddr: ":0",
		SecretsDir: t.TempDir(),
		TargetURL:  upstream.URL,
		Transport:  upstream.Client().Transport,
	}
	proxy, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/v1/messages", nil)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body, _ := io.ReadAll(w.Body)
	if !strings.Contains(string(body), "data: test") {
		t.Error("expected streaming data in response")
	}
}
