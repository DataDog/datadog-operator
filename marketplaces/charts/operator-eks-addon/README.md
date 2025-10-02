# Operator EKS Add-on

This is a wrapper chart for installing EKS add-on. Charts required for the add-on are added as a dependency to this chart. Chart itself doesn't contain any templates or configurable properties.

## Version Mapping
| `operator-addon-chart` | `datadog-operator` | `datadog-crds` | Operator | Agent | Cluster Agent |
| :-: | :-: | :-: | :-: | :-: | :-: |
| < 0.1.6 | 1.0.5 | 1.0.1 | 1.0.3 | 7.43.1 | 7.43.1 | 
| 0.1.6 | 1.4.1 | 1.3.0 | 1.3.0 | 7.47.1 | 7.47.1 |
| 0.1.7 | 1.5.1 | 1.4.0 | 1.4.0 | 7.50.3 | 7.50.3 |
| 0.1.9 | 1.8.1 | 1.7.0 | 1.7.0 | 7.54.0 | 7.54.0 |
| 0.1.10 | 2.5.1 | 2.3.0 | 1.11.1 | 7.60.0 | 7.60.0 |
| 0.1.12 | 2.7.0 | 2.4.1 | 1.12.1 | 7.62.2 | 7.62.2 |
| 0.1.13 | 2.9.1 | 2.7.0 | 1.14.0 | 7.64.3 | 7.64.3 |
| 0.1.15 | 2.11.0 | 2.9.0 | 1.16.0 | 7.67.0 | 7.67.0 |
| 0.1.16 | 2.12.1 | 2.10.0 | 1.17.0 | 7.68.2 | 7.68.2 |

* 0.1.8 failed validation and didn't go through.
* 0.1.11 failed validation and didn't go through.
* 0.1.14 skipped due to delays in submission.

## Pushing Add-on Chart

The below steps have been validated using `Helm v3.12.0`.

Prepare Chart:

* Update the `datadog-operator` dependency version in `charts/operator-eks-addon/Chart.yaml`.
* Bump `operator-eks-addon` chart version.
* Run:
    ```sh
    ./update-addon-chart.sh
    ```
  * Script vendors `datadog-operator` and `datadog-crds` charts and drops helm constructs incompatible with the add-on workflows.
  * It also also updates `addon-manifest.yaml`, `operator-eks-addon` rendering with default values.
* Update above table with new versions.
* Review changes in `addon-manifest.yaml`, create a PR and get it merged.

Package chart and validate:

* This step creates a chart archive. For example, `operator-eks-addon-0.1.0.tgz`
    ```sh
    helm package ./charts/operator-eks-addon
    ```

* Validate the artifact
    ```sh
    # Unpack the chart and go the the chart folder
    tar -xzf operator-eks-addon-0.1.3.tgz -C /tmp/
    cd /tmp/operator-eks-addon

    # Review chart version and dependency version are correct
    Ensure in all the manifests/templates the absence of unsupported Helm objects.

    # Render chart in a file
    helm template datadog-operator . -n datadog-agent > operator-addon.yaml

    # Render chart with a sample override
    helm template datadog-operator . --set datadog-operator.image.tag=1.2.0 > operator-addon.yaml
    ```
    Make sure the rendered manifest contains the CRD, Operator tag override works and uses correct EKS repo. 

    Install it on a Kind cluster after replacing registry `709825985650.dkr.ecr.us-east-1.amazonaws.com` with a public one.

    ```sh
    kubectl apply -f operator-addon.yaml
    ```
    Confirm Operator is deployed and pods reach a running state. Afterwards, create a secret and apply the default `DatadogAgent` manifest and make sure agents reach a running state and metrics show up in the app.

Push the artifact to EKS repo:
* Authenticate Helm with the repo, 
    ```sh
    aws ecr get-login-password --region us-east-1 | helm registry login --username AWS --password-stdin 709825985650.dkr.ecr.us-east-1.amazonaws.com
    ```

* Push the chart archive to the Marketplace repository. This will upload the chart at `datadog/helm-charts/operator-eks-addon` and tag it with version `0.1.0`. See [ECR documentation][eks-helm-push] for more details.
    ```sh
    helm push operator-eks-addon-0.1.0.tgz oci://709825985650.dkr.ecr.us-east-1.amazonaws.com/datadog/helm-charts
    ```

* Validate the version by listing the repository:
    ```sh
    aws ecr describe-images --registry-id 709825985650 --region us-east-1  --repository-name datadog/helm-charts/operator-eks-addon
    {
        "imageDetails": [
            {
                "registryId": "709825985650",
                "repositoryName": "datadog/helm-charts/operator-eks-addon",
                "imageDigest": "sha256:d6e54cbe69bfb962f0a4e16c9b29a7572f6aaf479de347f91bea8331a1a867f9",
                "imageTags": [
                    "0.1.0"
                ],
                "imageSizeInBytes": 63269,
                "imagePushedAt": 1690215560.0,
                "imageManifestMediaType": "application/vnd.oci.image.manifest.v1+json",
                "artifactMediaType": "application/vnd.cncf.helm.config.v1+json"
            }
        ]
    }
    ```

## Pushing Container Images
Images required during add-on installation must be available through the EKS marketplace repository. Each image can be copied by using `crane copy`. Make sure all referenced tags are uploaded to the respective repository.
```sh
aws ecr get-login-password --region us-east-1|crane auth login --username AWS --password-stdin 709825985650.dkr.ecr.us-east-1.amazonaws.com

❯ crane copy gcr.io/datadoghq/operator:1.0.3 709825985650.dkr.ecr.us-east-1.amazonaws.com/datadog/operator:1.0.3
```

To validate, describe the repository
```sh
aws ecr describe-images --registry-id 709825985650 --region us-east-1  --repository-name datadog/operator
..
        {
            "registryId": "709825985650",
            "repositoryName": "datadog/operator",
            "imageDigest": "sha256:e7ad530ca73db7324186249239dec25556b4d60d85fa9ba0374dd2d0468795b3",
            "imageTags": [
                "1.0.3"
            ],
..
```

[eks-helm-push]: https://docs.aws.amazon.com/AmazonECR/latest/userguide/push-oci-artifact.html
