#!/usr/bin/env bash

# Patch istio-galley to disable validation reconcile to prepare for patching the validation webhook
kubectl get deploy istio-galley -n istio-system -o yaml | sed --expression "s/--enable-reconcileWebhookConfiguration=true/--enable-reconcileWebhookConfiguration=false/g" - | kubectl apply -f -

# Patch the validate webhook to not check the PeerAuthentication and RequestAuthentication, galley doesn't support these beta resources anyway.
kubectl -n istio-system get validatingwebhookconfigurations istio-galley -o json | \
  jq '(.webhooks[] | select(.name == "pilot.validation.istio.io").rules[] | select(contains({"apiGroups": ["security.istio.io"]})) | .resources) = ["authorizationpolicies"]' \
  kubectl apply -f -


