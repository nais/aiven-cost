{
  inputs.nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";
  inputs.treefmt-nix = {
    url = "github:numtide/treefmt-nix";
    inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs =
    inputs:
    inputs.flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import inputs.nixpkgs { localSystem = { inherit system; }; };
      in
      {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            delve
            go
            ginkgo
            postgresql
          ];
        };
        formatter = inputs.treefmt-nix.lib.mkWrapper pkgs {
          programs.nixfmt.enable = true;
          programs.gofumpt.enable = true;
        };
      }
    );
}
