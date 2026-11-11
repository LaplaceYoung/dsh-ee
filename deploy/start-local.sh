#!/usr/bin/env sh
# Build the dsh-ee image and start it with a single-node TiDB. Run from anywhere.
set -eu
cd "$(dirname "$0")"
command -v docker >/dev/null || { echo "Docker isn't on PATH. Open Docker Desktop first." >&2; exit 1; }
docker info >/dev/null 2>&1 || { echo "Docker Desktop isn't running yet. Open it and wait for 'Engine running'." >&2; exit 1; }
echo "· Building dsh-ee image (first time takes a few minutes)"
docker build -t dsh-ee:latest ..
echo "· Starting dsh-ee + TiDB"
docker compose up -d
printf "· Waiting for dsh-ee"
i=0
until curl -fs http://127.0.0.1:8080/healthz >/dev/null 2>&1; do
  i=$((i+1)); [ $i -gt 120 ] && { echo; echo "Timed out. Check: docker compose logs dsh-ee" >&2; exit 1; }
  printf "."; sleep 5
done
echo
echo "dsh-ee is up: http://localhost:8080"
echo "Admin login: admin / $(sed -n 's/^DSH_EE_BOOTSTRAP_ADMIN_PASSWORD=//p' .env)"
