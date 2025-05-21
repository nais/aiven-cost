{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

    crane.url = "github:ipetkov/crane";

    flake-utils.url = "github:numtide/flake-utils";

    advisory-db = {
      url = "github:rustsec/advisory-db";
      flake = false;
    };
    treefmt-nix = {
      url = "github:numtide/treefmt-nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };

  };

  outputs =
    inputs:
    inputs.flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import inputs.nixpkgs { localSystem = { inherit system; }; };
        inherit (pkgs) lib;

        craneLib = inputs.crane.mkLib pkgs;
        src = craneLib.cleanCargoSource ./kafka-cost;

        commonArgs = {
          inherit src;
          strictDeps = true;
          buildInputs = [ ] ++ lib.optionals pkgs.stdenv.isDarwin [ pkgs.libiconv ];
        };

        cargoArtifacts = craneLib.buildDepsOnly commonArgs;

        kafka-cost = craneLib.buildPackage (commonArgs // { inherit cargoArtifacts; });

      in
      {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            rust-analyzer

            # To install aiven's cli
            pipx
          ];
          inputsFrom = [ kafka-cost ];
        };
        formatter = inputs.treefmt-nix.lib.mkWrapper pkgs {
          programs.nixfmt.enable = true;
          programs.gofumpt.enable = true;
          programs.rustfmt.enable = true;
        };
        packages.default = kafka-cost;
      }
    );
}
