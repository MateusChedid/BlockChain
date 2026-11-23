// Package chaincode implements the WattLedger GovernanceChaincode.
//
// It holds the configuration that every consortium member must agree on:
//   - consortium members (utility, certified installers, platform operator)
//   - the registry of certified IoT meters (owner, consumer unit, capacity, public key)
//   - the reference tariff schedule used by the simulated compensation bill
//   - the regulatory sandbox switch that keeps P2P trading disabled by default
//
// Every function is a signed Fabric transaction, so each administrative
// change is auditable by all members.
package chaincode

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// GovernanceChaincode defines the chaincode structure.
type GovernanceChaincode struct {
	contractapi.Contract
}

// MSP identifiers of the consortium organizations.
const (
	UtilityMSP    = "UtilityMSP"    // Distribution utility (DSO), owner of the revenue meters and the bill
	InstallerMSP  = "InstallerMSP"  // Certified solar installers, who provision the IoT meters
	ConsortiumMSP = "ConsortiumMSP" // Platform operator
)

// Object types used in composite keys.
const (
	MemberObjectType  = "Member"
	MeterObjectType   = "Meter"
	TariffObjectType  = "Tariff"
	SandboxObjectType = "Sandbox"
	DefaultKey        = "default"
)

// Reference values (Celesc B1 residential, before taxes, late 2025 bills).
// The Fio B value is an assumption and must be replaced by the utility.
const (
	DefaultDistributor   = "Celesc-DIS (reference values)"
	DefaultTE            = 0.32193 // R$/kWh
	DefaultTUSD          = 0.37375 // R$/kWh
	DefaultFioB          = 0.20    // R$/kWh, part of TUSD (assumption)
	DefaultFioBPct       = 60      // % of Fio B not compensated in 2026 (Lei 14.300, art. 27)
	DefaultAvailability  = 50      // kWh minimum billed (two-phase connection)
	DefaultTolerancePct  = 5.0     // max difference between IoT and revenue meter
	DefaultSandboxFeeBps = 100     // 1% fee, only used inside the sandbox
)

