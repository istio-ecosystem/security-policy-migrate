#!/usr/bin/env bash

# deploy test workloads
# Namespace foo: sleep, httpbin and helloworld
# Namespace bar: sleep, httpbin and helloworld
# Namespace naked (no sidecar): sleep, httpbin and helloworld

kubectl create ns foo
kubectl label ns foo istio.io/rev=asm-1614-0
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/sleep/sleep.yaml -n foo
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/httpbin/httpbin.yaml -n foo
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/helloworld/helloworld.yaml -n foo

kubectl create ns bar
kubectl label ns bar istio.io/rev=asm-1614-0
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/sleep/sleep.yaml -n bar
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/httpbin/httpbin.yaml -n bar
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/helloworld/helloworld.yaml -n bar

kubectl create ns naked
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/sleep/sleep.yaml -n naked
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/httpbin/httpbin.yaml -n naked
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/helloworld/helloworld.yaml -n naked
