{ name, tag }:
[
  {
    apiVersion = "batch/v1";
    kind = "CronJob";
    metadata = { inherit name; };
    metadata.namespace = "nais-system";
    spec = {
      concurrencyPolicy = "Forbid";
      failedJobsHistoryLimit = 1;
      jobTemplate = {
        spec = {
          template = {
            metadata = {
              labels.app = name;
              inherit name;
            };
            spec = {
              containers = [
                {
                  image = "europe-north1-docker.pkg.dev/nais-io/nais/images/${name}:${tag}";
                  imagePullPolicy = "Always";
                  inherit name;
                  envFrom = [
                    { secretRef = "aiven-cost"; }
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
  }
]
