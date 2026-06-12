#!/usr/bin/env bash

set -ex

kind delete cluster

# # Clean up old persistent volume claim on local host path for cleaner test info
# if [ -d "$CHANTICOVOLUMELOCATIONENV" ]; then
#     rm -rf "$CHANTICOVOLUMELOCATIONENV"
# fi
