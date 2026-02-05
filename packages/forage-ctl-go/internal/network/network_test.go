package network

import (
	"strings"
	"testing"
)

func TestResolveHosts(t *testing.T) {
	// Test with a well-known host
	resolved, err := ResolveHosts([]string{"localhost"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved host, got %d", len(resolved))
	}

	if resolved[0].Hostname != "localhost" {
		t.Errorf("expected hostname 'localhost', got %q", resolved[0].Hostname)
	}

	// localhost should resolve to 127.0.0.1 or ::1
	if len(resolved[0].IPs) == 0 {
		t.Error("expected at least one IP for localhost")
	}
}

func TestResolveHosts_UnresolvableHost(t *testing.T) {
	// Test with an unresolvable host - should not error
	resolved, err := ResolveHosts([]string{"this-host-does-not-exist-12345.invalid"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved host, got %d", len(resolved))
	}

	// Should have hostname but no IPs
	if resolved[0].Hostname != "this-host-does-not-exist-12345.invalid" {
		t.Errorf("unexpected hostname: %q", resolved[0].Hostname)
	}
}

func TestGenerateNftablesRules_RestrictedMode(t *testing.T) {
	cfg := &Config{
		Mode:         ModeRestricted,
		AllowedHosts: []string{"api.anthropic.com", "github.com"},
		NetworkSlot:  5,
	}

	rules := GenerateNftablesRules(cfg)

	// Check for expected elements
	expectedStrings := []string{
		"flush ruleset",
		"table inet filter",
		"set allowed_ipv4",
		"set allowed_ipv6",
		"10.100.5.1", // Gateway IP
		"127.0.0.1",
		"::1",
		"chain output",
		"policy drop",
		"oif \"lo\" accept",
		"ct state established,related accept",
		"@allowed_ipv4 accept",
		"@allowed_ipv6 accept",
		"reject with icmp type admin-prohibited",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(rules, expected) {
			t.Errorf("expected rules to contain %q", expected)
		}
	}
}

func TestGenerateNftablesRules_NonRestrictedMode(t *testing.T) {
	// Full mode should return empty string
	cfg := &Config{
		Mode:        ModeFull,
		NetworkSlot: 5,
	}

	rules := GenerateNftablesRules(cfg)
	if rules != "" {
		t.Errorf("expected empty rules for full mode, got %q", rules)
	}

	// None mode should also return empty string
	cfg.Mode = ModeNone
	rules = GenerateNftablesRules(cfg)
	if rules != "" {
		t.Errorf("expected empty rules for none mode, got %q", rules)
	}
}

func TestGenerateNftablesRules_NoAllowedHosts(t *testing.T) {
	cfg := &Config{
		Mode:         ModeRestricted,
		AllowedHosts: []string{},
		NetworkSlot:  5,
	}

	rules := GenerateNftablesRules(cfg)
	if rules != "" {
		t.Errorf("expected empty rules when no allowed hosts, got %q", rules)
	}
}

func TestGenerateDnsmasqConfig(t *testing.T) {
	allowedHosts := []string{"api.anthropic.com", "github.com", "*.openai.com"}

	config := GenerateDnsmasqConfig(allowedHosts)

	expectedStrings := []string{
		"no-resolv",
		"listen-address=127.0.0.1",
		"server=/api.anthropic.com/1.1.1.1",
		"server=/github.com/1.1.1.1",
		"server=/openai.com/1.1.1.1", // Wildcard domain
		"address=/#/",                // Block all other queries
		"cache-size=1000",
		"domain-needed",
		"bogus-priv",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(config, expected) {
			t.Errorf("expected config to contain %q", expected)
		}
	}
}

func TestGenerateNixNetworkConfig_Full(t *testing.T) {
	cfg := &Config{
		Mode:        ModeFull,
		NetworkSlot: 7,
	}

	config := GenerateNixNetworkConfig(cfg)

	expectedStrings := []string{
		"10.100.7.1",     // Gateway
		"1.1.1.1",        // DNS
		"8.8.8.8",        // DNS
		"defaultGateway", // Gateway setting
		"nameservers",    // DNS setting
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(config, expected) {
			t.Errorf("expected config to contain %q", expected)
		}
	}
}

func TestGenerateNixNetworkConfig_None(t *testing.T) {
	cfg := &Config{
		Mode:        ModeNone,
		NetworkSlot: 3,
	}

	config := GenerateNixNetworkConfig(cfg)

	expectedStrings := []string{
		"nameservers = []",
		"defaultGateway = null",
		"OUTPUT -j DROP",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(config, expected) {
			t.Errorf("expected config to contain %q", expected)
		}
	}

	// Should NOT have external DNS
	if strings.Contains(config, "1.1.1.1") || strings.Contains(config, "8.8.8.8") {
		t.Error("none mode should not have external DNS servers")
	}
}

func TestGenerateNixNetworkConfig_Restricted(t *testing.T) {
	cfg := &Config{
		Mode:         ModeRestricted,
		AllowedHosts: []string{"api.anthropic.com", "github.com"},
		NetworkSlot:  4,
	}

	config := GenerateNixNetworkConfig(cfg)

	expectedStrings := []string{
		"10.100.4.1",                      // Gateway
		"127.0.0.1",                       // Local DNS
		"services.dnsmasq",                // DNS filtering
		"server=/api.anthropic.com/",     // DNS forward rule
		"server=/github.com/",            // DNS forward rule
		"address = \"/#/\"",               // Block other DNS
		"networking.nftables",             // nftables
		"set allowed_ipv4",                // IP set
		"@allowed_ipv4 accept",            // Accept rule
		"reject with icmp type admin-prohibited",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(config, expected) {
			t.Errorf("expected config to contain %q\nconfig:\n%s", expected, config)
		}
	}
}

func TestGenerateNixNetworkConfig_RestrictedNoHosts(t *testing.T) {
	cfg := &Config{
		Mode:         ModeRestricted,
		AllowedHosts: []string{},
		NetworkSlot:  4,
	}

	config := GenerateNixNetworkConfig(cfg)

	// With no allowed hosts, restricted should behave like none
	if !strings.Contains(config, "OUTPUT -j DROP") {
		t.Error("restricted mode with no hosts should behave like none mode")
	}
}

func TestMode_String(t *testing.T) {
	tests := []struct {
		mode     Mode
		expected string
	}{
		{ModeFull, "full"},
		{ModeRestricted, "restricted"},
		{ModeNone, "none"},
	}

	for _, tt := range tests {
		if string(tt.mode) != tt.expected {
			t.Errorf("Mode %v: expected %q, got %q", tt.mode, tt.expected, string(tt.mode))
		}
	}
}
