# Shared option definitions for services.firefly-forage.
# Imported by both host.nix (NixOS) and darwin.nix (nix-darwin).
{ lib }:
let
  inherit (lib)
    mkEnableOption
    mkOption
    types
    ;

  # Agent definition type
  agentType = types.submodule {
    options = {
      package = mkOption {
        type = types.package;
        description = "The agent package to use";
      };

      secretName = mkOption {
        type = types.nullOr types.str;
        default = null;
        description = "Name of the secret (key in services.firefly-forage.secrets). Optional if using hostConfigDir for credentials.";
      };

      authEnvVar = mkOption {
        type = types.nullOr types.str;
        default = null;
        description = "Environment variable name for the auth token. Optional if using hostConfigDir for credentials.";
        example = "ANTHROPIC_API_KEY";
      };

      hostConfigDir = mkOption {
        type = types.nullOr types.str;
        default = null;
        description = "Host directory to mount for persistent agent configuration (supports ~ expansion)";
        example = "~/.claude";
      };

      containerConfigDir = mkOption {
        type = types.nullOr types.str;
        default = null;
        description = "Override container mount point (default: /home/<containerUsername>/.<dirname>)";
        example = "/home/agent/.claude";
      };

      hostConfigDirReadOnly = mkOption {
        type = types.bool;
        default = false;
        description = "Mount the config directory as read-only (default: false to allow token refresh)";
      };

      permissions = mkOption {
        type = types.nullOr (
          types.submodule {
            options = {
              skipAll = mkOption {
                type = types.bool;
                default = false;
                description = "Bypass all permission checks";
              };
              allow = mkOption {
                type = types.listOf types.str;
                default = [ ];
                description = "Permission rules to auto-approve (agent-specific format)";
                example = [
                  "Bash(npm run *)"
                  "Edit"
                  "Read"
                ];
              };
              deny = mkOption {
                type = types.listOf types.str;
                default = [ ];
                description = "Permission rules to always block";
                example = [ "Bash(rm -rf *)" ];
              };
            };
          }
        );
        default = null;
        description = "Agent permission rules. When null, no permission settings are generated.";
      };
    };
  };

  # Template definition type
  templateType = types.submodule {
    options = {
      description = mkOption {
        type = types.str;
        default = "";
        description = "Human-readable description of this template";
      };

      agents = mkOption {
        type = types.attrsOf agentType;
        default = { };
        description = "Agents available in this sandbox";
      };

      extraPackages = mkOption {
        type = types.listOf types.package;
        default = [ ];
        description = "Additional packages to include in the sandbox";
      };

      network = mkOption {
        type = types.enum [
          "full"
          "restricted"
          "none"
        ];
        default = "full";
        description = ''
          Network access mode:
          - full: Unrestricted internet access
          - restricted: Only allowed hosts can be accessed
          - none: No network access
        '';
      };

      allowedHosts = mkOption {
        type = types.listOf types.str;
        default = [ ];
        description = "Allowed hosts when network = restricted";
      };

      readOnlyWorkspace = mkOption {
        type = types.bool;
        default = false;
        description = "Mount the workspace as read-only inside the sandbox (filesystem-level enforcement)";
      };

      resourceLimits = {
        cpuQuota = mkOption {
          type = types.nullOr types.str;
          default = null;
          description = "CPU quota for the container (e.g., '200%' for 2 cores)";
          example = "200%";
        };

        memoryMax = mkOption {
          type = types.nullOr types.str;
          default = null;
          description = "Memory limit for the container (e.g., '4G')";
          example = "4G";
        };

        tasksMax = mkOption {
          type = types.nullOr types.int;
          default = null;
          description = "Maximum number of tasks/processes in the container";
          example = 512;
        };
      };

      initCommands = mkOption {
        type = types.listOf types.str;
        default = [ ];
        description = "Shell commands to run inside the container after creation. Failures warn but do not block creation.";
        example = [
          "npm install"
          "pip install pytest"
        ];
      };

      agentIdentity = {
        gitUser = mkOption {
          type = types.nullOr types.str;
          default = null;
          description = "Default git user.name for agents in sandboxes using this template";
          example = "Template Agent";
        };

        gitEmail = mkOption {
          type = types.nullOr types.str;
          default = null;
          description = "Default git user.email for agents in sandboxes using this template";
          example = "template-agent@example.com";
        };

        sshKeyPath = mkOption {
          type = types.nullOr types.path;
          default = null;
          description = "Path to SSH private key for agent push access in this template";
          example = "/run/secrets/template-agent-ssh-key";
        };
      };

      workspace = {
        mounts = mkOption {
          type = types.attrsOf (
            types.submodule {
              options = {
                containerPath = mkOption {
                  type = types.str;
                  description = "Mount point inside the container (e.g., '/workspace/.beads')";
                };
                hostPath = mkOption {
                  type = types.nullOr types.str;
                  default = null;
                  description = "Literal bind mount from host. Mutually exclusive with repo.";
                };
                repo = mkOption {
                  type = types.nullOr types.str;
                  default = null;
                  description = "Repo reference: null/empty for default --repo, a name for named --repo, or an absolute path";
                };
                mode = mkOption {
                  type = types.nullOr (
                    types.enum [
                      "jj"
                      "git-worktree"
                      "direct"
                    ]
                  );
                  default = null;
                  description = "VCS mode (null = auto-detect)";
                };
                branch = mkOption {
                  type = types.nullOr types.str;
                  default = null;
                  description = "Branch/ref to check out (VCS mounts only)";
                };
                readOnly = mkOption {
                  type = types.bool;
                  default = false;
                  description = "Mount as read-only";
                };
              };
            }
          );
          default = { };
          description = "Composable workspace mounts (keyed by name). When set, --repo becomes optional.";
        };

        useBeads = {
          enable = mkOption {
            type = types.bool;
            default = false;
            description = "Enable beads workspace overlay";
          };
          branch = mkOption {
            type = types.str;
            default = "beads-sync";
            description = "Branch to use for the beads workspace";
          };
          package = mkOption {
            type = types.nullOr types.package;
            default = null;
            description = "Beads package to install";
          };
          containerPath = mkOption {
            type = types.str;
            default = "/workspace/.beads";
            description = "Mount point for the beads workspace inside the container";
          };
          repo = mkOption {
            type = types.nullOr types.str;
            default = null;
            description = "Repo reference for beads (null = inherit default --repo)";
          };
        };
      };
    };
  };
