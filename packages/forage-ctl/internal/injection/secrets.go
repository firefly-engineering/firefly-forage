package injection

import (
	"context"
)

// SecretsContributor provides the /run/secrets mount.
type SecretsContributor struct {
	SecretsPath string // Host path to secrets directory
}

// NewSecretsContributor creates a new secrets contributor.
func NewSecretsContributor(secretsPath string) *SecretsContributor {
	return &SecretsContributor{
		SecretsPath: secretsPath,
	}
}

// ContributeMounts returns the secrets mount.
func (s *SecretsContributor) ContributeMounts(ctx context.Context, req *MountRequest) ([]Mount, error) {
	if s.SecretsPath == "" {
		return nil, nil
	}

	return []Mount{{
		HostPath:      s.SecretsPath,
		ContainerPath: "/run/secrets",
		ReadOnly:      true,
	}}, nil
}

// Ensure SecretsContributor implements MountContributor
var _ MountContributor = (*SecretsContributor)(nil)
