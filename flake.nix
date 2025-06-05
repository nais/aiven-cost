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
    { self, ... }@inputs:
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
        dockerTag =
          if lib.hasAttr "rev" self then
            "${builtins.toString self.revCount}-${self.shortRev}"
          else
            "gitDirty";
        tag = "${kafka-cost.version}-${dockerTag}";
        docker = pkgs.dockerTools.buildImage {
          inherit (kafka-cost) name;
          inherit tag;

          config = {
            Entrypoint = [ (lib.getExe kafka-cost) ];
          };
        };
        spec = pkgs.writeText "spec.yaml" (builtins.toJSON {
          apiVersion = "batch/v1";
          kind = "CronJob";
          metadata.name = kafka-cost.name;
          metadata.namespace = "nais-system";
          spec = {
            concurrencyPolicy = "Forbid";
            failedJobsHistoryLimit = 1;
            jobTemplate = {
              spec = {
                template = {
                  metadata = {
                    labels.app = kafka-cost.name;
                    name = kafka-cost.name;
                  };
                  spec = {
                    containers = [
                      {
                        image = "europe-north1-docker.pkg.dev/nais-io/nais/images/kafka-cost:${tag}";
                        imagePullPolicy = "Always";
                        name = kafka-cost.name;
                        env = [
                          {
                            name = "AIVEN_BILLING_GROUP_ID";
                            value = "7d14362d-1e2a-4864-b408-1cc631bc4fab";
                          }
                        ];
                        envFrom = [
                          { secretRef.name = kafka-cost.name; }
                        ];
                        resources = {
                          requests = {
                            cpu = "250m";
                            memory = "512Mi";
                            ephemeral-storage = "1Gi";
                          };
                          limits = {
                            cpu = "250m";
                            memory = "512Mi";
                            ephemeral-storage = "1Gi";
                          };
                        };
                        securityContext = {
                          allowPrivilegeEscalation = false;
                          capabilities.drop = [ "ALL" ];
                          readOnlyRootFilesystem = false;
                          runAsNonRoot = true;
                          runAsUser = 65532;
                          seccompProfile.type = "RuntimeDefault";
                        };
                        terminationMessagePath = "/dev/termination-log";
                        terminationMessagePolicy = "File";
                      }
                    ];
                    dnsPolicy = "ClusterFirst";
                    restartPolicy = "OnFailure";
                    schedulerName = "default-scheduler";
                    serviceAccount = "aiven-cost";
                    serviceAccountName = "aiven-cost";
                    terminationGracePeriodSeconds = 30;
                  };
                  schedule = "30 5 * * *";
                  successfulJobsHistoryLimit = 1;
                  suspend = false;
                };
              };
            };
          };
        });
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
        checks = { inherit (packages) default docker spec; };
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
