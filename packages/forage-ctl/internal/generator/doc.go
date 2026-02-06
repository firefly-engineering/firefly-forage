// Package generator provides Nix configuration generation for containers.
//
// This package generates the Nix expressions that define sandbox containers
// for use with extra-container. Generated configurations include network
// settings, bind mounts, SSH access, and agent-specific packages.
//
// # Container Configuration
//
// GenerateNixConfig creates a complete NixOS container configuration:
//
//	cfg := &generator.ContainerConfig{
//	    Name:           "my-sandbox",
//	    NetworkSlot:    1,
//	    Workspace:      "/path/to/workspace",
//	    SecretsPath:    "/run/secrets/my-sandbox",
//	    AuthorizedKeys: []string{"ssh-ed25519 ..."},
//	    Template:       template,
//	    HostConfig:     hostConfig,
//	    UID:            1000,
//	    GID:            100,
//	}
//
//	nixExpr := generator.GenerateNixConfig(cfg)
//
// # Generated Features
//
// The generated configuration includes:
//   - Private networking with NAT (10.100.X.0/24 subnets)
//   - SSH access with key authentication
//   - Workspace bind mount at /workspace
//   - Nix store shared read-only from host
//   - Secrets mounted at /run/secrets
//   - tmux session auto-started for agents
//   - Network firewall rules based on template settings
//
// # Skills Generation
//
// GenerateSkills creates markdown documentation for agents running in the
// sandbox, explaining the environment, available tools, and guidelines.
package generator
