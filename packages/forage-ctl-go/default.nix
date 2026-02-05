{
  lib,
  buildGoModule,
}:

buildGoModule {
  pname = "forage-ctl";
  version = "0.1.0";

  src = ./.;

  vendorHash = null;

  # Disable CGO for static build
  env.CGO_ENABLED = "0";

  # Rename binary from forage-ctl-go to forage-ctl
  postInstall = ''
    mv $out/bin/forage-ctl-go $out/bin/forage-ctl
  '';

  meta = with lib; {
    description = "Firefly Forage sandbox management CLI";
    homepage = "https://github.com/firefly-engineering/firefly-forage";
    license = licenses.mit;
    mainProgram = "forage-ctl";
  };
}
