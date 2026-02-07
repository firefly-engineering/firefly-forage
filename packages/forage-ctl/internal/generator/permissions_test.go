package generator

import (
	"encoding/json"
	"testing"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

func TestClaudePermissionsPolicy_SkipAll(t *testing.T) {
	policy := &claudePermissionsPolicy{}

	data, err := policy.GenerateSettings(&config.AgentPermissions{SkipAll: true})
	if err != nil {
		t.Fatalf("GenerateSettings failed: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data for skipAll")
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	permsRaw, ok := result["permissions"]
	if !ok {
		t.Fatal("missing 'permissions' key")
	}

	var perms struct {
		Allow []string `json:"allow"`
		Deny  []string `json:"deny"`
	}
	if err := json.Unmarshal(permsRaw, &perms); err != nil {
		t.Fatalf("failed to parse permissions: %v", err)
	}

	// Should contain all tool families
	for _, tool := range claudeToolFamilies {
		found := false
		for _, a := range perms.Allow {
			if a == tool {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("skipAll should include %q in allow list", tool)
		}
	}

	if len(perms.Deny) > 0 {
		t.Errorf("skipAll should not have deny list, got %v", perms.Deny)
	}
}

func TestClaudePermissionsPolicy_Granular(t *testing.T) {
	policy := &claudePermissionsPolicy{}

	data, err := policy.GenerateSettings(&config.AgentPermissions{
		Allow: []string{"Read", "Glob", "Grep"},
		Deny:  []string{"Bash(rm -rf *)"},
	})
	if err != nil {
		t.Fatalf("GenerateSettings failed: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data for granular permissions")
	}

	var result struct {
		Permissions struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if len(result.Permissions.Allow) != 3 {
		t.Errorf("expected 3 allow entries, got %d", len(result.Permissions.Allow))
	}
	if len(result.Permissions.Deny) != 1 {
		t.Errorf("expected 1 deny entry, got %d", len(result.Permissions.Deny))
	}
	if result.Permissions.Deny[0] != "Bash(rm -rf *)" {
		t.Errorf("deny[0] = %q, want %q", result.Permissions.Deny[0], "Bash(rm -rf *)")
	}
}

func TestClaudePermissionsPolicy_Nil(t *testing.T) {
	policy := &claudePermissionsPolicy{}

	data, err := policy.GenerateSettings(nil)
	if err != nil {
		t.Fatalf("GenerateSettings failed: %v", err)
	}
	if data != nil {
		t.Errorf("expected nil data for nil permissions, got %s", string(data))
	}
}

func TestClaudePermissionsPolicy_EmptyPermissions(t *testing.T) {
	policy := &claudePermissionsPolicy{}

	data, err := policy.GenerateSettings(&config.AgentPermissions{})
	if err != nil {
		t.Fatalf("GenerateSettings failed: %v", err)
	}
	if data != nil {
		t.Errorf("expected nil data for empty permissions, got %s", string(data))
	}
}

func TestClaudePermissionsPolicy_ContainerSettingsPath(t *testing.T) {
	policy := &claudePermissionsPolicy{}

	path := policy.ContainerSettingsPath()
	if path != "/etc/claude-code/managed-settings.json" {
		t.Errorf("ContainerSettingsPath() = %q, want %q", path, "/etc/claude-code/managed-settings.json")
	}
}

func TestGetPermissionsPolicy(t *testing.T) {
	// Known agent
	policy, ok := GetPermissionsPolicy("claude")
	if !ok {
		t.Error("expected to find policy for 'claude'")
	}
	if policy == nil {
		t.Error("expected non-nil policy for 'claude'")
	}

	// Unknown agent
	_, ok = GetPermissionsPolicy("unknown-agent")
	if ok {
		t.Error("expected no policy for 'unknown-agent'")
	}
}
