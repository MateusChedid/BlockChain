#!/bin/bash
# WattLedger - seeds the ledger with demo data through the REST API.
# Usage: ./scripts/seed-demo.sh [API_URL]   (default http://localhost:3000)
#
# After this script the current billing cycle has 3 weeks of signed readings
# for 3 prosumers on substation SUB-01. Eduardo's sensor is compromised from
# week 2 (valid signature, +40%): the audit and the reconciliation will catch it.
API=${1:-http://localhost:3000}

if ! docker network inspect fabric-network >/dev/null 2>&1; then
	echo "ERROR: the Fabric network is not running. Run ./fabric-network/deploy-network.sh first." >&2
	exit 1
fi
if ! curl -sf "$API/api/network" -H "X-Org: utility" >/dev/null; then
	echo "ERROR: the WattLedger API is not answering at $API." >&2
	echo "Run 'docker compose up -d --build' and check 'docker logs wattledger-api'." >&2
	exit 1
fi

call() {
	# call <org> <method> <path> [json]
	local org=$1 method=$2 path=$3 data=$4
	echo ""
	echo ">>> [$org] $method $path $data"
	if [ -n "$data" ]; then
		curl -s -X "$method" "$API$path" -H "Content-Type: application/json" -H "X-Org: $org" -d "$data"
	else
		curl -s -X "$method" "$API$path" -H "X-Org: $org"
	fi
	echo ""
}

echo "=== 1. Consortium onboards its members (only ConsortiumMSP may do it) ==="
call consortium POST /api/governance/members '{"memberID":"utility-dso","mspID":"UtilityMSP","displayName":"Distribution Utility (DSO)","role":"utility"}'
call consortium POST /api/governance/members '{"memberID":"solar-installers","mspID":"InstallerMSP","displayName":"Certified Solar Installers","role":"installer"}'
call consortium POST /api/governance/members '{"memberID":"wattledger-consortium","mspID":"ConsortiumMSP","displayName":"WattLedger Consortium","role":"operator"}'

echo "=== 2. Utility records the reference tariffs (Celesc B1, before taxes; Fio B is an assumption) ==="
call utility PUT /api/governance/tariffs '{"distributor":"Celesc-DIS (reference values)","teBRL":0.32193,"tusdBRL":0.37375,"fioBBRL":0.20,"fioBPct":60,"availabilityKWh":50,"tolerancePct":5}'

echo "=== 3. Households (off-chain profiles) ==="
call utility POST /api/users '{"id":"alice","displayName":"Alice Souza","type":"prosumer","substation":"SUB-01","consumerUnit":"UC-1001"}'
call utility POST /api/users '{"id":"bruno","displayName":"Bruno Lima","type":"prosumer","substation":"SUB-01","consumerUnit":"UC-1002"}'
call utility POST /api/users '{"id":"eduardo","displayName":"Eduardo Reis","type":"prosumer","substation":"SUB-01","consumerUnit":"UC-1003"}'
call utility POST /api/users '{"id":"carla","displayName":"Carla Mendes","type":"consumer","substation":"SUB-01","consumerUnit":"UC-1004"}'

echo "=== 4. Installer certifies the IoT meters (public keys and PV capacity go on-chain) ==="
call installer POST /api/meters '{"meterID":"M-ALICE-01","owner":"alice","consumerUnit":"UC-1001","substation":"SUB-01","capacityKWp":4}'
call installer POST /api/meters '{"meterID":"M-BRUNO-01","owner":"bruno","consumerUnit":"UC-1002","substation":"SUB-01","capacityKWp":3}'
call installer POST /api/meters '{"meterID":"M-EDUARDO-01","owner":"eduardo","consumerUnit":"UC-1003","substation":"SUB-01","capacityKWp":5}'

echo "=== 5. Three weeks of signed readings ==="
for week in 1 2 3; do
	call utility POST /api/meters/M-ALICE-01/readings '{"scenario":"honest","kwh":120.4}'
	call utility POST /api/meters/M-BRUNO-01/readings '{"scenario":"honest","kwh":94.5}'
	if [ "$week" == "1" ]; then
		call utility POST /api/meters/M-EDUARDO-01/readings '{"scenario":"honest","kwh":154}'
	else
		# Compromised sensor: physical energy 154 kWh, reported +40% with a valid signature
		call utility POST /api/meters/M-EDUARDO-01/readings '{"scenario":"compromised","kwh":154}'
	fi
done

echo ""
echo "=== Done. Open the portal at http://localhost:8080 ==="
