// Package proxy provides an HTTP proxy for API key injection and rate limiting.
//
// This package implements a reverse proxy that sits between sandboxes and
// external APIs (like the Anthropic API), injecting authentication and
// enforcing rate limits without exposing API keys inside containers.
//
// # Key Features
//
//   - API key injection: Keys stay on the host, never enter containers
//   - Per-sandbox rate limiting: Prevent runaway API usage
//   - Audit logging: Track all API requests for compliance
//   - Sandbox identification via X-Forage-Sandbox header
//
// # Configuration
//
//	cfg := &proxy.Config{
//	    ListenAddr:        ":8080",
//	    SecretsDir:        "/run/forage-secrets",
//	    TargetURL:         "https://api.anthropic.com",
//	    RateLimitRequests: 1000,
//	    RateLimitWindow:   time.Hour,
//	    AuditLogPath:      "/var/log/forage/proxy.log",
//	}
//
// # Running the Proxy
//
//	p, err := proxy.New(cfg)
//	if err != nil {
//	    return err
//	}
//	p.Start()  // Blocks, serving requests
//
// # How It Works
//
//  1. Sandbox sends request with X-Forage-Sandbox header
//  2. Proxy looks up API key for that sandbox
//  3. Proxy injects Authorization header
//  4. Request is forwarded to upstream API
//  5. Response is returned to sandbox
package proxy
