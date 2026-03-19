package agent

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
)

// keychainServiceName is the service name Claude Code uses for credential storage.
const keychainServiceName = "Claude Code-credentials"

// claudeCredentials holds the OAuth credential structure stored in the keychain.
type claudeCredentials struct {
	ClaudeAiOauth *oauthCredential `json:"claudeAiOauth,omitempty"`
}

type oauthCredential struct {
	AccessToken  string `json:"accessToken"`  //nolint:gosec // G117: this is a deserialized credential, not a hardcoded secret
	RefreshToken string `json:"refreshToken"` //nolint:gosec // G117: this is a deserialized credential, not a hardcoded secret
	ExpiresAt    int64  `json:"expiresAt"`    // milliseconds since epoch
}

// readOAuthToken reads the Claude OAuth access token from the host credential store.
// Returns the access token string, or empty string if unavailable or expired.
func readOAuthToken() string {
	if runtime.GOOS != "darwin" {
		// TODO: support Linux secret-service / libsecret
		return ""
	}

	token, err := readMacOSKeychainToken()
	if err != nil {
		logging.Debug("failed to read OAuth token from keychain", "error", err)
		return ""
	}
	return token
}

// readMacOSKeychainToken extracts the Claude OAuth access token from the macOS keychain.
func readMacOSKeychainToken() (string, error) {
	cmd := exec.Command("security", "find-generic-password",
		"-s", keychainServiceName,
		"-g",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("keychain lookup failed: %w", err)
	}

	// The password line looks like: password: "{ ... json ... }"
	password := extractKeychainPassword(string(out))
	if password == "" {
		return "", fmt.Errorf("no password found in keychain entry")
	}

	var creds claudeCredentials
	if err := json.Unmarshal([]byte(password), &creds); err != nil {
		return "", fmt.Errorf("failed to parse credentials JSON: %w", err)
	}

	if creds.ClaudeAiOauth == nil || creds.ClaudeAiOauth.AccessToken == "" {
		return "", fmt.Errorf("no OAuth access token in credentials")
	}

	// Check expiry (expiresAt is milliseconds since epoch)
	expiresAt := time.UnixMilli(creds.ClaudeAiOauth.ExpiresAt)
	if time.Now().After(expiresAt) {
		return "", fmt.Errorf("OAuth access token expired at %s", expiresAt)
	}

	remaining := time.Until(expiresAt)
	logging.Debug("read OAuth token from keychain", "expiresIn", remaining.Round(time.Minute))
	return creds.ClaudeAiOauth.AccessToken, nil
}

// extractKeychainPassword parses the password value from `security find-generic-password -g` output.
func extractKeychainPassword(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "password: \"") {
			// Strip prefix and trailing quote
			pw := strings.TrimPrefix(line, "password: \"")
			if len(pw) > 0 && pw[len(pw)-1] == '"' {
				pw = pw[:len(pw)-1]
			}
			return pw
		}
	}
	return ""
}
