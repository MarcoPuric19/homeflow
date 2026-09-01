#!/usr/bin/env sh
set -eu

namespace=homeflow-monitoring
secret=grafana-admin

kubectl apply -f monitoring/kubernetes/namespace.yaml >/dev/null
if kubectl -n "$namespace" get secret "$secret" >/dev/null 2>&1; then
  exit 0
fi

umask 077
password_file=$(mktemp /tmp/homeflow-grafana-password.XXXXXX)
trap 'rm -f "$password_file"' EXIT INT TERM
openssl rand -base64 24 | tr -d '\n' >"$password_file"
kubectl -n "$namespace" create secret generic "$secret" --from-file=admin-password="$password_file" >/dev/null
echo "Created local Grafana admin Secret. Use 'make grafana-password' to display it."
