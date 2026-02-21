# ADR 005: SSH Host Key Verification for Localhost Connections

## Status

Accepted

## Context

Firefly Forage uses SSH to connect to sandboxes running on localhost. Standard SSH security practice requires host key verification to prevent Man-in-the-Middle (MITM) attacks. However, sandbox containers are ephemeral and regenerate their SSH host keys on each creation.

The current implementation disables host key verification:
```go
StrictHostKeyChecking: false,
KnownHostsFile:        "/dev/null",
```

This decision requires documentation of the security trade-offs.

## Decision

Accept disabled host key verification for localhost-only sandbox connections with the following rationale:

### Threat Model Analysis

1. **Localhost-only scope**: All sandbox SSH connections target `localhost`. An attacker would need local system access to intercept these connections.

2. **Defense in depth**: If an attacker has local system access sufficient to perform a MITM attack on localhost SSH, they likely already have the ability to:
   - Read the sandbox secrets from `/run/forage-secrets/`
   - Access the container directly via the runtime
   - Modify the forage-ctl binary itself

3. **Ephemeral containers**: Sandboxes are designed to be short-lived. Host keys change on each container recreation, making traditional known_hosts management impractical.

4. **Port isolation**: Each sandbox uses a unique port, reducing the attack surface for port confusion attacks.

### Security Properties Maintained

- **Transport encryption**: SSH still encrypts all traffic
- **Client authentication**: Public key authentication prevents unauthorized access
- **Container isolation**: Network namespacing isolates containers from each other

### Mitigations

1. **Strict input validation**: Sandbox names are validated with allowlist regex
2. **Path traversal protection**: Config loading validates paths stay within base directories
3. **Port range enforcement**: Only configured port range is used

## Consequences

### Positive

- **Simplicity**: No host key management complexity for ephemeral containers
- **User experience**: No "host key changed" warnings when recreating sandboxes
- **Automation friendly**: Scripts don't need to handle known_hosts updates

### Negative

- **Reduced defense-in-depth**: One less security layer for localhost attacks
- **Security audit findings**: May be flagged in security reviews
- **Not extensible to remote**: This model cannot be extended to non-localhost connections

## Alternatives Considered

### 1. Per-sandbox known_hosts files

Store each sandbox's host key in a dedicated known_hosts file.

**Rejected because**: Adds complexity, host keys still change on container recreation, would require cleanup logic.

### 2. Pre-generated host keys in Nix configuration

Generate stable SSH host keys as part of the container configuration.

**Rejected because**: Storing private keys in Nix config (which goes to /nix/store) is a security anti-pattern. Keys would be world-readable.

### 3. Host key callback verification

Implement custom host key verification that accepts any key for localhost.

**Rejected because**: Adds complexity without meaningful security benefit given the threat model.

## Notes

If Firefly Forage is extended to support remote sandbox connections (not on localhost), a different security model will be required. This ADR only covers localhost connections.
