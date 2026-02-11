// Package reproducibility provides an abstraction over hermetic package
// management and store mounts. Currently backed by Nix, but abstracted
// for potential future alternatives.
package reproducibility

import (
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/injection"
)

// Reproducibility handles hermetic package installation and store mounts.
type Reproducibility interface {
	// StoreMount returns the mount for the package store (e.g., /nix/store).
	StoreMount() injection.Mount

	// ResolvePackage resolves a Package to an installable reference.
	// For Nix, this returns strings like "pkgs.git" or "pkgs.jujutsu_0_21_0".
	ResolvePackage(pkg injection.Package) (string, error)

	// BasePackages returns the minimal set of packages for any sandbox.
	BasePackages() []injection.Package
}
