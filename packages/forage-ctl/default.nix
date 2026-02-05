{
  writeShellApplication,
  coreutils,
  jq,
  systemd,
  openssh,
  gnugrep,
  gawk,
  findutils,
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
  ];

  text = builtins.readFile ./forage-ctl.sh;

  meta = {
    description = "CLI tool for managing Firefly Forage sandboxes";
    mainProgram = "forage-ctl";
  };
}
