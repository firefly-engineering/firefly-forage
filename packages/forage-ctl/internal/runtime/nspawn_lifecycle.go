package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
)

// Container lifecycle constants for NixOS systemd-nspawn containers.
const (
	// mutableServicesDir is where dynamically-installed systemd units live on NixOS.
	// Requires boot.extraSystemdUnitPaths = [ "/etc/systemd-mutable/system" ].
	mutableServicesDir = "/etc/systemd-mutable/system"

	// containerConfigDir is where NixOS container .conf files are stored (NixOS >=22.05).
	containerConfigDir = "/etc/nixos-containers"

	// containerStateDir is where NixOS container root filesystems live.
	containerStateDir = "/var/lib/nixos-containers"

	// gcRootsDir is where nix GC roots are stored. We use gcroots/auto so that
	// stale links are automatically cleaned up by the nix garbage collector.
	gcRootsDir = "/nix/var/nix/gcroots/auto"
)

// installContainer installs a container from a pre-built /etc store path.
// It symlinks the systemd service and container conf files into the mutable
// directories, creates nix GC roots to prevent garbage collection, reloads
// the systemd daemon, and optionally starts the container.
func installContainer(ctx context.Context, etcPath string, start bool) error {
	nixosEtc := etcPath + "/etc"
	if _, err := os.Stat(nixosEtc); err != nil {
		return fmt.Errorf("%s doesn't exist", nixosEtc)
	}

	// Discover containers from the built etc
	containers, err := discoverContainers(nixosEtc)
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		return fmt.Errorf("no container services found in %s/systemd/system", nixosEtc)
	}

	// Ensure target directories exist
	for _, dir := range []string{
		mutableServicesDir,
		containerConfigDir,
		gcRootsDir,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}

	var installed []string
	for _, name := range containers {
		changed, err := installSingleContainer(nixosEtc, name)
		if err != nil {
			return fmt.Errorf("failed to install container %s: %w", name, err)
		}
		if changed {
			installed = append(installed, name)
		}
	}

	if len(installed) > 0 {
		if err := systemctlDaemonReload(ctx); err != nil {
			return err
		}
	}

	if start {
		for _, name := range containers {
			svc := "container@" + name + ".service"
			if err := sudoRun(ctx, "systemctl", "start", svc); err != nil {
				return fmt.Errorf("failed to start %s: %w", svc, err)
			}
		}
	}

	return nil
}

// installSingleContainer installs one container's service and conf files.
// Returns true if the container was installed (i.e., changed from current state).
func installSingleContainer(nixosEtc, name string) (bool, error) {
	svc := "container@" + name + ".service"
	serviceFile := filepath.Join(nixosEtc, "systemd/system", svc)
	serviceDest := filepath.Join(mutableServicesDir, svc)

	confFile := filepath.Join(nixosEtc, "nixos-containers", name+".conf")
	confDest := filepath.Join(containerConfigDir, name+".conf")

	// Check if unchanged (same realpath for both service and conf symlinks)
	if isUnchanged(serviceFile, serviceDest) && isUnchanged(confFile, confDest) {
		logging.Debug("container unchanged, skipped", "name", name)
		return false, nil
	}

	// Resolve to real paths for symlink targets
	realService, err := filepath.EvalSymlinks(serviceFile)
	if err != nil {
		return false, fmt.Errorf("failed to resolve service file: %w", err)
	}
	realConf, err := filepath.EvalSymlinks(confFile)
	if err != nil {
		// Try legacy path (NixOS <22.05 uses /etc/containers/)
		altConfFile := filepath.Join(nixosEtc, "containers", name+".conf")
		realConf, err = filepath.EvalSymlinks(altConfFile)
		if err != nil {
			return false, fmt.Errorf("container conf file not found: %w", err)
		}
	}

	// Create symlinks
	if err := forceSymlink(realService, serviceDest); err != nil {
		return false, fmt.Errorf("failed to symlink service: %w", err)
	}
	if err := forceSymlink(realConf, confDest); err != nil {
		return false, fmt.Errorf("failed to symlink conf: %w", err)
	}

	// Create GC roots to prevent nix garbage collection
	gcRoot := filepath.Join(gcRootsDir, "extra-container-"+name)
	if err := forceSymlink(serviceDest, gcRoot); err != nil {
		return false, fmt.Errorf("failed to create service GC root: %w", err)
	}
	if err := forceSymlink(confDest, gcRoot+".conf"); err != nil {
		return false, fmt.Errorf("failed to create conf GC root: %w", err)
	}

	logging.Debug("container installed", "name", name)
	return true, nil
}

