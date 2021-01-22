#!/usr/bin/env bash

kubectl delete policies.authentication.istio.io --all --all-namespaces
kubectl delete meshpolicies.authentication.istio.io --all --all-namespaces
kubectl delete peerauthentications.security.istio.io --all --all-namespaces
kubectl delete requestauthentications.security.istio.io --all --all-namespaces
kubectl delete authorizationpolicies.security.istio.io --all --all-namespaces
