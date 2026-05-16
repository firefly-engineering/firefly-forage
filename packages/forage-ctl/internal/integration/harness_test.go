package integration

import (
	"testing"
)

func TestDefaultTemplate(t *testing.T) {
	tmpl := DefaultTemplate()

	if tmpl.Name != "integration-test" {
		t.Errorf("Name = %q, want %q", tmpl.Name, "integration-test")
	}
	if tmpl.Network != "none" {
		t.Errorf("Network = %q, want %q", tmpl.Network, "none")
	}
	if _, ok := tmpl.Agents["test"]; !ok {
		t.Error("Template should have 'test' agent")
	}
}
