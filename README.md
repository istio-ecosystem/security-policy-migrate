# security-policy-migrate

![Build and Test](https://github.com/yangminzhu/security-policy-migrate/workflows/Build/badge.svg)
![Release](https://github.com/yangminzhu/security-policy-migrate/workflows/Release/badge.svg)

A tool to convert the Istio v1alpha1 security policy to the v1beta1 version.

The tool helps you to upgrade the old Istio cluster (<= 1.5) to a newer version (>= 1.6) by migrating the deprecated
v1alpha1 authentication policy to the corresponding v1beta1 versions.

## Usage

1. Download the current latest release of the tool on github:

    ```console
    curl -L -s https://github.com/yangminzhu/security-policy-migrate/releases/download/v0.1/convert.tar.gz && tar -xvf convert.tar.gz && chmod +x convert 
    ```

1. Run the tool in the k8s cluster and redirect the generated beta policy to beta-policy.yaml:

    ```console
    ./convert > beta-policy.yaml
    ```

1. Dry-run the beta policy to make sure it will be accepted:

    ```console
    kubectl apply --dry-run=server -f beta-policy.yaml
    ```

1. Double check the generated beta policies to make sure it is correct before applying.