// destroyContainer stops and fully removes a container.
func destroyContainer(ctx context.Context, name string) error {
	svc := "container@" + name + ".service"
	serviceFile := filepath.Join(mutableServicesDir, svc)
	confFile := filepath.Join(containerConfigDir, name+".conf")
	stateDir := filepath.Join(containerStateDir, name)

	// Stop the service (non-blocking so we can kill it right after)
	_ = sudoRun(ctx, "systemctl", "stop", "--no-block", svc)

	// Kill it to ensure the machine terminates
	_ = sudoRun(ctx, "systemctl", "kill", svc)

	// Remove service symlinks
	_ = os.Remove(filepath.Join(mutableServicesDir, "machines.target.wants", svc))
	if isSymlink(serviceFile) {
		_ = os.Remove(serviceFile)
	}

	// Remove GC roots
	gcRoot := filepath.Join(gcRootsDir, "extra-container-"+name)
	_ = os.Remove(gcRoot)
	_ = os.Remove(gcRoot + ".conf")

	// Remove conf file — nixos-container destroy needs a conf file to exist
	// (even a dummy one), otherwise it fails. So replace with an empty touch.
	_ = os.Remove(confFile)
	f, err := os.Create(confFile)
	if err == nil {
		_ = f.Close()
	}

	// Remove immutable attribute from nested container var/empty files
	// so the state directory can be deleted
	removeVarEmptyImmutable(stateDir)

	// Clean up the container state directory (root filesystem, etc.)
	// nixos-container does this but we do it directly for reliability.
	if _, err := os.Stat(stateDir); err == nil {
		_ = sudoRun(ctx, "rm", "-rf", stateDir)
	}

	// Clean up the dummy conf file
	_ = os.Remove(confFile)

	// Reload systemd so it forgets the removed units
	return systemctlDaemonReload(ctx)
}

// discoverContainers finds container names from service files in the etc output.
func discoverContainers(nixosEtc string) ([]string, error) {
	pattern := filepath.Join(nixosEtc, "systemd/system/container@*.service")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob container services: %w", err)
	}

	var names []string
	for _, match := range matches {
		base := filepath.Base(match)
		// container@foo.service -> foo
		name := strings.TrimPrefix(base, "container@")
		name = strings.TrimSuffix(name, ".service")
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// isUnchanged checks if src and dest point to the same real path.
func isUnchanged(src, dest string) bool {
	realSrc, err := filepath.EvalSymlinks(src)
	if err != nil {
		return false
	}
	realDest, err := filepath.EvalSymlinks(dest)
	if err != nil {
		return false
	}
	return realSrc == realDest
}

// isSymlink returns true if path is a symlink.
func isSymlink(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeSymlink != 0
}

// forceSymlink creates a symlink, removing any existing file at dest.
func forceSymlink(target, dest string) error {
	_ = os.Remove(dest)
	return os.Symlink(target, dest)
}

// removeVarEmptyImmutable removes the immutable attribute from var/empty
// directories inside nested containers, which would otherwise prevent
// deletion of the container state directory.
func removeVarEmptyImmutable(stateDir string) {
	pattern := filepath.Join(stateDir, "var/lib/*containers/*/var/empty")
	matches, _ := filepath.Glob(pattern)
	for _, m := range matches {
		// chattr -i — ignore errors (file may not exist or not be immutable)
		cmd := exec.Command("chattr", "-i", m)
		_ = cmd.Run()
	}
}

// systemctlDaemonReload tells systemd to reload its unit files.
func systemctlDaemonReload(ctx context.Context) error {
	return sudoRun(ctx, "systemctl", "daemon-reload")
}

// sudoRun executes a command via sudo.
func sudoRun(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "sudo", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w (output: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
