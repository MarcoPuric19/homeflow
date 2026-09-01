#!/usr/bin/env sh
set -eu

missing=""
for tool in docker kubectl kind go git; do
  if ! command -v "${tool}" >/dev/null 2>&1; then
    missing="${missing} ${tool}"
  fi
done

if [ -n "${missing}" ]; then
  echo "Missing required local tools:${missing}" >&2
  exit 1
fi
docker compose version >/dev/null
echo "Required Sprint 1 tools are available."

