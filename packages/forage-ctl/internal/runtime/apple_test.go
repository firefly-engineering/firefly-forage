package runtime

import (
	"fmt"
	"os/exec"
	"testing"
)

func TestCpuQuotaToFloat(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"200%", "2.0"},
		{"100%", "1.0"},
		{"50%", "0.5"},
		{"400%", "4.0"},
		{"150%", "1.5"},
		{"invalid", ""},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := cpuQuotaToFloat(tc.input)
			if result != tc.expected {
				t.Errorf("cpuQuotaToFloat(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestContainerImage(t *testing.T) {
	t.Run("default image", func(t *testing.T) {
		rt := &AppleRuntime{BinaryPath: "container"}
		if img := rt.containerImage(); img != defaultAppleImage {
			t.Errorf("expected %q, got %q", defaultAppleImage, img)
		}
	})

	t.Run("custom image", func(t *testing.T) {
		rt := &AppleRuntime{BinaryPath: "container", Image: "custom/image:v1"}
		if img := rt.containerImage(); img != "custom/image:v1" {
			t.Errorf("expected %q, got %q", "custom/image:v1", img)
		}
	})
}

func TestIsContainerNotFound(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			"nil error",
			nil,
			false,
		},
		{
			"non-cmd error",
			fmt.Errorf("some other error"),
			false,
		},
		{
			"not found stderr",
			&containerCmdError{Subcommand: "rm", ExitCode: 1, Stderr: "container not found", Err: &exec.ExitError{}},
			true,
		},
		{
			"no such container",
			&containerCmdError{Subcommand: "rm", ExitCode: 1, Stderr: "No such container: foo", Err: &exec.ExitError{}},
			true,
		},
		{
			"does not exist",
			&containerCmdError{Subcommand: "rm", ExitCode: 1, Stderr: "container does not exist", Err: &exec.ExitError{}},
			true,
		},
		{
			"exit 0 with not found text",
			&containerCmdError{Subcommand: "rm", ExitCode: 0, Stderr: "not found", Err: nil},
			false,
		},
		{
			"exit 1 with unrelated stderr",
			&containerCmdError{Subcommand: "rm", ExitCode: 1, Stderr: "permission denied", Err: &exec.ExitError{}},
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isContainerNotFound(tc.err)
			if result != tc.expected {
				t.Errorf("isContainerNotFound(%v) = %v, want %v", tc.err, result, tc.expected)
			}
		})
	}
}

func TestAppleRuntime_Name(t *testing.T) {
	rt := &AppleRuntime{BinaryPath: "container"}
	if rt.Name() != "apple" {
		t.Errorf("expected 'apple', got %q", rt.Name())
	}
}

func TestAppleRuntime_ContainerInfo(t *testing.T) {
	rt := &AppleRuntime{BinaryPath: "container"}
	info := rt.ContainerInfo()
	if info.Username != "agent" {
		t.Errorf("expected username 'agent', got %q", info.Username)
	}
	if info.HomeDir != "/home/agent" {
		t.Errorf("expected home '/home/agent', got %q", info.HomeDir)
	}
	if info.WorkspaceDir != "/workspace" {
		t.Errorf("expected workspace '/workspace', got %q", info.WorkspaceDir)
	}
}

func TestAppleRuntime_containerName(t *testing.T) {
	t.Run("fallback to prefix", func(t *testing.T) {
		rt := &AppleRuntime{ContainerPrefix: "forage-"}
		name := rt.containerName("myproject")
		if name != "forage-myproject" {
			t.Errorf("expected 'forage-myproject', got %q", name)
		}
	})
}
