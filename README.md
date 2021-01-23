# security-policy-migrate

![Build and Test](https://github.com/yangminzhu/security-policy-migrate/workflows/Build/badge.svg)
![Release](https://github.com/yangminzhu/security-policy-migrate/workflows/Release/badge.svg)

A tool to convert the Istio v1alpha1 authentication policy to the v1beta1 version.

The tool helps you to upgrade the old Istio cluster (<= 1.5) to a newer version (>= 1.6) by migrating the deprecated
v1alpha1 authentication policy to the corresponding v1beta1 versions.

## Usage

1. Download the current latest release of the tool on github:

    ```bash
    curl -L -s https://github.com/yangminzhu/security-policy-migrate/releases/latest/download/convert.tar.gz --output convert.tar.gz
    ```

1. Extract the tool from the downloaded file:

    ```bash
    tar -xvf convert.tar.gz && chmod +x convert
    ```

1. Confirm the k8s cluster to be converted, the tool should be used with (old) Istio cluster that has v1alpha1 authentication policy:

    ```bash
    kubectl config current-context
    ```

1. Run the tool in the k8s cluster and store the beta policy in beta-policy.yaml:

    ```bash
    ./convert > beta-policy.yaml
    ```

1. Check the command output and make sure there are no errors, otherwise fix all errors and re-run the tool again.

1. Dry-run the beta policy to make sure it will be accepted:

    ```bash
    kubectl apply --dry-run=server -f beta-policy.yaml
    ```

1. Before applying in real cluster, double check the beta policies again to make sure it is correct.

## Supported Policy

This tool supports converting v1alpha1 authentication policy with the following limitations:

- Policy with multiple trigger rules is not supported;
- Policy with trigger rule using regex is not supported, this feature was experimental in alpha and removed in beta;
- Authorization policy is not supported, please check https://istio.io/latest/blog/2019/v1beta1-authorization-policy/#migration-from-the-v1alpha1-policy
  for manual conversion;
- etc.

The tool may also fail due to other issues (e.g. missing or unmatched service definition). Detail error message will be generated,
you should fix the error and re-run the tool.

If you are sure and confident that the error could be ignored safely, you can run the tool with `--ignore-error` to generate
the beta policy ignoring errors.

The tool also provides the flag `--context` and `--kubeconfig` to allow using with a specific cluster or config.

## Policy difference

Please be noted that the beta policy is very different from the alpha ones, some typical differences are listed below (not a full list):

- JWT policy denial message
   - In alpha policy, the HTTP code 401 will be returned with the body "Origin authentication failed."
   - In beta policy, the HTTP code 403 will be returned with the body "RBAC: access denied"

- Service name (alpha) v.s. Workload selector (beta)
   - In alpha policy, service name is used to select where to apply the policy
   - In beta policy, workload selector is used to select where to apply the policy

- etc.
