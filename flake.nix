{
  description = "Firefly Forage - Isolated sandboxes for AI coding agents";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    extra-container.url = "github:erikarvstedt/extra-container";
    extra-container.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs =
    {
      self,
      nixpkgs,
      extra-container,
    }:
    let
      supportedSystems = [
        "x86_64-linux"
        "aarch64-linux"
      ];

      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      pkgsFor = system: nixpkgs.legacyPackages.${system};
    in
    {
      nixosModules = {
        default = self.nixosModules.host;
        host = import ./modules/host.nix { inherit self extra-container; };
      };

      lib = import ./lib { inherit (nixpkgs) lib; };

      packages = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
        in
        {
          forage-ctl = pkgs.callPackage ./packages/forage-ctl {
            extra-container = extra-container.packages.${system}.default;
          };
          default = self.packages.${system}.forage-ctl;
        }
      );

      devShells = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              nixfmt-rfc-style
              nil
            ];
          };
        }
      );

      formatter = forAllSystems (system: (pkgsFor system).nixfmt-rfc-style);
    };
}
