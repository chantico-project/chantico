#!/usr/bin/env bash

set -ex

# Namespace
NAMESPACE="chantico"

# Targets and ports as arrays
TARGETS=("svc/chantico-prometheus" "svc/chantico-snmp" "deployment/chantico-filebrowser")
LOCAL_PORTS=(19090 19116 18888)
REMOTE_PORTS=(9090 9116 80)

# Port-forward loop
port_forward_loop() {
  local target=$1
  local local_port=$2
  local remote_port=$3
  echo "🚀 Starting port-forward: localhost:$local_port → $target:$remote_port"
  while true; do
    # Do not exit from the backgrounded loop function immediately
    # Handle errors as a lost forward
    set +e
    kubectl port-forward -n "$NAMESPACE" "$target" "$local_port:$remote_port"
    set -e
    echo "⚠️ Port-forwarding for $target lost. Retrying in 2 seconds..."
    sleep 2
  done
}

# Trap Ctrl+C to cleanly exit
trap "echo '🛑 Stopping port-forwarding...'; kill 0; exit 0" SIGINT

# Loop through arrays and start port-forwarding in background
for i in "${!TARGETS[@]}"; do
  port_forward_loop "${TARGETS[$i]}" "${LOCAL_PORTS[$i]}" "${REMOTE_PORTS[$i]}" &
done

# Wait for all background jobs
wait
