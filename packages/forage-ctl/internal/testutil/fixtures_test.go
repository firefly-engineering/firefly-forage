package testutil

import (
	"testing"
)

func TestLoadValidHostConfig(t *testing.T) {
	cfg, err := ValidHostConfig()
	if err != nil {
		t.Fatalf("ValidHostConfig() error: %v", err)
	}

	if cfg.User != "testuser" {
		t.Errorf("User = %q, want %q", cfg.User, "testuser")
	}
	if len(cfg.AuthorizedKeys) == 0 {
		t.Error("AuthorizedKeys should not be empty")
	}
	if _, ok := cfg.Secrets["anthropic-api-key"]; !ok {
		t.Error("Secrets should contain anthropic-api-key")
	}

	// Validate should pass
	if err := cfg.Validate(); err != nil {
		t.Errorf("Valid config should pass validation: %v", err)
	}
}

func TestLoadInvalidHostConfig(t *testing.T) {
	cfg, err := InvalidHostConfig()
	if err != nil {
		t.Fatalf("InvalidHostConfig() error: %v", err)
	}

	// Validate should fail
	if err := cfg.Validate(); err == nil {
		t.Error("Invalid config should fail validation")
	}
}

func TestLoadValidTemplate(t *testing.T) {
	tmpl, err := ValidTemplate()
	if err != nil {
		t.Fatalf("ValidTemplate() error: %v", err)
	}

	if tmpl.Name != "claude" {
		t.Errorf("Name = %q, want %q", tmpl.Name, "claude")
	}
	if tmpl.Network != "full" {
		t.Errorf("Network = %q, want %q", tmpl.Network, "full")
	}
	if _, ok := tmpl.Agents["claude"]; !ok {
		t.Error("Agents should contain claude")
	}

	// Validate should pass
	if err := tmpl.Validate(); err != nil {
		t.Errorf("Valid template should pass validation: %v", err)
	}
}

func TestLoadValidSandboxMetadata(t *testing.T) {
	meta, err := ValidSandboxMetadata()
	if err != nil {
		t.Fatalf("ValidSandboxMetadata() error: %v", err)
	}

	if meta.Name != "test-sandbox" {
		t.Errorf("Name = %q, want %q", meta.Name, "test-sandbox")
	}
	if meta.Template != "claude" {
		t.Errorf("Template = %q, want %q", meta.Template, "claude")
	}
	if meta.NetworkSlot != 1 {
		t.Errorf("NetworkSlot = %d, want 1", meta.NetworkSlot)
	}
	if meta.WorkspaceMode != "jj" {
		t.Errorf("WorkspaceMode = %q, want %q", meta.WorkspaceMode, "jj")
	}

	// Validate should pass
	if err := meta.Validate(); err != nil {
		t.Errorf("Valid metadata should pass validation: %v", err)
	}
}

func TestLoadFixture_NotFound(t *testing.T) {
	_, err := LoadFixture("nonexistent.json")
	if err == nil {
		t.Error("LoadFixture should error for nonexistent file")
	}
}
