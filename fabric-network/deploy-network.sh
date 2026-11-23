#!/bin/bash
# WattLedger - deploys a Hyperledger Fabric 2.5 network with three organizations
# (Utility / Installer / Consortium), one Raft orderer, one channel and two chaincodes.
set -e

cd "$(dirname "$0")"

export PATH=${PWD}/../bin:$PATH
export FABRIC_CFG_PATH=${PWD}/config

CHANNEL_NAME="energychannel"
ORDERER_ADDRESS="orderer.wattledger.com:7050"
ORDERER_TLS_HOSTNAME_OVERRIDE="orderer.wattledger.com"
ORDERER_CA=${PWD}/config/crypto-config/ordererOrganizations/wattledger.com/orderers/orderer.wattledger.com/msp/tlscacerts/tlsca.wattledger.com-cert.pem

ORGS="Utility Installer Consortium"
CC_VERSION="1.0"
CC_SEQUENCE="1"

################################################################################
# Helper Functions
################################################################################

step() {
	echo ""
	echo "================================================================"
	echo "  $1"
	echo "================================================================"
}

check_prerequisites() {
	for bin in peer configtxgen cryptogen docker go; do
		if ! command -v "$bin" >/dev/null 2>&1; then
			echo "ERROR: '$bin' not found in PATH. See README.md (Prerequisites)." >&2
			exit 1
		fi
	done
	for host in orderer.wattledger.com peer0.utility.wattledger.com peer0.installer.wattledger.com peer0.consortium.wattledger.com; do
		if ! grep -q "$host" /etc/hosts; then
			echo "ERROR: '$host' is missing in /etc/hosts. See README.md (Configure Hostnames)." >&2
			exit 1
		fi
	done
}

set_peer_context() {
	ORG="$1"
	case "$ORG" in
	Utility)
		DOMAIN="utility.wattledger.com"
		export CORE_PEER_LOCALMSPID="UtilityMSP"
		export CORE_PEER_ADDRESS=peer0.utility.wattledger.com:7051
		;;
	Installer)
		DOMAIN="installer.wattledger.com"
		export CORE_PEER_LOCALMSPID="InstallerMSP"
		export CORE_PEER_ADDRESS=peer0.installer.wattledger.com:9051
		;;
	Consortium)
		DOMAIN="consortium.wattledger.com"
		export CORE_PEER_LOCALMSPID="ConsortiumMSP"
		export CORE_PEER_ADDRESS=peer0.consortium.wattledger.com:11051
		;;
	*)
		echo "Unknown org: $ORG"
		exit 1
		;;
	esac
	export CORE_PEER_MSPCONFIGPATH=${PWD}/config/crypto-config/peerOrganizations/${DOMAIN}/users/Admin@${DOMAIN}/msp
	export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/config/crypto-config/peerOrganizations/${DOMAIN}/peers/peer0.${DOMAIN}/tls/ca.crt
	export CORE_PEER_TLS_ENABLED=true
}

wait_for_orderer_leader() {
	CONTAINER_NAME=${1:-orderer.wattledger.com}
	TIMEOUT=${2:-60}
	elapsed=0
	echo "Waiting up to ${TIMEOUT}s for ${CONTAINER_NAME} to elect a Raft leader..."
	while [ $elapsed -lt $TIMEOUT ]; do
		if docker logs "$CONTAINER_NAME" 2>&1 | grep -E "elected leader|Start accepting requests as Raft leader|Leader .* is present|Raft leader changed" >/dev/null; then
			echo "${CONTAINER_NAME} reports a leader. Continuing."
			return 0
		fi
		sleep 2
		elapsed=$((elapsed + 2))
	done
	echo "Timeout waiting for ${CONTAINER_NAME} to elect a leader." >&2
	return 1
}

################################################################################
# 0. Prerequisites and cleanup
################################################################################

check_prerequisites