in
{
  inherit agentType templateType;

  # The shared option tree under services.firefly-forage
  mkOptions =
    { defaultStateDir }:
    {
      enable = mkEnableOption "Firefly Forage AI sandbox system";

      user = mkOption {
        type = types.str;
        description = "Host user whose UID/GID will be used in sandboxes";
        example = "myuser";
      };

      authorizedKeys = mkOption {
        type = types.listOf types.str;
        default = [ ];
        description = "SSH public keys authorized to access sandboxes";
      };

      secrets = mkOption {
        type = types.attrsOf types.path;
        default = { };
        description = "Mapping of secret names to file paths containing API keys";
        example = {
          anthropic = "/run/secrets/anthropic-api-key";
          openai = "/run/secrets/openai-api-key";
        };
      };

      templates = mkOption {
        type = types.attrsOf templateType;
        default = { };
        description = "Sandbox templates";
      };

      stateDir = mkOption {
        type = types.path;
        default = defaultStateDir;
        description = "Directory for forage state and generated configs";
      };

      containerUsername = mkOption {
        type = types.str;
        default = "agent";
        description = "Username for the agent user inside sandbox containers";
      };

      workspacePath = mkOption {
        type = types.str;
        default = "/workspace";
        description = "Path to the workspace directory inside sandbox containers";
      };

      agentIdentity = {
        gitUser = mkOption {
          type = types.nullOr types.str;
          default = null;
          description = "Default git user.name for agent commits in sandboxes";
          example = "Forage Agent";
        };

        gitEmail = mkOption {
          type = types.nullOr types.str;
          default = null;
          description = "Default git user.email for agent commits in sandboxes";
          example = "agent@example.com";
        };

        sshKeyPath = mkOption {
          type = types.nullOr types.path;
          default = null;
          description = "Path to SSH private key for agent push access (e.g. sops-nix secret path)";
          example = "/run/secrets/agent-ssh-key";
        };
      };

      monitor = {
        enable = mkOption {
          type = types.bool;
          default = false;
          description = "Enable background health monitoring of sandboxes";
        };

        interval = mkOption {
          type = types.str;
          default = "60";
          description = "Health check interval in seconds";
        };

        autoRestart = mkOption {
          type = types.bool;
          default = false;
          description = "Automatically restart unhealthy containers";
        };
      };
    };
}
