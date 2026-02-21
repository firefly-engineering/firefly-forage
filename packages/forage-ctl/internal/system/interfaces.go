// Package system provides abstractions for OS operations to enable testing.
package system

import (
	"context"
	"io/fs"
	"os"
)

// FileSystem abstracts file system operations for testability.
type FileSystem interface {
	// ReadFile reads the named file and returns the contents.
	ReadFile(path string) ([]byte, error)

	// WriteFile writes data to the named file, creating it if necessary.
	WriteFile(path string, data []byte, perm fs.FileMode) error

	// Remove removes the named file or empty directory.
	Remove(path string) error

	// RemoveAll removes path and any children it contains.
	RemoveAll(path string) error

	// Stat returns file info for the named file.
	Stat(path string) (fs.FileInfo, error)

	// MkdirAll creates a directory named path, along with any necessary parents.
	MkdirAll(path string, perm fs.FileMode) error

	// Exists returns true if the path exists.
	Exists(path string) bool

	// IsDir returns true if the path is a directory.
	IsDir(path string) bool

	// ReadDir reads the named directory, returning all its directory entries.
	ReadDir(path string) ([]fs.DirEntry, error)

	// CopyFile copies a file from src to dst.
	CopyFile(src, dst string) error
}

// CommandExecutor abstracts command execution for testability.
type CommandExecutor interface {
	// Execute runs a command and returns its combined output.
	Execute(ctx context.Context, name string, args ...string) ([]byte, error)

	// ExecuteWithStdin runs a command with the given stdin and returns output.
	ExecuteWithStdin(ctx context.Context, stdin string, name string, args ...string) ([]byte, error)

	// ExecuteInteractive runs a command with stdin/stdout/stderr connected to the terminal.
	ExecuteInteractive(ctx context.Context, name string, args ...string) error

	// ReplaceProcess replaces the current process with the given command (exec syscall).
	ReplaceProcess(name string, args ...string) error
}

// Default instances using real OS operations.
var (
	defaultFS       FileSystem      = &osFileSystem{}
	defaultExecutor CommandExecutor = &osExecutor{}
)

// DefaultFS returns the default FileSystem implementation using real OS operations.
func DefaultFS() FileSystem {
	return defaultFS
}

// DefaultExecutor returns the default CommandExecutor implementation.
func DefaultExecutor() CommandExecutor {
	return defaultExecutor
}

// SetDefaultFS sets the default FileSystem (useful for testing).
func SetDefaultFS(fs FileSystem) {
	defaultFS = fs
}

// SetDefaultExecutor sets the default CommandExecutor (useful for testing).
func SetDefaultExecutor(exec CommandExecutor) {
	defaultExecutor = exec
}

// ResetDefaults restores the default OS implementations.
func ResetDefaults() {
	defaultFS = &osFileSystem{}
	defaultExecutor = &osExecutor{}
}

// osFileSystem implements FileSystem using real OS operations.
type osFileSystem struct{}

func (f *osFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (f *osFileSystem) WriteFile(path string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func (f *osFileSystem) Remove(path string) error {
	return os.Remove(path)
}

func (f *osFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (f *osFileSystem) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func (f *osFileSystem) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (f *osFileSystem) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (f *osFileSystem) IsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (f *osFileSystem) ReadDir(path string) ([]fs.DirEntry, error) {
	return os.ReadDir(path)
}

func (f *osFileSystem) CopyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, srcInfo.Mode())
}