step "0. Cleaning up previous run"
docker compose down --volumes --remove-orphans 2>/dev/null || true
docker ps -aq --filter "name=dev-peer0" | xargs -r docker rm -f >/dev/null 2>&1 || true
rm -rf ./config/crypto-config ./channel-artifacts ./system-genesis-block ./*.tar.gz
mkdir -p ./channel-artifacts ./system-genesis-block

echo "Pre-pulling chaincode build images..."
docker pull -q hyperledger/fabric-ccenv:2.5 >/dev/null
docker pull -q hyperledger/fabric-baseos:2.5 >/dev/null

################################################################################
# 1-3. Crypto material and channel artifacts
################################################################################

step "1. Generating crypto material (X.509 certificates for every org)"
cryptogen generate --config=./config/crypto-config.yaml --output="./config/crypto-config"

step "2. Generating genesis block"
configtxgen -profile WattLedgerGenesis -channelID system-channel -outputBlock ./system-genesis-block/genesis.block

step "3. Generating channel transaction for '${CHANNEL_NAME}'"
configtxgen -profile EnergyChannelProfile -outputCreateChannelTx ./channel-artifacts/${CHANNEL_NAME}.tx -channelID ${CHANNEL_NAME}

################################################################################
# 4-6. Start network, create channel, join peers
################################################################################

step "4. Starting network containers"
docker compose up -d

wait_for_orderer_leader orderer.wattledger.com 90 || {
	echo "Orderer not ready; aborting channel create." >&2
	exit 1
}

step "5. Creating channel '${CHANNEL_NAME}'"
set_peer_context Utility
peer channel create \
	-o ${ORDERER_ADDRESS} \
	--ordererTLSHostnameOverride ${ORDERER_TLS_HOSTNAME_OVERRIDE} \
	-c ${CHANNEL_NAME} \
	-f ./channel-artifacts/${CHANNEL_NAME}.tx \
	--outputBlock ./channel-artifacts/${CHANNEL_NAME}.block \
	--tls --cafile ${ORDERER_CA}

step "6. Joining peers to channel"
for ORG in $ORGS; do
	set_peer_context $ORG
	echo "Joining peer of $ORG ($CORE_PEER_ADDRESS)"
	peer channel join -b ./channel-artifacts/${CHANNEL_NAME}.block
done

################################################################################
# 7. Anchor peers (needed for cross-org discovery used by the Gateway)
################################################################################

step "7. Updating anchor peers"
for ORG in $ORGS; do
	configtxgen -profile EnergyChannelProfile \
		-outputAnchorPeersUpdate ./channel-artifacts/${ORG}MSPanchors.tx \
		-channelID ${CHANNEL_NAME} \
		-asOrg ${ORG}Org
	set_peer_context $ORG
	peer channel update \
		-f ./channel-artifacts/${ORG}MSPanchors.tx \
		-c ${CHANNEL_NAME} \
		-o ${ORDERER_ADDRESS} \
		--ordererTLSHostnameOverride ${ORDERER_TLS_HOSTNAME_OVERRIDE} \
		--tls --cafile ${ORDERER_CA} || echo "WARNING: anchor peer update for ${ORG} failed (it may already be set)."
	echo "Updated anchor peer of ${ORG}"
	sleep 1
done

################################################################################
# 8-10. Chaincode lifecycle: package, install, approve, commit
################################################################################

step "8. Packaging and installing chaincodes"
for CC in governance energy; do
	echo "Vendoring Go dependencies of ${CC}..."
	(cd ./chaincode/${CC} && go mod vendor)
	peer lifecycle chaincode package ${CC}-cc.tar.gz --path ./chaincode/${CC} --lang golang --label ${CC}-cc_${CC_VERSION}
done

for ORG in $ORGS; do
	set_peer_context $ORG
	for CC in governance energy; do
		echo "Installing ${CC}-cc on ${CORE_PEER_ADDRESS} (first build may take a few minutes)"
		peer lifecycle chaincode install ${CC}-cc.tar.gz
	done
done

set_peer_context Utility
PACKAGE_ID_GOV="$(peer lifecycle chaincode queryinstalled | sed -n "s/Package ID: \(.*\), Label: governance-cc_${CC_VERSION}/\1/p")"
PACKAGE_ID_ENE="$(peer lifecycle chaincode queryinstalled | sed -n "s/Package ID: \(.*\), Label: energy-cc_${CC_VERSION}/\1/p")"
if [ -z "$PACKAGE_ID_GOV" ] || [ -z "$PACKAGE_ID_ENE" ]; then
	echo "Failed to determine package IDs." >&2
	peer lifecycle chaincode queryinstalled || true
	exit 1
fi
echo "Governance package ID: $PACKAGE_ID_GOV"
echo "Energy package ID:     $PACKAGE_ID_ENE"

step "9. Approving chaincode definitions for each org"
for ORG in $ORGS; do
	set_peer_context $ORG
	echo "Approving governance-cc for $ORG"
	peer lifecycle chaincode approveformyorg -o ${ORDERER_ADDRESS} --ordererTLSHostnameOverride ${ORDERER_TLS_HOSTNAME_OVERRIDE} \
		--channelID ${CHANNEL_NAME} --name governance-cc --version ${CC_VERSION} --package-id ${PACKAGE_ID_GOV} --sequence ${CC_SEQUENCE} \
		--tls --cafile ${ORDERER_CA}
	echo "Approving energy-cc for $ORG"
	peer lifecycle chaincode approveformyorg -o ${ORDERER_ADDRESS} --ordererTLSHostnameOverride ${ORDERER_TLS_HOSTNAME_OVERRIDE} \
		--channelID ${CHANNEL_NAME} --name energy-cc --version ${CC_VERSION} --package-id ${PACKAGE_ID_ENE} --sequence ${CC_SEQUENCE} \
		--tls --cafile ${ORDERER_CA}
done

set_peer_context Utility
for CC in governance-cc energy-cc; do
	echo "Commit readiness of ${CC}:"
	peer lifecycle chaincode checkcommitreadiness --channelID ${CHANNEL_NAME} --name ${CC} --version ${CC_VERSION} --sequence ${CC_SEQUENCE} --output json
done

step "10. Committing chaincodes to channel"
PEER_ARGS=""
for ORG in $ORGS; do
	set_peer_context $ORG
	PEER_ARGS+=" --peerAddresses ${CORE_PEER_ADDRESS} --tlsRootCertFiles ${CORE_PEER_TLS_ROOTCERT_FILE}"
done

set_peer_context Utility
for CC in governance-cc energy-cc; do
	peer lifecycle chaincode commit -o ${ORDERER_ADDRESS} --ordererTLSHostnameOverride ${ORDERER_TLS_HOSTNAME_OVERRIDE} \
		--channelID ${CHANNEL_NAME} --name ${CC} --version ${CC_VERSION} --sequence ${CC_SEQUENCE} \
		--tls --cafile ${ORDERER_CA} ${PEER_ARGS}
	peer lifecycle chaincode querycommitted --channelID ${CHANNEL_NAME} --name ${CC}
done

################################################################################
# 11. Warm-up (starts the chaincode containers so the first demo call is fast)
################################################################################

step "11. Warming up chaincode containers"
sleep 3
for ORG in $ORGS; do
	set_peer_context $ORG
	peer chaincode query -C ${CHANNEL_NAME} -n governance-cc -c '{"Args":["GetTariffs"]}' >/dev/null && echo "governance-cc ready on ${ORG}"
	peer chaincode query -C ${CHANNEL_NAME} -n energy-cc -c '{"Args":["metering:ListAccounts"]}' >/dev/null && echo "energy-cc ready on ${ORG}"
done

step "WattLedger network deployed successfully!"
docker ps --format "table {{.Names}}\t{{.Status}}"
