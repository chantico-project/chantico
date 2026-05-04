#!/usr/bin/env bash

set -ex

SCRIPT_DIR=$(dirname -- "$( readlink -f -- "$0"; )")
SNMP_MOCK_TAG="${SNMP_MOCK_TAG:-latest}"

# get kind
go install sigs.k8s.io/kind@v0.30.0

# If go is not yet added to $PATH:
#echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.bashrc && source ~/.bashrc

kind create cluster --config "$SCRIPT_DIR/kind-config.yaml"

kubectl create namespace chantico

# Create storageclass from https://github.com/rancher/local-path-provisioner
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.32/deploy/local-path-storage.yaml

pushd "$SCRIPT_DIR"

# Update CRDs in helm deployment
make -C ../ sync-deployment-crds

# Install chantico dependencies (filebrowser, prometheus, snmp exporter)
helm install chantico ../config/deployment/ --set controller.include=false --set pvc.storageClassName="local-path" -n chantico

# Make snmp-mock docker image
CI_REGISTRY="ci.tno.nl/ipcei-cis-misd-sustainable-datacenters/wp2/energy-domain-controller/chantico"
SNMP_MOCK_IMAGE="$CI_REGISTRY/chantico-snmp-mock:$SNMP_MOCK_TAG"
docker pull "$SNMP_MOCK_IMAGE"
docker tag "$SNMP_MOCK_IMAGE" chantico-snmp-mock:latest
kind load docker-image chantico-snmp-mock:latest --name kind

# Apply to k8s
kubectl apply -f ../config/samples/chantico_v1alpha1_physicalmeasurement_mock.yaml
kubectl apply -f k8s/snmp-mock-deployment.yaml
kubectl apply -f k8s/snmp-mock-service.yaml

popd
