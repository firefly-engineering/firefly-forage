package cmd

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/proxy"
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Run the API proxy server",
	Long: `Run an HTTP proxy server that injects API keys into requests.

The proxy reads API keys from the secrets directory and injects them
into requests from sandboxes. This allows sandboxes to make API calls
without having direct access to the API keys.

Sandboxes should:
1. Set ANTHROPIC_BASE_URL to point to this proxy
2. Include X-Forage-Sandbox header with their sandbox name

The proxy will:
- Inject the appropriate API key for the sandbox
- Apply rate limiting (if configured)
- Log all requests to the audit log (if configured)

IMPORTANT: This only works for API key authentication. For Claude Max/Pro
plans using OAuth, authentication must be done inside the sandbox via
'claude login'. The proxy can still provide rate limiting and logging
for Max plans, but cannot inject authentication.`,
	RunE: runProxy,
}

var (
	proxyListen     string
	proxyTarget     string
	proxyRateLimit  int
	proxyRateWindow time.Duration
	proxyAuditLog   string
)

func init() {
	proxyCmd.Flags().StringVar(&proxyListen, "listen", ":8080", "Address to listen on")
	proxyCmd.Flags().StringVar(&proxyTarget, "target", "https://api.anthropic.com", "Upstream API URL")
	proxyCmd.Flags().IntVar(&proxyRateLimit, "rate-limit", 0, "Max requests per window (0 = unlimited)")
	proxyCmd.Flags().DurationVar(&proxyRateWindow, "rate-window", time.Minute, "Rate limit window duration")
	proxyCmd.Flags().StringVar(&proxyAuditLog, "audit-log", "", "Path to audit log file")
	rootCmd.AddCommand(proxyCmd)
}

func runProxy(cmd *cobra.Command, args []string) error {
	paths := config.DefaultPaths()

	cfg := &proxy.Config{
		ListenAddr:        proxyListen,
		SecretsDir:        paths.SecretsDir,
		TargetURL:         proxyTarget,
		RateLimitRequests: proxyRateLimit,
		RateLimitWindow:   proxyRateWindow,
		AuditLogPath:      proxyAuditLog,
		Logger:            logging.Logger,
	}

	server, err := proxy.NewServer(cfg)
	if err != nil {
		return err
	}

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logging.Info("shutting down proxy server")
		_ = server.Stop() // Best-effort shutdown
	}()

	// Handle SIGHUP for config reload
	hupCh := make(chan os.Signal, 1)
	signal.Notify(hupCh, syscall.SIGHUP)
	go func() {
		for range hupCh {
			logging.Info("reloading API keys")
			if err := server.ReloadKeys(); err != nil {
				logging.Warn("failed to reload keys", "error", err)
			}
		}
	}()

	logInfo("Starting API proxy server on %s", proxyListen)
	logInfo("Target: %s", proxyTarget)
	logInfo("Secrets: %s", paths.SecretsDir)
	if proxyRateLimit > 0 {
		logInfo("Rate limit: %d requests per %s", proxyRateLimit, proxyRateWindow)
	}
	if proxyAuditLog != "" {
		logInfo("Audit log: %s", proxyAuditLog)
	}

	return server.Start()
}