// Member is a consortium organization.
type Member struct {
	MemberID    string `json:"memberID"`
	MSPID       string `json:"mspID"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Active      bool   `json:"active"`
	CreatedAt   int64  `json:"createdAt"`
	OnboardedBy string `json:"onboardedBy"`
}

// Meter is a certified IoT meter installed at a prosumer's consumer unit.
type Meter struct {
	MeterID      string  `json:"meterID"`
	Owner        string  `json:"owner"`        // account (holder) that receives the credits
	ConsumerUnit string  `json:"consumerUnit"` // utility account number (UC)
	Substation   string  `json:"substation"`
	CapacityKWp  float64 `json:"capacityKWp"` // installed PV capacity, used by the plausibility checks
	PublicKeyPEM string  `json:"publicKeyPEM"`
	RegisteredBy string  `json:"registeredBy"`
	Active       bool    `json:"active"`
	CreatedAt    int64   `json:"createdAt"`
}

// TariffSchedule holds the reference tariff used by the simulated bill.
type TariffSchedule struct {
	Distributor     string  `json:"distributor"`
	TEBRL           float64 `json:"teBRL"`           // energy tariff, R$/kWh
	TUSDBRL         float64 `json:"tusdBRL"`         // distribution tariff, R$/kWh
	FioBBRL         float64 `json:"fioBBRL"`         // Fio B component of TUSD, R$/kWh
	FioBPct         int     `json:"fioBPct"`         // % of Fio B not compensated this year
	AvailabilityKWh float64 `json:"availabilityKWh"` // minimum billed energy
	TolerancePct    float64 `json:"tolerancePct"`    // reconciliation tolerance
	UpdatedBy       string  `json:"updatedBy"`
	UpdatedAt       int64   `json:"updatedAt"`
}

// SandboxConfig controls the legally dependent P2P module.
type SandboxConfig struct {
	P2PEnabled bool   `json:"p2pEnabled"`
	Reference  string `json:"reference"` // regulatory authorization reference
	FeeBps     int    `json:"feeBps"`
	UpdatedBy  string `json:"updatedBy"`
	UpdatedAt  int64  `json:"updatedAt"`
}

// PriceBand is the corridor a sandbox trade must respect, derived from the tariffs.
type PriceBand struct {
	FloorBRL   float64 `json:"floorBRL"`   // credit value after the non-compensated Fio B, + R$0.01
	CeilingBRL float64 `json:"ceilingBRL"` // full tariff (TE + TUSD) - R$0.02
	FeeBps     int     `json:"feeBps"`
}

// #############################
// #          Members          #
// #############################

// OnboardMember admits a new organization. Only ConsortiumMSP may call it.
func (g *GovernanceChaincode) OnboardMember(
	ctx contractapi.TransactionContextInterface, memberID, mspID, displayName, role string,
) error {
	callerMSP, err := getCallerMSP(ctx)
	if err != nil {
		return err
	}
	if callerMSP != ConsortiumMSP {
		return fmt.Errorf("access denied: only %s can onboard members (caller %s)", ConsortiumMSP, callerMSP)
	}
	memberID = strings.TrimSpace(memberID)
	mspID = strings.TrimSpace(mspID)
	if memberID == "" || mspID == "" {
		return fmt.Errorf("memberID and mspID are required")
	}
	key, err := ctx.GetStub().CreateCompositeKey(MemberObjectType, []string{memberID})
	if err != nil {
		return err
	}
	existing, err := ctx.GetStub().GetState(key)
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("member %s already exists", memberID)
	}
	ts, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return err
	}
	bz, _ := json.Marshal(Member{
		MemberID: memberID, MSPID: mspID, DisplayName: displayName, Role: role,
		Active: true, CreatedAt: ts.Seconds, OnboardedBy: callerMSP,
	})
	return ctx.GetStub().PutState(key, bz)
}

// ListMembers returns all consortium members.
func (g *GovernanceChaincode) ListMembers(ctx contractapi.TransactionContextInterface) ([]Member, error) {
	iter, err := ctx.GetStub().GetStateByPartialCompositeKey(MemberObjectType, []string{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	members := []Member{}
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		var m Member
		if err := json.Unmarshal(kv.Value, &m); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, nil
}

// #############################
// #       IoT meters          #
// #############################

// RegisterMeter certifies an IoT meter and stores its ECDSA public key.
// Only the utility or a certified installer may register meters.
func (g *GovernanceChaincode) RegisterMeter(
	ctx contractapi.TransactionContextInterface,
	meterID, owner, consumerUnit, substation, capacityKWp, publicKeyPEM string,
) error {
	callerMSP, err := getCallerMSP(ctx)
	if err != nil {
		return err
	}
	if callerMSP != UtilityMSP && callerMSP != InstallerMSP {
		return fmt.Errorf("access denied: only %s or %s can register meters (caller %s)", UtilityMSP, InstallerMSP, callerMSP)
	}
	meterID = strings.TrimSpace(meterID)
	owner = strings.TrimSpace(owner)
	consumerUnit = strings.TrimSpace(consumerUnit)
	substation = strings.TrimSpace(substation)
	if meterID == "" || owner == "" || consumerUnit == "" || substation == "" {
		return fmt.Errorf("meterID, owner, consumerUnit and substation are required")
	}
	capacity, err := strconv.ParseFloat(capacityKWp, 64)
	if err != nil || capacity <= 0 || capacity > 75 {
		return fmt.Errorf("capacityKWp must be between 0 and 75 kWp (microgeneration)")
	}
	if err := validatePublicKey(publicKeyPEM); err != nil {
		return err
	}
	key, err := ctx.GetStub().CreateCompositeKey(MeterObjectType, []string{meterID})
	if err != nil {
		return err
	}
	existing, err := ctx.GetStub().GetState(key)
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("meter %s already registered", meterID)
	}
	ts, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return err
	}
	bz, _ := json.Marshal(Meter{
		MeterID: meterID, Owner: owner, ConsumerUnit: consumerUnit, Substation: substation,
		CapacityKWp: round4(capacity), PublicKeyPEM: publicKeyPEM, RegisteredBy: callerMSP,
		Active: true, CreatedAt: ts.Seconds,
	})
	if err := ctx.GetStub().PutState(key, bz); err != nil {
		return err
	}
	return ctx.GetStub().SetEvent("MeterRegistered", []byte(meterID))
}

// DeactivateMeter revokes a meter. Only the utility may do it.
func (g *GovernanceChaincode) DeactivateMeter(ctx contractapi.TransactionContextInterface, meterID string) error {
	callerMSP, err := getCallerMSP(ctx)
	if err != nil {
		return err
	}
	if callerMSP != UtilityMSP {
		return fmt.Errorf("access denied: only %s can deactivate meters (caller %s)", UtilityMSP, callerMSP)
	}
	m, err := g.GetMeter(ctx, meterID)
	if err != nil {
		return err
	}
	m.Active = false
	key, err := ctx.GetStub().CreateCompositeKey(MeterObjectType, []string{meterID})
	if err != nil {
		return err
	}
	bz, _ := json.Marshal(m)
	return ctx.GetStub().PutState(key, bz)
}

// GetMeter returns a registered meter (also queried by energy-cc).
func (g *GovernanceChaincode) GetMeter(ctx contractapi.TransactionContextInterface, meterID string) (*Meter, error) {
	key, err := ctx.GetStub().CreateCompositeKey(MeterObjectType, []string{meterID})
	if err != nil {
		return nil, err
	}
	bz, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, err
	}
	if bz == nil {
		return nil, fmt.Errorf("meter %s is not registered", meterID)
	}
	var m Meter
	if err := json.Unmarshal(bz, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// ListMeters returns every registered meter.
func (g *GovernanceChaincode) ListMeters(ctx contractapi.TransactionContextInterface) ([]Meter, error) {
	iter, err := ctx.GetStub().GetStateByPartialCompositeKey(MeterObjectType, []string{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	meters := []Meter{}
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		var m Meter
		if err := json.Unmarshal(kv.Value, &m); err != nil {
			return nil, err
		}
		meters = append(meters, m)
	}
	sort.Slice(meters, func(i, j int) bool { return meters[i].MeterID < meters[j].MeterID })
	return meters, nil
}

// #############################
// #         Tariffs           #
// #############################

// UpdateTariffs records the reference tariff schedule. Only the utility may call it.
func (g *GovernanceChaincode) UpdateTariffs(
	ctx contractapi.TransactionContextInterface,
	distributor, teBRL, tusdBRL, fioBBRL, fioBPct, availabilityKWh, tolerancePct string,
) error {
	callerMSP, err := getCallerMSP(ctx)
	if err != nil {
		return err
	}
	if callerMSP != UtilityMSP {
		return fmt.Errorf("access denied: only %s can update tariffs (caller %s)", UtilityMSP, callerMSP)
	}
	te, err1 := strconv.ParseFloat(teBRL, 64)
	tusd, err2 := strconv.ParseFloat(tusdBRL, 64)
	fioB, err3 := strconv.ParseFloat(fioBBRL, 64)
	pct, err4 := strconv.Atoi(fioBPct)
	avail, err5 := strconv.ParseFloat(availabilityKWh, 64)
	tol, err6 := strconv.ParseFloat(tolerancePct, 64)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || err6 != nil {
		return fmt.Errorf("invalid tariff values")
	}
	if te <= 0 || tusd <= 0 || fioB < 0 || fioB > tusd {
		return fmt.Errorf("tariffs must be positive and Fio B cannot exceed TUSD")
	}
	if pct < 0 || pct > 100 || avail < 0 || tol <= 0 || tol > 50 {
		return fmt.Errorf("fioBPct must be 0-100, availability >= 0 and tolerance between 0 and 50%%")
	}
	ts, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return err
	}
	if strings.TrimSpace(distributor) == "" {
		distributor = DefaultDistributor
	}
	t := TariffSchedule{
		Distributor: distributor, TEBRL: round5(te), TUSDBRL: round5(tusd), FioBBRL: round5(fioB),
		FioBPct: pct, AvailabilityKWh: avail, TolerancePct: tol, UpdatedBy: callerMSP, UpdatedAt: ts.Seconds,
	}
	key, err := ctx.GetStub().CreateCompositeKey(TariffObjectType, []string{DefaultKey})
	if err != nil {
		return err
	}
	bz, _ := json.Marshal(t)
	if err := ctx.GetStub().PutState(key, bz); err != nil {
		return err
	}
	return ctx.GetStub().SetEvent("TariffsUpdated", bz)
}

// GetTariffs returns the tariff schedule (reference defaults if never set).
func (g *GovernanceChaincode) GetTariffs(ctx contractapi.TransactionContextInterface) (*TariffSchedule, error) {
	key, err := ctx.GetStub().CreateCompositeKey(TariffObjectType, []string{DefaultKey})
	if err != nil {
		return nil, err
	}
	bz, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, err
	}
	if bz == nil {
		return &TariffSchedule{
			Distributor: DefaultDistributor, TEBRL: DefaultTE, TUSDBRL: DefaultTUSD, FioBBRL: DefaultFioB,
			FioBPct: DefaultFioBPct, AvailabilityKWh: DefaultAvailability, TolerancePct: DefaultTolerancePct,
			UpdatedBy: "default",
		}, nil
	}
	var t TariffSchedule
	if err := json.Unmarshal(bz, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// #####################################
// #   Regulatory sandbox (future)     #
// #####################################

// SetSandboxMode enables or disables the P2P trading simulation.
// Under Lei 14.300 and REN 1.059 credits cannot be sold or moved to another
// holder, so this switch stays off unless a regulatory sandbox authorizes it.
// Only the utility may change it.
func (g *GovernanceChaincode) SetSandboxMode(
	ctx contractapi.TransactionContextInterface, enabled, reference string,
) error {
	callerMSP, err := getCallerMSP(ctx)
	if err != nil {
		return err
	}
	if callerMSP != UtilityMSP {
		return fmt.Errorf("access denied: only %s can change the sandbox mode (caller %s)", UtilityMSP, callerMSP)
	}
	on, err := strconv.ParseBool(enabled)
	if err != nil {
		return fmt.Errorf("enabled must be true or false")
	}
	if on && strings.TrimSpace(reference) == "" {
		return fmt.Errorf("a regulatory authorization reference is required to enable the sandbox")
	}
	ts, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return err
	}
	cfg := SandboxConfig{P2PEnabled: on, Reference: reference, FeeBps: DefaultSandboxFeeBps, UpdatedBy: callerMSP, UpdatedAt: ts.Seconds}
	key, err := ctx.GetStub().CreateCompositeKey(SandboxObjectType, []string{DefaultKey})
	if err != nil {
		return err
	}
	bz, _ := json.Marshal(cfg)
	if err := ctx.GetStub().PutState(key, bz); err != nil {
		return err
	}
	return ctx.GetStub().SetEvent("SandboxModeChanged", bz)
}

// GetSandboxConfig returns the sandbox switch (disabled by default).
func (g *GovernanceChaincode) GetSandboxConfig(ctx contractapi.TransactionContextInterface) (*SandboxConfig, error) {
	key, err := ctx.GetStub().CreateCompositeKey(SandboxObjectType, []string{DefaultKey})
	if err != nil {
		return nil, err
	}
	bz, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, err
	}
	if bz == nil {
		return &SandboxConfig{P2PEnabled: false, Reference: "", FeeBps: DefaultSandboxFeeBps, UpdatedBy: "default"}, nil
	}
	var cfg SandboxConfig
	if err := json.Unmarshal(bz, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// GetPriceBand derives the sandbox price corridor from the tariffs.
func (g *GovernanceChaincode) GetPriceBand(ctx contractapi.TransactionContextInterface) (*PriceBand, error) {
	t, err := g.GetTariffs(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := g.GetSandboxConfig(ctx)
	if err != nil {
		return nil, err
	}
	full := t.TEBRL + t.TUSDBRL
	creditValue := full - t.FioBBRL*float64(t.FioBPct)/100
	return &PriceBand{FloorBRL: round4(creditValue + 0.01), CeilingBRL: round4(full - 0.02), FeeBps: cfg.FeeBps}, nil
}

// #############################
// #          Helpers          #
// #############################

func validatePublicKey(publicKeyPEM string) error {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return fmt.Errorf("invalid public key: PEM block not found")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("invalid public key: %v", err)
	}
	if _, ok := pub.(*ecdsa.PublicKey); !ok {
		return fmt.Errorf("invalid public key: meters must use ECDSA keys")
	}
	return nil
}

func getCallerMSP(ctx contractapi.TransactionContextInterface) (string, error) {
	return ctx.GetClientIdentity().GetMSPID()
}

func round4(v float64) float64 { return math.Round(v*10000) / 10000 }
func round5(v float64) float64 { return math.Round(v*100000) / 100000 }
