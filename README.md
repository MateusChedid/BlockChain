# WattLedger MVP (PD3)

Auditable measurement and reconciliation of distributed solar energy on a
permissioned Hyperledger Fabric consortium, with a **simulated** compensation
bill under Lei 14.300.

INE5458 · Blockchain and Cryptocurrencies Technologies · UFSC
Gabriel Sartori Rangel · Julia Macedo de Castro · Mateus Goulart Chedid · Roberto Vandresen Neto

---

## Scope, demonstrated vs. future

The PD2 feedback asked us to scope the MVP to auditable measurement and
reconciliation with simulated billing, and to separate what we demonstrate
from what depends on regulation. The code follows that split.

| Demonstrated in this MVP | Where |
|---|---|
| Certified IoT meters with ECDSA P-256 keys (public key and PV capacity on-chain) | `governance-cc` RegisterMeter |
| Signed weekly batches, replay and overlap protection | `energy-cc` metering SubmitReading |
| Physical limit (12 kWh/kWp/day, rejected) and plausibility limit (6.5, quarantined) | SubmitReading |
| Reconciliation against the utility revenue meter (legal source of truth, ±5% tolerance) | ReconcileCycle |
| Peer audit of yield per kWp on the same substation | AuditReport |
| Simulated compensation bill with local tariffs and the 2026 Fio B rule (60%) | SimulateBill |
| MSP-based access control and an immutable audit trail | every function |

| Future, legally or contractually dependent | Status in the code |
|---|---|
| P2P sale or transfer of credits between holders (not allowed today by Lei 14.300 art. 28 and REN ANEEL 1.059/2023) | `energy-cc` market contract, refused unless the utility enables the regulatory sandbox flag |
| Integration with the real billing system | not implemented, the bill is simulated |
| Meter data as legal evidence (INMETRO-certified hardware, metering agreement) | simulated meter HSM |
| Retail offers after the free-market opening (Lei 15.269/2025) | not implemented |

**What a signature proves.** A valid signature proves that the data came from
a certified device and was not changed in transit. It does not prove that the
sensor measured the physical truth. That is why every batch also goes through
the physical and plausibility limits, the reconciliation with the revenue
meter and the peer audit. Official credits always come from the revenue meter.

---

## Architecture

```
 Web portal (React, :8080) ──/api──► REST API (Go/Gin, :3000) ──Fabric Gateway (gRPC/TLS)──► peers
   Consortium Console                 · 1 identity per org (X-Org header)
   Household App                      · IoT meter simulator (ECDSA HSM + physical truth)
                                      · simulated utility revenue meter

 Hyperledger Fabric 2.5 · channel "energychannel" · endorsement MAJORITY (2 of 3)
   peer0.utility.wattledger.com:7051      UtilityMSP     (distribution utility)
   peer0.installer.wattledger.com:9051    InstallerMSP   (certified installers)
   peer0.consortium.wattledger.com:11051  ConsortiumMSP  (platform operator)
   orderer.wattledger.com:7050            OrdererMSP     (Raft)

 Chaincodes (Go)
   governance-cc  members, meters, tariffs, sandbox switch
   energy-cc      "metering" contract (demonstrated) + "market" contract (future, sandbox only)
```

---

## Setup (once per machine)

Tested on Linux. On Windows use WSL2 with Docker Desktop. Keep about 8 GB of
RAM free for Docker. All commands below run from the project root (the folder
that contains this README).

### 1. Docker without sudo

Install Docker and Docker Compose v2, then add your user to the `docker` group
so the scripts can reach the Docker daemon.

```bash
sudo usermod -aG docker $USER
newgrp docker              # applies to this terminal; log out and in to apply everywhere
docker run --rm hello-world
```

Do not run the scripts with `sudo` and do not `chmod 666` the Docker socket.
Files created as root break later steps, and an open socket lets any local
user control Docker.

### 2. Go 1.23 or newer

The deploy script runs `go mod vendor` to package the chaincodes. The Ubuntu
`apt` package is usually too old, so install the official release
(use `arm64` instead of `amd64` if `uname -m` prints `aarch64`).

