# security-policy-migrate

A tool to convert the Istio v1alpha1 security policy to the v1beta1 version.

The tool helps you to upgrade the old Istio cluster (<= 1.5) to a newer version (>= 1.6) by migrating the deprecated
v1alpha1 authentication policy to the corresponding v1beta1 versions.

## Usage

1. Build the tool from source:

    ```console
    make build
    ```

1. Run the tool in the k8s cluster and redirect the generated beta policy to beta-policy.yaml:

    ```console
    $ ./out/convert > ./out/beta-policy.yaml
    ```

1. Dry-run the beta policy to make sure it will be accepted:

    ```console
    $ kubectl apply --dry-run=server -f out/beta-policy.yaml
    ```

1. Double check the generated beta policies to make sure it is correct before applying.
