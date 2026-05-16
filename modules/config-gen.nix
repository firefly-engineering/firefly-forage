# Shared configuration generation logic for services.firefly-forage.
# Used by both host.nix (NixOS) and darwin.nix (nix-darwin) to produce
# config.json and template JSON files.
{ lib }:
let
  inherit (lib)
    mapAttrs
    hasPrefix
    filterAttrs
    optionalAttrs
    ;
in
rec {
  # Resolve ~ to a user's home directory
  resolveTilde =
    userHome: path:
    if path == null then
      null
    else if hasPrefix "~/" path then
      userHome + (builtins.substring 1 (builtins.stringLength path - 1) path)
    else if path == "~" then
      userHome
    else
      path;

  # Derive container config dir from host path
  # e.g., ~/.claude -> /home/agent/.claude
  deriveContainerPath =
    containerUsername: hostPath:
    let
      baseName = baseNameOf hostPath;
    in
    "/home/${containerUsername}/${baseName}";

  # Generate the template JSON for a single template.
  # resolveTilde' should be a partially-applied (resolveTilde userHome).
  mkTemplateJSON =
    {
      cfg,
      template,
      resolveTilde',
    }:
    let
      # Merge explicit workspace.mounts with useBeads-injected mount
      beadsMount =
        if template.workspace.useBeads.enable then
          {
            beads = {
              containerPath = template.workspace.useBeads.containerPath;
              repo = template.workspace.useBeads.repo;
              mode = "jj";
              branch = template.workspace.useBeads.branch;
              readOnly = false;
              hostPath = null;
            };
          }
        else
          { };
      allMounts = template.workspace.mounts // beadsMount;

      # Merge useBeads package into extraPackages
      beadsPackages =
        if template.workspace.useBeads.enable && template.workspace.useBeads.package != null then
          [ template.workspace.useBeads.package ]
        else
          [ ];
      allExtraPackages = template.extraPackages ++ beadsPackages;
    in
    {
      inherit (template)
        description
        network
        allowedHosts
        readOnlyWorkspace
        ;
      agents = mapAttrs (
        agentName: agent:
        let
          resolvedHostConfigDir = resolveTilde' agent.hostConfigDir;
          resolvedContainerConfigDir =
            if agent.containerConfigDir != null then
              agent.containerConfigDir
            else if resolvedHostConfigDir != null then
              deriveContainerPath cfg.containerUsername resolvedHostConfigDir
            else
              null;
        in
        {
          inherit (agent) secretName authEnvVar hostConfigDirReadOnly;
          packagePath = agent.package.pname;
          hostConfigDir = resolvedHostConfigDir;
          containerConfigDir = resolvedContainerConfigDir;
          permissions =
            if agent.permissions != null then
              {
                inherit (agent.permissions) skipAll allow deny;
              }
            else
              null;
        }
      ) template.agents;
      extraPackages = map (p: p.pname) allExtraPackages;
    }
    // optionalAttrs (allMounts != { }) {
      workspaceMounts = mapAttrs (
        mountName: mount:
        filterAttrs (_: v: v != null) {
          inherit (mount) containerPath readOnly;
          hostPath = if mount.hostPath != null then resolveTilde' mount.hostPath else null;
          repo = mount.repo;
          mode = mount.mode;
          branch = mount.branch;
        }
      ) allMounts;
    }
    //
      optionalAttrs
        (
          template.resourceLimits.cpuQuota != null
          || template.resourceLimits.memoryMax != null
          || template.resourceLimits.tasksMax != null
        )
        {
          resourceLimits = filterAttrs (_: v: v != null) {
            cpuQuota = template.resourceLimits.cpuQuota;
            memoryMax = template.resourceLimits.memoryMax;
            tasksMax = template.resourceLimits.tasksMax;
          };
        }
    // optionalAttrs (template.initCommands != [ ]) {
      inherit (template) initCommands;
    }
    //
      optionalAttrs
        (
          template.agentIdentity.gitUser != null
          || template.agentIdentity.gitEmail != null
          || template.agentIdentity.sshKeyPath != null
        )
        {
          agentIdentity = filterAttrs (_: v: v != null) {
            gitUser = template.agentIdentity.gitUser;
            gitEmail = template.agentIdentity.gitEmail;
            sshKeyPath =
              if template.agentIdentity.sshKeyPath != null then
                resolveTilde' (toString template.agentIdentity.sshKeyPath)
              else
                null;
          };
        };

  # Generate the host config JSON.
  mkHostConfigJSON =
    {
      cfg,
      resolveTilde',
      extraAttrs ? { },
    }:
    {
      user = cfg.user;
      secrets = cfg.secrets;
      stateDir = cfg.stateDir;
    }
    // lib.optionalAttrs (cfg.containerUsername != "agent") {
      containerUsername = cfg.containerUsername;
    }
    // lib.optionalAttrs (cfg.workspacePath != "/workspace") {
      workspacePath = cfg.workspacePath;
    }
    //
      lib.optionalAttrs
        (
          cfg.agentIdentity.gitUser != null
          || cfg.agentIdentity.gitEmail != null
          || cfg.agentIdentity.sshKeyPath != null
        )
        {
          agentIdentity = filterAttrs (_: v: v != null) {
            gitUser = cfg.agentIdentity.gitUser;
            gitEmail = cfg.agentIdentity.gitEmail;
            sshKeyPath =
              if cfg.agentIdentity.sshKeyPath != null then
                resolveTilde' (toString cfg.agentIdentity.sshKeyPath)
              else
                null;
          };
        }
    // extraAttrs;

  # Generate template etc entries (name -> { target, text })
  mkTemplateEtcEntries =
    {
      cfg,
      configDir,
      resolveTilde',
    }:
    mapAttrs (name: template: {
      target = "${configDir}/templates/${name}.json";
      text = builtins.toJSON (
        {
          inherit name;
        }
        // mkTemplateJSON {
          inherit cfg template resolveTilde';
        }
      );
    }) cfg.templates;
}