```bash
cd ~
wget https://go.dev/dl/go1.23.4.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.23.4.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

### 3. Hyperledger Fabric 2.5 binaries and images

Run this in the project root. It creates `bin/` (`peer`, `configtxgen`,
`cryptogen`) and pulls the Fabric Docker images. Both are ignored by git.

```bash
curl -sSLO https://raw.githubusercontent.com/hyperledger/fabric/main/scripts/install-fabric.sh
chmod +x install-fabric.sh
./install-fabric.sh --fabric-version 2.5.14 binary docker
./bin/peer version
docker images | grep hyperledger     # fabric-peer, fabric-orderer, fabric-ccenv, fabric-baseos
```

### 4. Network hostnames in `/etc/hosts`

The API and the CLI reach the peers by name, so the four hostnames must point
to `127.0.0.1`. This command appends them without touching the rest of the file.

```bash
echo "127.0.0.1   orderer.wattledger.com
127.0.0.1   peer0.utility.wattledger.com
127.0.0.1   peer0.installer.wattledger.com
127.0.0.1   peer0.consortium.wattledger.com" | sudo tee -a /etc/hosts
```

To edit by hand instead, run `sudo nano /etc/hosts`, paste the four lines at
the end, save with `Ctrl+O` and `Enter`, and exit with `Ctrl+X`. Check the
result with `tail -5 /etc/hosts`.

On WSL2, Windows may rewrite `/etc/hosts` when WSL restarts. If the lines are
gone, add them again before deploying.

### 5. Free ports

The project uses ports 3000 (API), 8080 (portal), 7050, 7051, 9051 and 11051
(Fabric). Grafana and other dev tools also use 3000 by default. Check before
starting.

```bash
sudo ss -ltnp | grep -E ':(3000|8080|7050|7051|9051|11051) '
docker ps --format "table {{.Names}}\t{{.Ports}}"
```

Stop whatever is listed (`docker stop <name>` or `sudo systemctl stop grafana-server`).

---

## Run

The order matters. The API and the portal join the `fabric-network` Docker
network, which only exists after the deploy.

```bash
./start-all.sh
```

`start-all.sh` runs the three steps below in order and stops at the first
error. To run them one at a time,

```bash
./fabric-network/deploy-network.sh   # network, channel, anchor peers, both chaincodes (5 to 10 min the first time)
docker compose up -d --build         # API + web
./scripts/seed-demo.sh               # members, tariffs, households, meters, 3 weeks of readings
```

* Web portal at http://localhost:8080
* API at http://localhost:3000 (Postman collection in `api/postman/`)

Quick check after the run

```bash
docker network ls | grep fabric-network
docker ps --format "table {{.Names}}\t{{.Status}}"   # orderer, 3 peers, dev-peer0..., wattledger-api, wattledger-web
curl localhost:3000/api/governance/tariffs           # JSON with teBRL 0.32193
```

The billing cycle is the current month, so run the seed and record the demo in
the same month.

Reset everything (always both commands)

```bash
docker compose down -v
./fabric-network/remove-network.sh
```

### If port 3000 must stay in use

Change the API mapping in `docker-compose.yml` from `"3000:3000"` to
`"3001:3000"` and pass the new address to the seed. The portal keeps working
because it talks to the API inside the Docker network.

```bash
docker compose up -d --build
./scripts/seed-demo.sh http://localhost:3001
```

---

## API overview

Send `X-Org: utility | installer | consortium` to choose the signing organization.

| Method | Path | Chaincode |
|---|---|---|
| GET/POST | `/api/governance/members` | ListMembers / OnboardMember |
| GET/PUT | `/api/governance/tariffs` | GetTariffs / UpdateTariffs |
| GET/PUT | `/api/governance/sandbox` | GetSandboxConfig / SetSandboxMode |
| GET/POST | `/api/meters` | ListMeters / RegisterMeter |
| POST | `/api/meters/:id/readings` `{scenario}` | SubmitReading (honest, compromised, gross, impossible, tamper, forged, replay) |
| GET | `/api/readings`, `/api/meter-states`, `/api/simulator/:id` | ListReadings, ListMeterStates, simulator |
| GET/POST | `/api/reconciliations` | ListReconciliations / ReconcileCycle (revenue meter simulated) |
| GET | `/api/audit?cycle=&substation=` | AuditReport |
| POST/GET | `/api/billing/simulate`, `/api/billing/statements` | SimulateBill / ListStatements |
| GET | `/api/accounts`, `/api/accounts/:owner`, `/api/accounts/:owner/history` | ListAccounts, GetAccount, GetAccountHistory |
| * | `/api/sandbox/...` | market contract (future, refused unless sandbox is on) |
| GET/POST | `/api/users` | off-chain household profiles |

## Reference values (editable on-chain by the utility)

* TE R$ 0.32193/kWh and TUSD R$ 0.37375/kWh, Celesc B1 residential, before taxes (bills of late 2025).
* Fio B R$ 0.20/kWh is an **assumption** to be replaced by the utility's value.
* Fio B not compensated 60% in 2026 (Lei 14.300, art. 27), minimum billed 50 kWh, tolerance ±5%.
* Taxes (ICMS, PIS/COFINS), tariff flags and public lighting are not included in the simulated bill.

## Other MVP simplifications

* Single Raft orderer (SmartBFT is available in Fabric v3).
* LevelDB world state, no Private Data Collections yet (personal data stays off-chain).
* The revenue meter and the meter HSM are simulated inside the API.

---

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| `permission denied while trying to connect to the Docker daemon socket` | Your user is not in the `docker` group. See Setup step 1. If it persists, start the daemon with `sudo systemctl start docker`, or on WSL2 enable your distro in Docker Desktop under Settings, Resources, WSL Integration. |
| `ERROR: 'go' not found in PATH` | Go is not installed or not in the PATH. See Setup step 2 and open a new terminal or run `source ~/.bashrc`. |
| `ERROR: 'peer' not found in PATH` or `peer: command not found` | `install-fabric.sh` was not run in the project root, so `bin/` is missing. See Setup step 3. |
| `network fabric-network declared as external, but could not be found` | `docker compose up` ran before the deploy, or the deploy failed. Run `./fabric-network/deploy-network.sh` first and check that it ends without `ERROR`. |
| Seed answers `Unauthorized` with `messageId: auth.unauthorized` | Another service (usually Grafana) is on port 3000, not the WattLedger API. See Setup step 5. |
| Seed prints empty answers, or `localhost:8080` refuses the connection | The API and web containers are not running. Check `docker ps`, then `docker logs wattledger-api`. Usually the Fabric network is not up. |
| API cannot reach the peers | A hostname is missing in `/etc/hosts` (Setup step 4), or the `fabric-network` Docker network does not exist. |
| Deploy is slow or stops at chaincode install | The first install downloads Go modules and builds inside `fabric-ccenv`. Check the internet connection, run `./fabric-network/remove-network.sh` and deploy again. |
| A reading is rejected as replay after restarting | The simulator cursor lives in the `api-data` volume. Reset with both commands and seed again. |
| Reconciliation finds no readings | The seed ran in a different month. Reset and seed again. |
| Unexpected `access denied` | Check the organization in the portal selector or the `X-Org` header in Postman. |

Logs are in `docker logs wattledger-api`, and the chaincode logs are in
`docker logs <dev-peer0... container>`.
