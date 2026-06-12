#!/bin/bash

set -ex

NAMESPACE="chantico"

SCRIPT_DIR=$(dirname -- "$( readlink -f -- "$0"; )")
MIB_SOURCE_DIR="$SCRIPT_DIR/mibs"

if [ ! -z "$1" ]; then
    MIB_SOURCE_DIR="$1"
fi
if [ "$MIB_SOURCE_DIR" == "-h" ] || [ "$MIB_SOURCE_DIR" == "--help" ] || [ ! -d "$MIB_SOURCE_DIR" ]; then
    echo "Usage: $0 [mib_source_directory]"
    exit 0
fi

# Wait for filebrowser to be ready before copying files
echo "Waiting for filebrowser deployment to be ready..."
kubectl rollout status deployment/chantico-filebrowser -n "$NAMESPACE" --timeout=0s
kubectl cp "$MIB_SOURCE_DIR/." -n "$NAMESPACE" "$(kubectl get pod -n "$NAMESPACE" -l app=chantico-filebrowser -o jsonpath='{.items[0].metadata.name}'):/srv/snmp/mibs/"
