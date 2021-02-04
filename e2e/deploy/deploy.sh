#!/usr/bin/env bash

# deploy test workloads
# Namespace foo and bar: sleep, httpbin (8000, 80), helloworld (5000) and tcp-echo (9000, 9001)
# Namespace naked (no sidecar): sleep, httpbin (8000, 80), helloworld (5000) and tcp-echo (9000, 9001)

kubectl create ns foo
kubectl label ns foo istio-injection=enabled istio-injection- --overwrite
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/sleep/sleep.yaml -n foo
kubectl apply -f ./httpbin.yaml -n foo
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/helloworld/helloworld.yaml -n foo
kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-1.8/samples/tcp-echo/tcp-echo.yaml -n foo

kubectl create ns bar
kubectl label ns bar istio-injection=enabled istio-injection- --overwrite
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/sleep/sleep.yaml -n bar
kubectl apply -f ./httpbin.yaml -n bar
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/helloworld/helloworld.yaml -n bar
kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-1.8/samples/tcp-echo/tcp-echo.yaml -n bar

kubectl create ns naked
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/sleep/sleep.yaml -n naked
kubectl apply -f ./httpbin.yaml -n naked
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/helloworld/helloworld.yaml -n naked
kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-1.8/samples/tcp-echo/tcp-echo.yaml -n naked
