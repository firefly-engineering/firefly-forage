{
  lib,
  buildGoModule,
  git,
  jujutsu,
}:

buildGoModule {
  pname = "forage-ctl";
  version = "0.1.0";

  src = ./.;

  vendorHash = "sha256-GexDCgbFueOXsiSBBmJb14b7gC5g9P27BE72U/vV2+A=";

  # Disable CGO for static build
  env.CGO_ENABLED = "0";

  # Tests shell out to git and jj for config parsing and workspace operations
  nativeCheckInputs = [
    git
    jujutsu
  ];

  meta = with lib; {
    description = "Firefly Forage sandbox management CLI";
    homepage = "https://github.com/firefly-engineering/firefly-forage";
    license = licenses.mit;
    mainProgram = "forage-ctl";
  };
}
