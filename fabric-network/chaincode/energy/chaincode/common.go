// Package chaincode implements the WattLedger energy chaincode.
//
// It has two contracts that share one world state:
//   - MeteringContract ("metering")  demonstrated scope of the MVP:
//     signed readings, plausibility checks, reconciliation against the
//     utility revenue meter, peer audit and a SIMULATED compensation bill
//     following Lei 14.300 (no real billing integration).
//   - MarketContract   ("market")    FUTURE, legally dependent capability:
//     P2P trading of credits. Disabled unless the utility turns on the
//     regulatory sandbox in governance-cc.
package chaincode

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// Names and constants shared by both contracts.
const (
	GovernanceChaincodeName = "governance-cc"

	UtilityMSP = "UtilityMSP"

	// TreasuryAccount collects the sandbox brokerage fee.
	TreasuryAccount = "wattledger-treasury"

	// Physical and plausibility limits, in kWh per installed kWp per day.
	// Above MaxPhysicalYield a reading is impossible and is rejected.
	// Above PlausibleYield it is accepted but quarantined until reconciliation.
	MaxPhysicalYield = 12.0
	PlausibleYield   = 6.5

	AccountObjectType        = "Account"
	ReadingObjectType        = "Reading"
	MeterStateObjectType     = "MeterState"
	ReconciliationObjectType = "Reconciliation"
	StatementObjectType      = "Statement"
	OrderObjectType          = "Order"
	TradeObjectType          = "Trade"
)

// Account tracks the energy credits of one holder.
type Account struct {
	Owner          string  `json:"owner"`
	PendingKWh     float64 `json:"pendingKWh"`     // signed IoT readings waiting for reconciliation
	QuarantinedKWh float64 `json:"quarantinedKWh"` // readings flagged as implausible, waiting for reconciliation
	CreditKWh      float64 `json:"creditKWh"`      // official credits (utility revenue meter) available on the bill
	LockedKWh      float64 `json:"lockedKWh"`      // sandbox only: credits escrowed in sell orders
	InjectedKWh    float64 `json:"injectedKWh"`    // lifetime official injection
	CompensatedKWh float64 `json:"compensatedKWh"` // lifetime credits used on simulated bills
	// Sandbox only: money positions created by simulated P2P trades.
	SandboxReceivableBRL float64 `json:"sandboxReceivableBRL"`
	SandboxPayableBRL    float64 `json:"sandboxPayableBRL"`
	LastAction           string  `json:"lastAction"`
	UpdatedAt            int64   `json:"updatedAt"`
}

// Meter mirrors governance-cc.
type Meter struct {
	MeterID      string  `json:"meterID"`
	Owner        string  `json:"owner"`
	ConsumerUnit string  `json:"consumerUnit"`
	Substation   string  `json:"substation"`
	CapacityKWp  float64 `json:"capacityKWp"`
	PublicKeyPEM string  `json:"publicKeyPEM"`
	RegisteredBy string  `json:"registeredBy"`
	Active       bool    `json:"active"`
	CreatedAt    int64   `json:"createdAt"`
}

// TariffSchedule mirrors governance-cc.
type TariffSchedule struct {
	Distributor     string  `json:"distributor"`
	TEBRL           float64 `json:"teBRL"`
	TUSDBRL         float64 `json:"tusdBRL"`
	FioBBRL         float64 `json:"fioBBRL"`
	FioBPct         int     `json:"fioBPct"`
	AvailabilityKWh float64 `json:"availabilityKWh"`
	TolerancePct    float64 `json:"tolerancePct"`
	UpdatedBy       string  `json:"updatedBy"`
	UpdatedAt       int64   `json:"updatedAt"`
}

// SandboxConfig mirrors governance-cc.
type SandboxConfig struct {
	P2PEnabled bool   `json:"p2pEnabled"`
	Reference  string `json:"reference"`
	FeeBps     int    `json:"feeBps"`
	UpdatedBy  string `json:"updatedBy"`
	UpdatedAt  int64  `json:"updatedAt"`
}

// PriceBand mirrors governance-cc.
type PriceBand struct {
	FloorBRL   float64 `json:"floorBRL"`
	CeilingBRL float64 `json:"ceilingBRL"`
	FeeBps     int     `json:"feeBps"`
}

// ##########################################
// #  Account book (transaction-local cache) #
// ##########################################

// accountBook caches accounts during a transaction, because Fabric's
// GetState does not see writes made earlier in the same transaction.
type accountBook struct {
	ctx     contractapi.TransactionContextInterface
	cache   map[string]*Account
	actions map[string][]string
	order   []string
}

func newAccountBook(ctx contractapi.TransactionContextInterface) *accountBook {
	return &accountBook{ctx: ctx, cache: map[string]*Account{}, actions: map[string][]string{}}
}

func (b *accountBook) get(owner string) (*Account, error) {
	if a, ok := b.cache[owner]; ok {
		return a, nil
	}
	a, err := readAccount(b.ctx, owner)
	if err != nil {
		return nil, err
	}
	b.cache[owner] = a
	b.order = append(b.order, owner)
	return a, nil
}

