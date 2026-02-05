{
  lib,
  buildGoModule,
}:

buildGoModule {
  pname = "forage-ctl";
  version = "0.1.0";

  src = ./.;

  vendorHash = "sha256-hoXlgAidu3eEhY5GBx/YDxzThdBIKfQtmA9bQ21eIzU=";

  # Disable CGO for static build
  env.CGO_ENABLED = "0";

  meta = with lib; {
    description = "Firefly Forage sandbox management CLI";
    homepage = "https://github.com/firefly-engineering/firefly-forage";
    license = licenses.mit;
    mainProgram = "forage-ctl";
  };
}
