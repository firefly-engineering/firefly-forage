// Package runtime provides a unified interface for container runtimes.
//
// Supported runtimes:
//   - nspawn: NixOS containers via extra-container (Linux)
//   - docker: Docker containers (Linux, macOS, Windows)
//   - apple: Apple Container (macOS 13+)
//
// Runtime selection is automatic based on platform and available tools.
// Use Global() to get the detected runtime, or construct specific
// implementations directly for testing.
//
// # Runtime Interface
//
// The Runtime interface defines operations common to all container backends:
//   - Create, Start, Stop, Destroy: Container lifecycle
//   - IsRunning, Status: Container state queries
//   - Exec, ExecInteractive: Command execution inside containers
//   - List: Enumerate all managed containers
//
// # SSH Runtime
//
// SSHRuntime extends Runtime for backends that provide SSH access to containers.
// This allows unified SSH-based access regardless of the underlying container
// technology. Methods include SSHPort, SSHExec, and SSHInteractive.
//
// # Mock Runtime
//
// For testing, use NewMockRuntime() to create a mock implementation that can
// be configured with expected responses and used to verify command execution.
package runtime
