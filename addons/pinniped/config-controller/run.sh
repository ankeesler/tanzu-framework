#!/bin/bash

cd addons/pinniped/config-controller

image="harbor-repo.vmware.com/tkgiam/$(whoami)/pinniped-config-controller-manager:$(uuidgen)"
docker build -t "$image" .
docker push "$image"
ytt --data-value "image=$image" -f deployment.yaml | kubectl apply -f -
