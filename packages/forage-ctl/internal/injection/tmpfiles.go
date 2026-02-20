package injection

import (
	"context"
	"fmt"
)

// BaseTmpfilesContributor provides essential tmpfiles rules for sandboxes.
type BaseTmpfilesContributor struct {
	HomeDir  string // Container home directory (e.g., "/home/agent")
	Username string // Container username (e.g., "agent")
}

// NewBaseTmpfilesContributor creates a new base tmpfiles contributor.
func NewBaseTmpfilesContributor(homeDir, username string) *BaseTmpfilesContributor {
	return &BaseTmpfilesContributor{
		HomeDir:  homeDir,
		Username: username,
	}
}

// ContributeTmpfilesRules returns essential tmpfiles rules.
func (b *BaseTmpfilesContributor) ContributeTmpfilesRules(ctx context.Context, req *TmpfilesRequest) ([]string, error) {
	homeDir := b.HomeDir
	username := b.Username

	// Use request values if provided, falling back to configured values
	if req != nil {
		if req.HomeDir != "" {
			homeDir = req.HomeDir
		}
		if req.Username != "" {
			username = req.Username
		}
	}

	if homeDir == "" {
		homeDir = "/home/agent"
	}
	if username == "" {
		username = "agent"
	}

	return []string{
		fmt.Sprintf("d %s/.config 0755 %s users -", homeDir, username),
	}, nil
}

// Ensure BaseTmpfilesContributor implements TmpfilesContributor
var _ TmpfilesContributor = (*BaseTmpfilesContributor)(nil)
