package nixcache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKey_Deterministic(t *testing.T) {
	k1 := Key([]byte(`{"name":"test"}`), "/nix/store/abc-nixpkgs", 1000, 100, "24.11")
	k2 := Key([]byte(`{"name":"test"}`), "/nix/store/abc-nixpkgs", 1000, 100, "24.11")
	if k1 != k2 {
		t.Errorf("same inputs should produce same key: %s != %s", k1, k2)
	}
}

func TestKey_DifferentInputs(t *testing.T) {
	k1 := Key([]byte(`{"name":"test1"}`), "/nix/store/abc-nixpkgs", 1000, 100, "24.11")
	k2 := Key([]byte(`{"name":"test2"}`), "/nix/store/abc-nixpkgs", 1000, 100, "24.11")
	if k1 == k2 {
		t.Error("different template JSON should produce different keys")
	}
}

func TestKey_DifferentNixpkgs(t *testing.T) {
	k1 := Key([]byte(`{"name":"test"}`), "/nix/store/abc-nixpkgs", 1000, 100, "24.11")
	k2 := Key([]byte(`{"name":"test"}`), "/nix/store/def-nixpkgs", 1000, 100, "24.11")
	if k1 == k2 {
		t.Error("different nixpkgs path should produce different keys")
	}
}

func TestKey_DifferentUID(t *testing.T) {
	k1 := Key([]byte(`{"name":"test"}`), "/nix/store/abc", 1000, 100, "24.11")
	k2 := Key([]byte(`{"name":"test"}`), "/nix/store/abc", 1001, 100, "24.11")
	if k1 == k2 {
		t.Error("different UID should produce different keys")
	}
}

func TestCache_GetMiss(t *testing.T) {
	c := New(t.TempDir())
	got := c.Get("nonexistent")
	if got != "" {
		t.Errorf("expected empty string for cache miss, got %q", got)
	}
}

func TestCache_PutAndGet(t *testing.T) {
	cacheDir := t.TempDir()
	c := New(cacheDir)

	// Create a fake store path that exists on disk
	storePath := filepath.Join(t.TempDir(), "nix-store-fake")
	if err := os.MkdirAll(storePath, 0755); err != nil {
		t.Fatal(err)
	}

	key := "testkey123"
	if err := c.Put(key, storePath); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got := c.Get(key)
	if got != storePath {
		t.Errorf("Get after Put: got %q, want %q", got, storePath)
	}
}

func TestCache_GetStaleEntry(t *testing.T) {
	cacheDir := t.TempDir()
	c := New(cacheDir)

	// Store path that doesn't exist
	key := "stalekey"
	if err := c.Put(key, "/nix/store/nonexistent-path"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Should return empty because the store path doesn't exist
	got := c.Get(key)
	if got != "" {
		t.Errorf("expected empty string for stale entry, got %q", got)
	}

	// Entry should have been cleaned up
	if _, err := os.Stat(c.entryPath(key)); !os.IsNotExist(err) {
		t.Error("stale entry should have been removed from disk")
	}
}

func TestCache_Invalidate(t *testing.T) {
	cacheDir := t.TempDir()
	c := New(cacheDir)

	storePath := filepath.Join(t.TempDir(), "fake-store")
	if err := os.MkdirAll(storePath, 0755); err != nil {
		t.Fatal(err)
	}

	key := "invalidate-test"
	if err := c.Put(key, storePath); err != nil {
		t.Fatal(err)
	}

	c.Invalidate(key)

	got := c.Get(key)
	if got != "" {
		t.Errorf("expected empty after Invalidate, got %q", got)
	}
}
