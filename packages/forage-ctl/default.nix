{
  writeShellApplication,
  coreutils,
  jq,
  systemd,
  openssh,
  gnugrep,
  gawk,
  findutils,
  nix,
  extra-container,
  jujutsu,
}:
writeShellApplication {
  name = "forage-ctl";

  runtimeInputs = [
    coreutils
    jq
    systemd
    openssh
    gnugrep
    gawk
    findutils
    nix
    extra-container
    jujutsu
  ];

  text = builtins.readFile ./forage-ctl.sh;

  meta = {
    description = "CLI tool for managing Firefly Forage sandboxes";
    mainProgram = "forage-ctl";
  };
}
