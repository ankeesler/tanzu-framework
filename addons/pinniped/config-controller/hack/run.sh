#!/bin/bash

# Copyright 2022 VMware, Inc. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

MY_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
TF_ROOT="${MY_DIR}/../../../.."
PACKAGE_ROOT="${TF_ROOT}/packages/management/pinniped-config-controller-manager"

# Always run from config-controller directory for reproducibility
cd "${MY_DIR}/.."

tag="dev"
# tag="$(uuidgen)" # Uncomment to create random image every time
controller_image="harbor-repo.vmware.com/tkgiam/$(whoami)/pinniped-config-controller-manager:$tag"
package_image="harbor-repo.vmware.com/tkgiam/$(whoami)/pinniped-config-controller-manager-package:$tag"

# Build pinniped-config-controller-manager image
docker build -t "$controller_image" .
docker push "$controller_image"

# Ensure generated deployment YAML (e.g., RBAC)
./hack/generate.sh

# Tell package to map default pinniped-config-controller-manager image to dev image
kbld_config="/tmp/pinniped-config-controller-manager-kbld-config.yaml"
cat <<EOF >"$kbld_config"
---
apiVersion: kbld.k14s.io/v1alpha1
kind: Config
overrides:
- image: pinniped-config-controller-manager:latest  # This image is hardcoded in the package
  newImage: ${controller_image}                     # This image is the dev image built above
EOF
ytt -f "${PACKAGE_ROOT}/bundle/config" \
  | kbld -f "$kbld_config" -f - --imgpkg-lock-output "${PACKAGE_ROOT}/bundle/.imgpkg/images.yml"

# Build the package
imgpkg push -b "$package_image" -f "${PACKAGE_ROOT}/bundle"

# Create the package on the cluster
yq e ".spec.template.spec.fetch[0].imgpkgBundle.image = \"${package_image}\"" "${PACKAGE_ROOT}/package.yaml" \
  | kubectl apply -f -
kubectl apply -f "${PACKAGE_ROOT}/metadata.yaml"

# Deploy the package on the cluster
package_sa_name="pinniped-config-controller-manager-package-sa"
package_namespace="tkg-system"
controller_namespace="pinniped-config-controller-manager-test-namespace"
cat <<EOF | kubectl apply -f -
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ${package_sa_name}
  namespace: ${package_namespace}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: ${package_sa_name}-cluster-admin
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: ${package_sa_name}
  namespace: tkg-system
---
apiVersion: v1
kind: Secret
metadata:
  name: pinniped-config-controller-manager-config
  namespace: ${package_namespace}
stringData:
  values.yaml: |
    namespace: ${controller_namespace}
---
apiVersion: packaging.carvel.dev/v1alpha1
kind: PackageInstall
metadata:
  name: pinniped-config-controller-manager
  namespace: ${package_namespace}
spec:
  packageRef:
    refName: pinniped-config-controller-manager.tanzu.vmware.com
    versionSelection:
      prereleases: {}
  syncPeriod: 30s
  serviceAccountName: ${package_sa_name}
  values:
  - secretRef:
      name: pinniped-config-controller-manager-config
EOF

# Wait for the app to be reconciled
kubectl wait --timeout 1m --for condition=Reconciling -n tkg-system app pinniped-config-controller-manager
kubectl wait --timeout 1m --for condition=ReconcileSucceeded -n tkg-system app pinniped-config-controller-manager

# Tail the logs
kubectl logs -n "$controller_namespace" deploy/pinniped-config-controller-manager -f
