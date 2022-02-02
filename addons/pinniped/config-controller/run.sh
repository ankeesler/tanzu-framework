#!/bin/bash

set -e

tag="dev"
# tag="$(uuidgen)" # Uncomment to create random image every time
image="harbor-repo.vmware.com/tkgiam/$(whoami)/pinniped-config-controller-manager:$tag"
docker build -t "$image" .
docker push "$image"
ytt --data-value "image=$image" -f deployment.yaml | kbld -f - | kapp deploy -a pinniped-config-controller-manager -f - -y
