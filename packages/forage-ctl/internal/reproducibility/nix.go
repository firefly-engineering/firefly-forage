package reproducibility

import (
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
	// Normalize package name for Nix (replace hyphens with underscores in some cases)
	nixName := normalizeNixPackageName(pkg.Name)

	if pkg.Version == "" {
		return "pkgs." + nixName, nil
	}

	// Handle version pinning by constructing a versioned package reference.
	// This is a simplified approach - in practice, version pinning in Nix
	// is more complex and may require different strategies per package.
	versionedName := fmt.Sprintf("%s_%s", nixName, normalizeVersion(pkg.Version))
	return "pkgs." + versionedName, nil
}

// normalizeNixPackageName normalizes a package name for Nix.
// Most packages use their canonical names, but some need adjustment.
func normalizeNixPackageName(name string) string {
	// Special cases for packages with different Nix names
	switch name {
	case "jujutsu":
		return "jujutsu"
	case "ripgrep":
		return "ripgrep"
	case "fd":
		return "fd"
	default:
		return name
	}
}

// normalizeVersion normalizes a version string for use in Nix package names.
// Converts "0.21.0" to "0_21_0".
func normalizeVersion(version string) string {
	return strings.ReplaceAll(version, ".", "_")
}

// Ensure NixReproducibility implements Reproducibility.
var _ Reproducibility = (*NixReproducibility)(nil)
