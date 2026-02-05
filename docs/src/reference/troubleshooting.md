# Troubleshooting

Common issues and their solutions.

## Installation Issues

### "Host configuration not found"

```
✗ Host configuration not found: /etc/firefly-forage/config.json
ℹ Is firefly-forage enabled in your NixOS configuration?
```

**Cause:** The Forage module isn't enabled or the system hasn't been rebuilt.

**Solution:**
```nix
services.firefly-forage.enable = true;
```

Then rebuild:
```bash
sudo nixos-rebuild switch
```

### "Templates directory not found"

```
✗ Templates directory not found: /etc/firefly-forage/templates
```

**Cause:** No templates are defined in the configuration.

**Solution:** Add at least one template:
```nix
services.firefly-forage.templates.claude = {
  agents.claude = { ... };
};
```

## Sandbox Creation Issues

### "Template not found"

```
✗ Template not found: mytemplate
```

**Cause:** The specified template doesn't exist.

**Solution:** List available templates:
```bash
forage-ctl templates
```

### "Workspace directory does not exist"

```
✗ Workspace directory does not exist: /path/to/project
```

**Cause:** The path doesn't exist or is misspelled.

**Solution:** Create the directory or check the path:
```bash
mkdir -p ~/projects/myproject
forage-ctl up myproject -t claude -w ~/projects/myproject
```

### "Not a jj repository"

```
✗ Not a jj repository: /path/to/repo
ℹ Initialize with: jj git init
```

**Cause:** Using `--repo` with a directory that isn't a JJ repository.

**Solution:** Initialize JJ:
```bash
cd /path/to/repo
jj git init --colocate
```

### "JJ workspace already exists"

```
✗ JJ workspace 'myname' already exists in /path/to/repo
```

**Cause:** A JJ workspace with that name already exists.

**Solution:** Use a different sandbox name, or remove the existing workspace:
```bash
jj workspace forget myname -R /path/to/repo
```

### "No available ports"

```
✗ No available ports in range 2200-2299
```

**Cause:** All ports in the configured range are in use.

**Solution:**
1. Remove unused sandboxes: `forage-ctl down <name>`
2. Increase the port range in configuration:
```nix
services.firefly-forage.portRange = {
  from = 2200;
  to = 2399;  # Expanded range
};
```

### "Failed to create container"

```
✗ Failed to create container
```

**Cause:** extra-container or systemd-nspawn failed.

**Solution:** Check system logs:
```bash
journalctl -u container@forage-myproject -n 50
```

Common causes:
- Insufficient permissions (run as root)
- Resource constraints
- Conflicting container names

## Connection Issues

### SSH Connection Refused

```
ssh: connect to host localhost port 2200: Connection refused
```

**Cause:** Container isn't running or SSH isn't ready.

**Solution:**
1. Check sandbox status:
```bash
forage-ctl ps
```

2. If stopped, the container may have failed. Check logs:
```bash
journalctl -u container@forage-myproject
```

3. Try resetting:
```bash
forage-ctl reset myproject
```

### SSH Timeout

```
ℹ Waiting for SSH to become available on port 2200...
✗ Timeout waiting for SSH (60s)
```

**Cause:** Container is starting slowly or SSH failed to start.

**Solution:** The container may still be starting. Wait and try:
```bash
forage-ctl ssh myproject
```

If it persists, check container logs:
```bash
machinectl status forage-myproject
journalctl -M forage-myproject -u sshd
```

### Permission Denied (SSH)

```
agent@localhost: Permission denied (publickey).
```

**Cause:** SSH key not authorized.

**Solution:** Ensure your key is in the configuration:
```nix
services.firefly-forage.authorizedKeys = [
  "ssh-ed25519 AAAA..."
];
```

Or use your user's keys:
```nix
services.firefly-forage.authorizedKeys =
  config.users.users.myuser.openssh.authorizedKeys.keys;
```

## Runtime Issues

### Agent Authentication Fails

```
Error: Invalid API key
```

**Cause:** Secret file is missing or has wrong content.

**Solution:**
1. Check the secret path in configuration
2. Verify the secret file exists and has correct content
3. Check sandbox secrets:
```bash
forage-ctl exec myproject -- cat /run/secrets/anthropic
```

### "Command not found" for Agent

```
bash: claude: command not found
```

**Cause:** Agent wrapper wasn't created or PATH issue.

**Solution:**
1. Check the template defines the agent correctly
2. Verify the package path exists:
```bash
forage-ctl exec myproject -- ls -la /nix/store/*claude*
```

### Workspace Permission Issues

```
Permission denied: /workspace/file
```

**Cause:** UID mismatch between container and host.

**Solution:** Ensure `services.firefly-forage.user` matches the owner of workspace files:
```nix
services.firefly-forage.user = "myuser";  # Owner of project files
```

### Nix Commands Fail

```
error: cannot open connection to remote store 'daemon'
```

**Cause:** Nix daemon socket not accessible.

**Solution:** This usually indicates a container configuration issue. Reset the sandbox:
```bash
forage-ctl reset myproject
```

## JJ Workspace Issues

### JJ Commands Fail Inside Sandbox

```
Error: There is no jj repo at the working directory
```

**Cause:** The `.jj` bind mount isn't working.

**Solution:**
1. Check the workspace has `.jj`:
```bash
forage-ctl exec myproject -- ls -la /workspace/.jj
```

2. The `.jj/repo` should be a symlink to the source repo. If broken, recreate the sandbox:
```bash
forage-ctl down myproject
forage-ctl up myproject -t claude --repo /path/to/repo
```

### Changes Not Visible Between Sandboxes

**This is expected behavior.** Each JJ workspace has an independent working copy. To share changes:

1. Commit in one sandbox:
```bash
# In sandbox-a
jj describe -m "My changes"
```

2. Update in another:
```bash
# In sandbox-b
jj status  # Will show changes from the shared repo
```

## Cleanup Issues

### Sandbox Won't Delete

```
forage-ctl down myproject
# Hangs or fails
```

**Solution:** Force cleanup:
```bash
# Stop container manually
sudo machinectl terminate forage-myproject

# Remove metadata
sudo rm /var/lib/firefly-forage/sandboxes/myproject.json

# Clean up secrets
sudo rm -rf /run/forage-secrets/myproject
```

### Orphaned JJ Workspace

If a sandbox was removed but the JJ workspace remains:

```bash
# List workspaces
jj workspace list -R /path/to/repo

# Remove orphan
jj workspace forget orphan-name -R /path/to/repo
rm -rf /var/lib/firefly-forage/workspaces/orphan-name
```

## Getting Help

If you can't resolve an issue:

1. Check the [GitHub issues](https://github.com/firefly-engineering/firefly-forage/issues)
2. Gather diagnostic information:
```bash
forage-ctl ps
journalctl -u container@forage-NAME -n 100
machinectl status forage-NAME
```
3. Open a new issue with the diagnostic output
