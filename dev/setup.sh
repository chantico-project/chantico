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

# Install CRDs using kustomize
make -C "$SCRIPT_DIR/.." install

# Install chantico dependencies (filebrowser, prometheus, snmp exporter)
CI_REGISTRY="ci.tno.nl/ipcei-cis-misd-sustainable-datacenters/wp2/energy-domain-controller/chantico"
helm install chantico ../config/deployment/ --set controller.include=false --set pvc.storageClassName="local-path" -n chantico

# Make snmp-mock docker image
SNMP_MOCK_IMAGE="$CI_REGISTRY/chantico-snmp-mock:$SNMP_MOCK_TAG"
docker pull "$SNMP_MOCK_IMAGE"
docker tag "$SNMP_MOCK_IMAGE" chantico-snmp-mock:latest
kind load docker-image chantico-snmp-mock:latest --name kind

# Copy mock MIB file onto the PVC so the SNMP generator job can find it.
# In production this is done manually via the filebrowser UI.
echo "Waiting for chantico-filebrowser to be ready..."
kubectl rollout status deployment/chantico-filebrowser -n chantico --timeout=120s
kubectl exec -n chantico deployment/chantico-filebrowser -- mkdir -p /srv/snmp/mibs
kubectl cp "$SCRIPT_DIR/TNO-PDU-MIB.txt" chantico/$(kubectl get pod -n chantico -l app=chantico-filebrowser -o jsonpath='{.items[0].metadata.name}'):/srv/snmp/mibs/TNO-PDU-MIB.txt

# Apply to k8s
kubectl apply -f ../config/samples/chantico_v1alpha1_physicalmeasurement_mock.yaml
kubectl apply -f k8s/snmp-mock-deployment.yaml
kubectl apply -f k8s/snmp-mock-service.yaml

popd
