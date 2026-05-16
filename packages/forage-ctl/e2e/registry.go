//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// VMInfo holds metadata about a running E2E VM.
type VMInfo struct {
	ID        string    `json:"id"`
	PID       int       `json:"pid"`
	SSHPort   int       `json:"ssh_port"`
	StartTime time.Time `json:"start_time"`
	TmpDir    string    `json:"tmp_dir"`
	GitBranch string    `json:"git_branch,omitempty"`
}

// registryDir returns the directory for VM state files.
func registryDir() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "forage-e2e")
	}
	return fmt.Sprintf("/tmp/forage-e2e-vms-%d", os.Getuid())
}

// Register records a running VM in the registry.
func Register(info VMInfo) error {
	dir := registryDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, info.ID+".json"), data, 0600)
}

// Deregister removes a VM from the registry.
func Deregister(id string) {
	os.Remove(filepath.Join(registryDir(), id+".json"))
}

// ListVMs returns all registered VMs, garbage-collecting stale entries.
func ListVMs() ([]VMInfo, error) {
	dir := registryDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var vms []VMInfo
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		var info VMInfo
		if err := json.Unmarshal(data, &info); err != nil {
			continue
		}

		// Check if process is still alive
		proc, err := os.FindProcess(info.PID)
		if err != nil {
			// Process doesn't exist, clean up stale entry
			os.Remove(filepath.Join(dir, entry.Name()))
			continue
		}

		if err := proc.Signal(syscall.Signal(0)); err != nil {
			// Process is dead, clean up stale entry
			os.Remove(filepath.Join(dir, entry.Name()))
			continue
		}

		vms = append(vms, info)
	}

	return vms, nil
}

// KillVM terminates a VM by its registry ID.
func KillVM(id string) error {
	dir := registryDir()
	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return fmt.Errorf("vm %s not found in registry: %w", id, err)
	}

	var info VMInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return fmt.Errorf("parse registry entry: %w", err)
	}

	proc, err := os.FindProcess(info.PID)
	if err != nil {
		Deregister(id)
		return fmt.Errorf("process %d not found: %w", info.PID, err)
	}

	// SIGTERM first
	log.Printf("sending SIGTERM to VM %s (PID %d)", id, info.PID)
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		Deregister(id)
		return fmt.Errorf("sigterm: %w", err)
	}

	// Wait up to 15s
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			break // Process exited
		}
		time.Sleep(500 * time.Millisecond)
	}

	// SIGKILL if still alive
	if err := proc.Signal(syscall.Signal(0)); err == nil {
		log.Printf("VM %s did not exit, sending SIGKILL", id)
		proc.Signal(syscall.SIGKILL)
		time.Sleep(time.Second)
	}

	// Clean up tmpdir
	if info.TmpDir != "" {
		os.RemoveAll(info.TmpDir)
	}

	Deregister(id)
	return nil
}
