# Security

Forage provides isolation for AI agents, but it's important to understand the threat model and limitations.

## Threat Model

### Trusted

- Host system administrator
- Nix store contents (from nixpkgs/trusted sources)
- Forage module configuration

### Untrusted

- AI agent behavior
- Code being worked on in workspace
- Packages installed by agents at runtime

## Security Layers

### Container Isolation

Sandboxes use systemd-nspawn containers:

- Separate PID namespace
- Separate network namespace
- Separate mount namespace
- Resource limits (cgroups)
- Ephemeral root filesystem

### Filesystem Isolation

| Path | Access | Notes |
|------|--------|-------|
| `/` | Read-write | Ephemeral (tmpfs), lost on restart |
| `/nix/store` | Read-only | Shared from host |
| `/workspace` | Read-write | Bind-mounted from host |
| `/run/secrets` | Read-only | API keys and credentials |

Agents can only persistently modify files in `/workspace`.

### Network Isolation

| Mode | Description |
|------|-------------|
| `full` | Unrestricted internet access |
| `restricted` | Allowlist of specific hosts |
| `none` | No network access |

Even with `network = "none"`, containers can communicate with the nix daemon socket.

### Auth Obfuscation

API keys are:
- Stored in files, not environment variables
- Read at runtime by wrapper scripts
- Set only for the agent process

This makes casual credential discovery harder, but doesn't prevent a determined agent from reading `/run/secrets/`.

## Mitigations

| Threat | Mitigation |
|--------|------------|
| Agent exfiltrates API keys | API proxy (keeps secrets on host); obfuscation via wrappers (UX convenience, not a security boundary) |
| Agent accesses host filesystem | Container isolation, explicit bind mounts only |
| Agent makes unwanted network calls | Network isolation modes |
| Agent runs dangerous commands | Permission rules (`allow`/`deny`) via managed settings |
| Agent corrupts system state | Ephemeral root, easy reset |
| Agent fills disk | Ephemeral tmpfs has size limits |
| Agent escapes container | systemd-nspawn security features |

## Limitations

### Auth Obfuscation Is Not Foolproof

A determined agent could:
- Read files in `/run/secrets/` directly
- Inspect its own process memory
- Intercept API calls

Wrappers provide obfuscation, not security. They stop casual discovery, not intentional exfiltration.

### Container Escape Vulnerabilities

systemd-nspawn is not a security boundary like a VM. Kernel vulnerabilities could allow container escape. For high-security scenarios, consider:
- Running sandboxes in VMs
- Additional seccomp filtering
- SELinux/AppArmor policies

### DNS Resolution Timing

In `restricted` mode, allowed host IPs are resolved at sandbox creation time and baked into nftables rules. If a host's IPs change (e.g., CDN rotation), the rules become stale and connectivity may break until the sandbox is reconfigured with `forage-ctl network`.

### Network Exfiltration

Even with `network = "none"`, agents could potentially:
- Encode data in DNS queries (if DNS is available)
- Use timing side channels
- Embed data in legitimate API calls

### Workspace Access

Agents have full read-write access to `/workspace`. They could:
- Modify or delete project files
- Read sensitive files in the project
- Create files that execute on the host

## Best Practices

### Secret Management

```nix
# Use proper secret management (sops-nix, agenix)
secrets = {
  anthropic = config.sops.secrets.anthropic-api-key.path;
};

# Don't hardcode secrets
# BAD: secrets = { anthropic = "/home/user/.secrets/key"; };
```

### Template Design

```nix
# Minimize installed packages
extraPackages = with pkgs; [ ripgrep fd ];
# Don't include: curl, wget, netcat, etc. unless needed

# Use network isolation when possible
network = "none";  # For tasks that don't need network

# Use granular permissions instead of skipAll when possible
agents.claude.permissions = {
  allow = [ "Read" "Glob" "Grep" "Edit(src/**)" ];
  deny = [ "Bash(rm -rf *)" ];
};
```

### Agent Permissions

Use the most restrictive permissions that still allow the agent to do its job:

- Prefer granular `allow`/`deny` over `skipAll`
- Use `deny` rules to block dangerous patterns even when allowing broad tool access
- `skipAll` is convenient for trusted development workflows but grants full tool access

### Workspace Hygiene

- Don't put sensitive files (SSH keys, credentials) in project directories
- Use `.gitignore` / `.jjignore` to exclude sensitive patterns
- Review agent-created files before committing

### Regular Resets

```bash
# Reset sandbox periodically to clear accumulated state
forage-ctl reset myproject
```

### Monitor Agent Activity

- Review files modified by agents
- Check git/jj history for unexpected changes
- Monitor network traffic if concerned

## Additional Security Features

### API Proxy

The `forage-ctl proxy` command starts an HTTP proxy that:
- Keeps secrets on the host, never in containers
- Injects API keys into requests at runtime
- Can log all API calls for audit
- Enables rate limiting and request filtering

## Future Security Enhancements

### Syscall Filtering

Additional seccomp profiles to restrict:
- Dangerous syscalls
- Network operations
- File operations outside allowed paths

### Read-Only Workspace Mode

For review tasks where the agent shouldn't modify files:

```nix
templates.review = {
  readOnlyWorkspace = true;
  # ...
};
```

This is implemented and enforces filesystem-level read-only mounting of `/workspace`.

## Reporting Security Issues

If you discover a security vulnerability in Forage, please report it responsibly:

1. Do not open a public issue
2. Email security concerns to the maintainers
3. Allow time for a fix before public disclosure

See the project repository for contact information.
