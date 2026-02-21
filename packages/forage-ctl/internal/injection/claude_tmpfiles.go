package injection

import (
	"context"
	"fmt"
)

// ClaudeTmpfilesContributor provides tmpfiles rules for Claude directories.
type ClaudeTmpfilesContributor struct {
	HomeDir  string
	Username string
}

// NewClaudeTmpfilesContributor creates a Claude tmpfiles contributor.
func NewClaudeTmpfilesContributor(homeDir, username string) *ClaudeTmpfilesContributor {
	return &ClaudeTmpfilesContributor{
		HomeDir:  homeDir,
		Username: username,
	}
}

// ContributeTmpfilesRules returns Claude-specific tmpfiles rules.
func (c *ClaudeTmpfilesContributor) ContributeTmpfilesRules(ctx context.Context, req *TmpfilesRequest) ([]string, error) {
	homeDir := c.HomeDir
	username := c.Username

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
		fmt.Sprintf("d %s/.claude 0755 %s users -", homeDir, username),
		fmt.Sprintf("d %s/.claude/commands 0755 %s users -", homeDir, username),
		fmt.Sprintf("d %s/.claude/skills 0755 %s users -", homeDir, username),
		// Also create the managed settings directory
		"d /etc/claude-code 0755 root root -",
	}, nil
}

// Ensure ClaudeTmpfilesContributor implements TmpfilesContributor
var _ TmpfilesContributor = (*ClaudeTmpfilesContributor)(nil)
