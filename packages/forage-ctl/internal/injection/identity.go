package injection

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// IdentityContributor provides identity-related contributions.
// This includes SSH key mounts and git/jj config init commands.
type IdentityContributor struct {
	GitUser    string
	GitEmail   string
	SSHKeyPath string
	HomeDir    string // Container home directory
}

// NewIdentityContributor creates a new identity contributor.
func NewIdentityContributor(gitUser, gitEmail, sshKeyPath, homeDir string) *IdentityContributor {
	return &IdentityContributor{
		GitUser:    gitUser,
		GitEmail:   gitEmail,
		SSHKeyPath: sshKeyPath,
		HomeDir:    homeDir,
	}
}

// ContributeMounts returns SSH key mounts.
func (i *IdentityContributor) ContributeMounts(ctx context.Context, req *MountRequest) ([]Mount, error) {
	if i.SSHKeyPath == "" {
		return nil, nil
	}

	// Check that the key files exist
	if _, err := os.Stat(i.SSHKeyPath); err != nil {
		return nil, nil
	}
	pubKeyPath := i.SSHKeyPath + ".pub"
	if _, err := os.Stat(pubKeyPath); err != nil {
		return nil, nil
	}

	homeDir := i.HomeDir
	if homeDir == "" {
		homeDir = "/home/agent"
	}

	keyName := filepath.Base(i.SSHKeyPath)
	sshDir := filepath.Join(homeDir, ".ssh")

	return []Mount{
		{
			HostPath:      i.SSHKeyPath,
			ContainerPath: filepath.Join(sshDir, keyName),
			ReadOnly:      true,
		},
		{
			HostPath:      pubKeyPath,
			ContainerPath: filepath.Join(sshDir, keyName+".pub"),
			ReadOnly:      true,
		},
	}, nil
}

// ContributeTmpfilesRules returns tmpfiles rules for SSH directory.
func (i *IdentityContributor) ContributeTmpfilesRules(ctx context.Context, req *TmpfilesRequest) ([]string, error) {
	if i.SSHKeyPath == "" {
		return nil, nil
	}

	homeDir := i.HomeDir
	username := "agent"

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

	return []string{
		fmt.Sprintf("d %s/.ssh 0700 %s users -", homeDir, username),
	}, nil
}

// ContributePromptFragments returns identity information for prompts.
func (i *IdentityContributor) ContributePromptFragments(ctx context.Context) ([]PromptFragment, error) {
	if i.GitUser == "" && i.GitEmail == "" && i.SSHKeyPath == "" {
		return nil, nil
	}

	var content string
	if i.GitUser != "" || i.GitEmail != "" {
		content = "Git authorship is configured for this sandbox"
		if i.GitUser != "" {
			content += " as **" + i.GitUser + "**"
		}
		if i.GitEmail != "" {
			content += " <" + i.GitEmail + ">"
		}
		content += ". All commits will use this identity automatically."
	}
	if i.SSHKeyPath != "" {
		if content != "" {
			content += " "
		}
		content += "An SSH key is available for pushing to remote repositories. SSH is configured to use this key automatically for all hosts."
	}

	return []PromptFragment{{
		Section:  PromptSectionIdentity,
		Priority: 10,
		Content:  content,
	}}, nil
}

// Ensure IdentityContributor implements interfaces
var (
	_ MountContributor    = (*IdentityContributor)(nil)
	_ TmpfilesContributor = (*IdentityContributor)(nil)
	_ PromptContributor   = (*IdentityContributor)(nil)
)
