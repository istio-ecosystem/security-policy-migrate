# security-policy-migrate

![Build and Test](https://github.com/yangminzhu/security-policy-migrate/workflows/Build/badge.svg)
![Release](https://github.com/yangminzhu/security-policy-migrate/workflows/Release/badge.svg)

A tool to convert the Istio v1alpha1 security policy to the v1beta1 version.

The tool helps you to upgrade the old Istio cluster (<= 1.5) to a newer version (>= 1.6) by migrating the deprecated
v1alpha1 authentication policy to the corresponding v1beta1 versions.

## Usage

1. Download the current latest release of the tool on github:

    ```console
    $ curl -L -s https://github.com/yangminzhu/security-policy-migrate/releases/download/v0.2/convert.tar.gz --output convert.tar.gz
    ```

1. Extract the tool from the downloaded file:

    ```console
    $ tar -xvf convert.tar.gz && chmod +x convert
    ```

1. Confirm the k8s cluster to be converted, the tool should be used with (old) Istio cluster that has v1alpha1 authentication policy:

    ```console
    $ kubectl config current-context
    ```

1. Run the tool in the k8s cluster and store the beta policy in beta-policy.yaml:

    ```console
    $ ./convert > beta-policy.yaml
    ```

1. Check the command output and make sure there are no errors, otherwise fix all errors and re-run the tool again.

1. Dry-run the beta policy to make sure it will be accepted:

    ```console
    $ kubectl apply --dry-run=server -f beta-policy.yaml
    ```

1. Before applying in real cluster, double check the beta policies again to make sure it is correct.
