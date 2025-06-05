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
    {self, ...}@inputs:
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
        dockerTag = if lib.hasAttr "rev" self then
          "${builtins.toString self.revCount}-${self.shortRev}"
        else
          "gitDirty";

        docker = pkgs.dockerTools.buildImage {
            inherit (kafka-cost) name;
            tag = "${kafka-cost.version}-${dockerTag}";
            config = {
              Entrypoint = [ (lib.getExe kafka-cost) ];
            };
          };
          spec = pkgs.writeText "spec.yaml" (builtins.concatStringsSep ''

            ---
          '' (builtins.map builtins.toJSON [
            {
              # TODO: Write naisjob, just w/nix =D
            }
          ]));
      in
      rec {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            rust-analyzer
            cargo-watch
            clippy
            rustfmt

            # To install aiven's cli
            pipx
          ];
          inputsFrom = [ kafka-cost ];
        };
        checks = {inherit (packages) default docker spec;};
        formatter = inputs.treefmt-nix.lib.mkWrapper pkgs {
          programs.nixfmt.enable = true;
          programs.gofumpt.enable = true;
          programs.rustfmt.enable = true;
        };
        packages.default = kafka-cost;
        packages.docker = docker;
        packages.spec = spec;
      }
    );
}
