#!/bin/bash
# WattLedger - runs the whole demo setup in order and stops at the first error.
set -e
cd "$(dirname "$0")"
./fabric-network/deploy-network.sh
docker compose up -d --build
echo "Waiting for the API..."
for i in $(seq 1 30); do
	curl -sf http://localhost:3000/api/network -H "X-Org: utility" >/dev/null && break
	sleep 2
done
./scripts/seed-demo.sh
echo "Portal: http://localhost:8080"
