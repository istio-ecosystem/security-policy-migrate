# security-policy-migrate

A tool to convert the Istio v1alpha1 security policy to the v1beta1 version.

## Usage

1. Build the tool from source:

    ```console
    make build
    ```

1. Run the tool in the k8s cluster and redirect the generated beta policy to beta-policy.yaml:

    ```console
    $ ./out/convert > ./out/beta-policy.yaml
    ```

1. Double check the generated beta policies to make sure it is correct before applying.