func (b *accountBook) note(owner, action string) {
	b.actions[owner] = append(b.actions[owner], action)
}

func (b *accountBook) flush() error {
	ts, err := b.ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return err
	}
	for _, owner := range b.order {
		a := b.cache[owner]
		a.PendingKWh = round3(a.PendingKWh)
		a.QuarantinedKWh = round3(a.QuarantinedKWh)
		a.CreditKWh = round3(a.CreditKWh)
		a.LockedKWh = round3(a.LockedKWh)
		a.InjectedKWh = round3(a.InjectedKWh)
		a.CompensatedKWh = round3(a.CompensatedKWh)
		a.SandboxReceivableBRL = round2(a.SandboxReceivableBRL)
		a.SandboxPayableBRL = round2(a.SandboxPayableBRL)
		if len(b.actions[owner]) > 0 {
			a.LastAction = strings.Join(b.actions[owner], " | ")
		}
		a.UpdatedAt = ts.Seconds
		if err := putJSON(b.ctx, AccountObjectType, []string{owner}, a); err != nil {
			return err
		}
	}
	return nil
}

func readAccount(ctx contractapi.TransactionContextInterface, owner string) (*Account, error) {
	key, err := ctx.GetStub().CreateCompositeKey(AccountObjectType, []string{owner})
	if err != nil {
		return nil, err
	}
	bz, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, err
	}
	if bz == nil {
		return &Account{Owner: owner}, nil
	}
	var a Account
	if err := json.Unmarshal(bz, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// #####################################
// #   Cross-chaincode governance reads #
// #####################################

func invokeGovernance(ctx contractapi.TransactionContextInterface, fn string, args ...string) ([]byte, error) {
	ccArgs := [][]byte{[]byte(fn)}
	for _, a := range args {
		ccArgs = append(ccArgs, []byte(a))
	}
	resp := ctx.GetStub().InvokeChaincode(GovernanceChaincodeName, ccArgs, "")
	if resp.Status != 200 {
		return nil, fmt.Errorf("%s", resp.Message)
	}
	return resp.Payload, nil
}

func fetchJSON(ctx contractapi.TransactionContextInterface, out interface{}, fn string, args ...string) error {
	payload, err := invokeGovernance(ctx, fn, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("failed to decode %s: %v", fn, err)
	}
	return nil
}

func fetchMeter(ctx contractapi.TransactionContextInterface, meterID string) (*Meter, error) {
	var m Meter
	if err := fetchJSON(ctx, &m, "GetMeter", meterID); err != nil {
		return nil, err
	}
	return &m, nil
}

func fetchMeters(ctx contractapi.TransactionContextInterface) ([]Meter, error) {
	var ms []Meter
	if err := fetchJSON(ctx, &ms, "ListMeters"); err != nil {
		return nil, err
	}
	return ms, nil
}

func fetchTariffs(ctx contractapi.TransactionContextInterface) (*TariffSchedule, error) {
	var t TariffSchedule
	if err := fetchJSON(ctx, &t, "GetTariffs"); err != nil {
		return nil, err
	}
	return &t, nil
}

func fetchSandbox(ctx contractapi.TransactionContextInterface) (*SandboxConfig, error) {
	var c SandboxConfig
	if err := fetchJSON(ctx, &c, "GetSandboxConfig"); err != nil {
		return nil, err
	}
	return &c, nil
}

func fetchPriceBand(ctx contractapi.TransactionContextInterface) (*PriceBand, error) {
	var b PriceBand
	if err := fetchJSON(ctx, &b, "GetPriceBand"); err != nil {
		return nil, err
	}
	return &b, nil
}

// #############################
// #          Helpers          #
// #############################

func putJSON(ctx contractapi.TransactionContextInterface, objectType string, attrs []string, v interface{}) error {
	key, err := ctx.GetStub().CreateCompositeKey(objectType, attrs)
	if err != nil {
		return err
	}
	bz, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return ctx.GetStub().PutState(key, bz)
}

func getJSON(ctx contractapi.TransactionContextInterface, out interface{}, objectType string, attrs ...string) (bool, error) {
	key, err := ctx.GetStub().CreateCompositeKey(objectType, attrs)
	if err != nil {
		return false, err
	}
	bz, err := ctx.GetStub().GetState(key)
	if err != nil || bz == nil {
		return false, err
	}
	return true, json.Unmarshal(bz, out)
}

func getCallerMSP(ctx contractapi.TransactionContextInterface) (string, error) {
	return ctx.GetClientIdentity().GetMSPID()
}

func requireUtility(ctx contractapi.TransactionContextInterface, action string) (string, error) {
	msp, err := getCallerMSP(ctx)
	if err != nil {
		return "", err
	}
	if msp != UtilityMSP {
		return "", fmt.Errorf("access denied: only %s can %s (caller %s)", UtilityMSP, action, msp)
	}
	return msp, nil
}

func shortTxID(ctx contractapi.TransactionContextInterface) string {
	id := ctx.GetStub().GetTxID()
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }
