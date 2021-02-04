# security-policy-migrate

![Build and Test](https://github.com/istio-ecosystem/security-policy-migrate/workflows/Build/badge.svg)
![Release](https://github.com/istio-ecosystem/security-policy-migrate/workflows/Release/badge.svg)

A tool to convert the Istio v1alpha1 authentication policy to the v1beta1 version.

The tool helps you to upgrade the old Istio cluster (<= 1.5) to a newer version (>= 1.6) by migrating the deprecated
v1alpha1 authentication policy to the corresponding v1beta1 versions.

## Usage

1. Download the current latest release of the tool on github:

    ```bash
    curl -L -s https://github.com/istio-ecosystem/security-policy-migrate/releases/latest/download/convert.tar.gz --output convert.tar.gz
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

    You could also use the flag `--per-namespace` to store policies per-namespace so that you could verify and apply the
    generated policies gradually per-namespace:

    ```bash
    mkdir beta-policy-dir
    ./convert --per-namespace beta-policy-dir
    ls beta-policy-dir
    ```

1. Check the command output and make sure there are no errors, otherwise fix all errors and re-run the tool again.

1. Dry-run the beta policy to make sure it will be accepted:

    ```bash
    kubectl apply --dry-run=server -f beta-policy.yaml
    ```

1. Before applying in a real cluster, double check the beta policies again to make sure it is correct.

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

## Common Errors

The following table lists common errors that could be returned by the tool together with the suggestions to fix the error:

| Error Message                                                                        | Why it happens                                                                                                                                                                                           | Suggestions                                                                                                                                                                                                                                                                                                                                                                                         |
|--------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| found duplicate target my-service                                                    | Multiple duplicate targets are specified in the same v1alpha1 Policy.                                                                                                                                    | Fix the v1alpha1 Policy to not use duplicate target name                                                                                                                                                                                                                                                                                                                                            |
| failed to convert target (my-service) to workload selector: could not find port      | The v1alpha1 Policy is using port-level configuration but the port could not be found in the corresponding service definition.                                                                           | This usually means there is either a typo in your existing v1alpha1 Policy probably or it is out-dated and inconsistent with the k8s Service.  The policy may not work as expected already, fix the policy to use the correct service name or port number. If the policy is correct, fix the corresponding k8s Service definition. If the target is not needed, remove it from the v1alpha1 Policy. |
| failed to convert target (my-service) to workload selector: could not find port name | Similar to the case above, but the mismatch is in the service name.                                                                                                                                      | See above.                                                                                                                                                                                                                                                                                                                                                                                          |
| failed to convert target (my-service) to workload selector: could not find service   | Similar to the case above, but more specifically the corresponding k8s service could not be found at all.                                                                                                | See above, make sure the k8s Service exist and it matches to your v1alpha1 policy.                                                                                                                                                                                                                                                                                                                  |
| triggerRule is not supported with multiple JWT issuer                                | This happens when you used the triggerRule field with multiple issuers. The semantics could be very complicated depending on your actual use case and the tool does not support this kind of conversion. | If your issuers are using the same triggerRule, you could manually convert them to a single AuthorizationPolicy easily.  If these issuers are using different triggerRule, you could potentially use the "request.auth.claims[iss]" condition to distinguish them if your JWT token includes the proper "iss" claim.                                                                                  |
| triggerRule.regex ("some-regex") is not supported                                    | The v1beta1 AuthorizationPolicy no longer supports regex matching.                                                                                                                                       | Consider convert the regex to prefix/suffix/exact matching.                                                                                                                                                                                                                                                                                                                                         |
| JWT is never supported in peer method                                                | The v1alpha1 Policy is using JWT method in its peer method lists.                                                                                                                                        | This is not supported in v1alpha1 Policy and should not be used in the first place.                                                                                                                                                                                                                                                                                                                 |
