package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// tokenFileName is the file name for the stored Claude OAuth token.
	tokenFileName = "claude-oauth.json"

	// tokenExpiryDuration is how long a setup-token is valid (1 year).
	tokenExpiryDuration = 365 * 24 * time.Hour

	// tokenWarnThreshold triggers a warning when the token expires within this window.
	tokenWarnThreshold = 30 * 24 * time.Hour
)

// StoredToken represents a persisted OAuth token with metadata.
type StoredToken struct {
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// TokenStatus describes the state of a stored token.
type TokenStatus int

const (
	TokenMissing  TokenStatus = iota // no token file
	TokenExpired                     // token past expiry
	TokenExpiring                    // token valid but within warn threshold
	TokenValid                       // token valid with plenty of time
)

// TokenStore manages persistent Claude OAuth tokens on disk.
type TokenStore struct {
	dir string // directory containing the token file
}

// NewTokenStore creates a TokenStore rooted at the given state directory.
// Tokens are stored under <stateDir>/tokens/.
func NewTokenStore(stateDir string) *TokenStore {
	return &TokenStore{dir: filepath.Join(stateDir, "tokens")}
}

// Store persists a long-lived token to disk.
func (s *TokenStore) Store(token string) (*StoredToken, error) {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return nil, fmt.Errorf("create token directory: %w", err)
	}

	now := time.Now()
	st := &StoredToken{
		Token:     token,
		CreatedAt: now,
		ExpiresAt: now.Add(tokenExpiryDuration),
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal token: %w", err)
	}

	path := filepath.Join(s.dir, tokenFileName)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return nil, fmt.Errorf("write token file: %w", err)
	}

	return st, nil
}

// Load reads the stored token from disk. Returns nil if no token exists.
func (s *TokenStore) Load() (*StoredToken, error) {
	path := filepath.Join(s.dir, tokenFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read token file: %w", err)
	}

	var st StoredToken
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parse token file: %w", err)
	}

	return &st, nil
}

// Remove deletes the stored token.
func (s *TokenStore) Remove() error {
	path := filepath.Join(s.dir, tokenFileName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove token file: %w", err)
	}
	return nil
}

// Status checks the current token state.
func (s *TokenStore) Status() (TokenStatus, *StoredToken, error) {
	st, err := s.Load()
	if err != nil {
		return TokenMissing, nil, err
	}
	if st == nil {
		return TokenMissing, nil, nil
	}

	now := time.Now()
	if now.After(st.ExpiresAt) {
		return TokenExpired, st, nil
	}
	if st.ExpiresAt.Sub(now) < tokenWarnThreshold {
		return TokenExpiring, st, nil
	}
	return TokenValid, st, nil
}

// Token returns the access token string if valid, or empty string with a
// human-readable reason if not.
func (s *TokenStore) Token() (string, string) {
	status, st, err := s.Status()
	if err != nil {
		return "", fmt.Sprintf("failed to read token: %v", err)
	}

	switch status {
	case TokenMissing:
		return "", "no token stored"
	case TokenExpired:
		return "", fmt.Sprintf("token expired on %s", st.ExpiresAt.Format("2006-01-02"))
	case TokenExpiring:
		remaining := time.Until(st.ExpiresAt)
		days := int(remaining.Hours() / 24)
		return st.Token, fmt.Sprintf("token expires in %d days", days)
	case TokenValid:
		return st.Token, ""
	}
	return "", "unknown token state"
}
