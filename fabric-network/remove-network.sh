#!/bin/bash
# WattLedger - stops the Fabric network and removes every generated artifact.
cd "$(dirname "$0")"

echo "--- Stopping Fabric containers ---"
docker compose -f docker-compose.yaml down --volumes --remove-orphans || true

echo "--- Removing chaincode containers and images ---"
docker ps -aq --filter "name=dev-peer0" | xargs -r docker rm -f >/dev/null 2>&1 || true
docker images -q --filter "reference=dev-peer0*" | xargs -r docker rmi -f >/dev/null 2>&1 || true

echo "--- Removing crypto material, channel artifacts and packages ---"
rm -rf config/crypto-config channel-artifacts system-genesis-block ./*.tar.gz
rm -rf chaincode/governance/vendor chaincode/energy/vendor

if [ "$1" == "--prune-images" ]; then
	echo "--- Removing all Hyperledger Docker images ---"
	docker images -q "hyperledger/*" | xargs -r docker rmi -f || true
fi

echo "Network teardown complete. All Fabric artifacts removed."
