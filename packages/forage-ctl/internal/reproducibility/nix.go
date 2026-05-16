package reproducibility

import (
	"context"
	"fmt"
	"strings"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/injection"
)

// NixReproducibility implements Reproducibility using Nix.
type NixReproducibility struct {
	// NixpkgsPath is the path to the nixpkgs input (optional, for pinning).
	NixpkgsPath string
}

// NewNixReproducibility creates a new NixReproducibility instance.
func NewNixReproducibility() *NixReproducibility {
	return &NixReproducibility{}
}

// StoreMount returns the /nix/store mount.
func (n *NixReproducibility) StoreMount() injection.Mount {
	return injection.Mount{
		HostPath:      "/nix/store",
		ContainerPath: "/nix/store",
		ReadOnly:      true,
	}
}

// BasePackages returns the minimal set of packages for any sandbox.
func (n *NixReproducibility) BasePackages() []injection.Package {
	return []injection.Package{
		{Name: "git"},
		{Name: "jujutsu"},
		{Name: "neovim"},
		{Name: "ripgrep"},
		{Name: "fd"},
	}
}

// ResolvePackage resolves a Package to a Nix package expression.
// If Version is empty, returns "pkgs.{Name}".
// If Version is set, attempts to resolve to a versioned package.
func (n *NixReproducibility) ResolvePackage(pkg injection.Package) (string, error) {
	nixName := pkg.Name

	if pkg.Version == "" {
		return "pkgs." + nixName, nil
	}

	// Handle version pinning by constructing a versioned package reference.
	// This is a simplified approach - in practice, version pinning in Nix
	// is more complex and may require different strategies per package.
	versionedName := fmt.Sprintf("%s_%s", nixName, normalizeVersion(pkg.Version))
	return "pkgs." + versionedName, nil
}

// normalizeVersion normalizes a version string for use in Nix package names.
// Converts "0.21.0" to "0_21_0".
func normalizeVersion(version string) string {
	return strings.ReplaceAll(version, ".", "_")
}

// ContributeMounts returns the /nix/store mount.
func (n *NixReproducibility) ContributeMounts(ctx context.Context, req *injection.MountRequest) ([]injection.Mount, error) {
	return []injection.Mount{n.StoreMount()}, nil
}

// Ensure NixReproducibility implements Reproducibility and MountContributor.
var (
	_ Reproducibility            = (*NixReproducibility)(nil)
	_ injection.MountContributor = (*NixReproducibility)(nil)
)
