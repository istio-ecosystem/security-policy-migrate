#!/usr/bin/env bash

# Usage
# -j: include a valid JWT token in the request
# -p /path: send request to the specified /path. Default is /headers
# -c context: use the specified context in all kubectl commands

REQ_PATH="/headers"
CONTEXT=""

POSITIONAL=()
while [[ $# -gt 0 ]]
do
key="$1"

case $key in
    -j|--jwt)
    JWT="yes"
    shift # past argument
    ;;
    -p|--path)
    REQ_PATH="$2"
    shift
    shift
    ;;
    -c|--context)
    CONTEXT="$2"
    shift
    shift
    ;;
    *)    # unknown option
    POSITIONAL+=("$1") # save it in an array for later
    shift # past argument
    ;;
esac
done
set -- "${POSITIONAL[@]}" # restore positional parameters

if [ -n "$CONTEXT" ]; then
  CONTEXT="--context $CONTEXT"
else
  CONTEXT=""
fi

if [ -n "$JWT" ]; then
  TOKEN=$(curl https://raw.githubusercontent.com/istio/istio/release-1.4/security/tools/jwt/samples/demo.jwt -s)
  kubectl exec -t $(kubectl get pod -l app=sleep -o jsonpath={.items..metadata.name} ${CONTEXT}) -c sleep ${CONTEXT} -- \
    curl -s http://httpbin:8000${REQ_PATH} --header "Authorization: Bearer $TOKEN"
else
  kubectl exec -t $(kubectl get pod -l app=sleep -o jsonpath={.items..metadata.name} ${CONTEXT}) -c sleep ${CONTEXT} -- \
    curl -s http://httpbin:8000${REQ_PATH}
fi
