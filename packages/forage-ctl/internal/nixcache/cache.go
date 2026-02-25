// Package nixcache manages cached NixOS system store paths per template.
// It prevents repeated expensive NixOS evaluations when multiple sandboxes
// share the same template configuration.
package nixcache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
)

// Cache manages cached inner system store paths keyed by template configuration.
type Cache struct {
	// CacheDir is the base directory for cache files (e.g., /var/lib/firefly-forage/cache).
	CacheDir string
}

// New creates a new Cache with the given base directory.
func New(cacheDir string) *Cache {
	return &Cache{CacheDir: cacheDir}
}

// cacheEntry is the on-disk format for a cached store path.
type cacheEntry struct {
	StorePath string `json:"storePath"`
	CreatedAt string `json:"createdAt"`
}

// Key computes a cache key from the inputs that affect the inner system evaluation.
// Any change to these inputs should produce a different key.
func Key(templateJSON []byte, nixpkgsPath string, uid, gid int, stateVersion string) string {
	h := sha256.New()
	h.Write(templateJSON)
	h.Write([]byte(nixpkgsPath))
	fmt.Fprintf(h, "\nuid=%d\ngid=%d\nstateVersion=%s", uid, gid, stateVersion)
	return fmt.Sprintf("%x", h.Sum(nil))[:32]
}

// Get returns the cached system store path for the given key, or empty string
// if not cached or the cached path no longer exists on disk (e.g., GC'd).
func (c *Cache) Get(key string) string {
	entryPath := c.entryPath(key)
	data, err := os.ReadFile(entryPath)
	if err != nil {
		return ""
	}

	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		logging.Debug("nixcache: corrupt entry", "key", key, "error", err)
		return ""
	}

	// Verify the store path still exists (may have been garbage collected)
	if _, err := os.Stat(entry.StorePath); err != nil {
		logging.Debug("nixcache: store path gone", "key", key, "path", entry.StorePath)
		_ = os.Remove(entryPath)
		c.removeGCRoot(key)
		return ""
	}

	return entry.StorePath
}

// Put stores a system store path for the given key and creates a GC root
// to prevent nix garbage collection from removing the cached closure.
func (c *Cache) Put(key string, storePath string) error {
	if err := os.MkdirAll(filepath.Dir(c.entryPath(key)), 0755); err != nil {
		return fmt.Errorf("nixcache: failed to create cache dir: %w", err)
	}

	entry := cacheEntry{
		StorePath: storePath,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("nixcache: failed to marshal entry: %w", err)
	}

	if err := os.WriteFile(c.entryPath(key), data, 0644); err != nil {
		return fmt.Errorf("nixcache: failed to write entry: %w", err)
	}

	// Create a GC root so nix-collect-garbage doesn't remove the closure
	if err := c.createGCRoot(key, storePath); err != nil {
		logging.Warn("nixcache: failed to create GC root", "key", key, "error", err)
		// Non-fatal: cache still works, just at risk of GC
	}

	return nil
}

// Invalidate removes a cache entry and its GC root.
func (c *Cache) Invalidate(key string) {
	_ = os.Remove(c.entryPath(key))
	c.removeGCRoot(key)
}

func (c *Cache) entryPath(key string) string {
	return filepath.Join(c.CacheDir, "nixcache", key+".json")
}

func (c *Cache) gcRootPath(key string) string {
	return filepath.Join("/nix/var/nix/gcroots/auto", "forage-cache-"+key)
}

func (c *Cache) createGCRoot(key, storePath string) error {
	gcRootDir := filepath.Dir(c.gcRootPath(key))
	if err := os.MkdirAll(gcRootDir, 0755); err != nil {
		return err
	}

	linkPath := c.gcRootPath(key)
	// Remove existing symlink if present
	_ = os.Remove(linkPath)
	return os.Symlink(storePath, linkPath)
}

func (c *Cache) removeGCRoot(key string) {
	_ = os.Remove(c.gcRootPath(key))
}
