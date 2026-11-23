// Package services wraps the chaincode functions used by the HTTP handlers.
package services

import (
	"api/gateway"
)

// LedgerService submits and evaluates chaincode transactions as a given organization.
type LedgerService struct {
	gw         *gateway.Gateway
	governance string
	energy     string
}

// NewLedgerService creates the service.
func NewLedgerService(gw *gateway.Gateway, governanceCC, energyCC string) *LedgerService {
	return &LedgerService{gw: gw, governance: governanceCC, energy: energyCC}
}

// Contract names inside the energy chaincode.
const (
	MeteringContract = "metering" // demonstrated MVP scope
	MarketContract   = "market"   // future capability, sandbox only
)

// submit sends a transaction: endorse -> order -> commit (writes the ledger).
func (s *LedgerService) submit(org, chaincode, contract, fn string, args ...string) ([]byte, error) {
	client, err := s.gw.Org(org)
	if err != nil {
		return nil, err
	}
	return client.Contract(chaincode, contract).SubmitTransaction(fn, args...)
}

// evaluate runs a read-only query on a peer (nothing is written).
func (s *LedgerService) evaluate(org, chaincode, contract, fn string, args ...string) ([]byte, error) {
	client, err := s.gw.Org(org)
	if err != nil {
		return nil, err
	}
	return client.Contract(chaincode, contract).EvaluateTransaction(fn, args...)
}

// ---------------- governance-cc ----------------

func (s *LedgerService) OnboardMember(org, memberID, mspID, displayName, role string) error {
	_, err := s.submit(org, s.governance, "", "OnboardMember", memberID, mspID, displayName, role)
	return err
}

func (s *LedgerService) ListMembers(org string) ([]byte, error) {
	return s.evaluate(org, s.governance, "", "ListMembers")
}

func (s *LedgerService) RegisterMeter(org, meterID, owner, consumerUnit, substation, capacityKWp, publicKeyPEM string) error {
	_, err := s.submit(org, s.governance, "", "RegisterMeter", meterID, owner, consumerUnit, substation, capacityKWp, publicKeyPEM)
	return err
}

func (s *LedgerService) DeactivateMeter(org, meterID string) error {
	_, err := s.submit(org, s.governance, "", "DeactivateMeter", meterID)
	return err
}

func (s *LedgerService) GetMeter(org, meterID string) ([]byte, error) {
	return s.evaluate(org, s.governance, "", "GetMeter", meterID)
}

func (s *LedgerService) ListMeters(org string) ([]byte, error) {
	return s.evaluate(org, s.governance, "", "ListMeters")
}

func (s *LedgerService) UpdateTariffs(org, distributor, te, tusd, fioB, fioBPct, availability, tolerance string) error {
	_, err := s.submit(org, s.governance, "", "UpdateTariffs", distributor, te, tusd, fioB, fioBPct, availability, tolerance)
	return err
}

func (s *LedgerService) GetTariffs(org string) ([]byte, error) {
	return s.evaluate(org, s.governance, "", "GetTariffs")
}

func (s *LedgerService) SetSandboxMode(org, enabled, reference string) error {
	_, err := s.submit(org, s.governance, "", "SetSandboxMode", enabled, reference)
	return err
}

func (s *LedgerService) GetSandboxConfig(org string) ([]byte, error) {
	return s.evaluate(org, s.governance, "", "GetSandboxConfig")
}

func (s *LedgerService) GetPriceBand(org string) ([]byte, error) {
	return s.evaluate(org, s.governance, "", "GetPriceBand")
}

// ---------------- energy-cc : metering (demonstrated) ----------------

func (s *LedgerService) SubmitReading(org, meterID, readingID, kwh, periodStart, periodEnd, signature string) ([]byte, error) {
	return s.submit(org, s.energy, MeteringContract, "SubmitReading", meterID, readingID, kwh, periodStart, periodEnd, signature)
}

func (s *LedgerService) ReconcileCycle(org, meterID, cycle, injected, consumed string) ([]byte, error) {
	return s.submit(org, s.energy, MeteringContract, "ReconcileCycle", meterID, cycle, injected, consumed)
}

func (s *LedgerService) SimulateBill(org, owner, cycle string) ([]byte, error) {
	return s.submit(org, s.energy, MeteringContract, "SimulateBill", owner, cycle)
}

func (s *LedgerService) AuditReport(org, cycle, substation string) ([]byte, error) {
	return s.evaluate(org, s.energy, MeteringContract, "AuditReport", cycle, substation)
}

func (s *LedgerService) GetAccount(org, owner string) ([]byte, error) {
	return s.evaluate(org, s.energy, MeteringContract, "GetAccount", owner)
}

func (s *LedgerService) ListAccounts(org string) ([]byte, error) {
	return s.evaluate(org, s.energy, MeteringContract, "ListAccounts")
}

func (s *LedgerService) GetAccountHistory(org, owner string) ([]byte, error) {
	return s.evaluate(org, s.energy, MeteringContract, "GetAccountHistory", owner)
}

func (s *LedgerService) ListReadings(org, meterID string) ([]byte, error) {
	return s.evaluate(org, s.energy, MeteringContract, "ListReadings", meterID)
}

func (s *LedgerService) ListMeterStates(org string) ([]byte, error) {
	return s.evaluate(org, s.energy, MeteringContract, "ListMeterStates")
}

func (s *LedgerService) ListReconciliations(org string) ([]byte, error) {
	return s.evaluate(org, s.energy, MeteringContract, "ListReconciliations")
}

func (s *LedgerService) ListStatements(org string) ([]byte, error) {
	return s.evaluate(org, s.energy, MeteringContract, "ListStatements")
}

// ---------------- energy-cc : market (future, sandbox only) ----------------

func (s *LedgerService) SubmitAsk(org, seller, substation, quantity, price string) ([]byte, error) {
	return s.submit(org, s.energy, MarketContract, "SubmitAsk", seller, substation, quantity, price)
}

func (s *LedgerService) SubmitBid(org, buyer, substation, quantity, price string) ([]byte, error) {
	return s.submit(org, s.energy, MarketContract, "SubmitBid", buyer, substation, quantity, price)
}

func (s *LedgerService) CancelOrder(org, orderID string) error {
	_, err := s.submit(org, s.energy, MarketContract, "CancelOrder", orderID)
	return err
}

func (s *LedgerService) MatchOrders(org, substation string) ([]byte, error) {
	return s.submit(org, s.energy, MarketContract, "MatchOrders", substation)
}

func (s *LedgerService) ListOrders(org, substation, status string) ([]byte, error) {
	return s.evaluate(org, s.energy, MarketContract, "ListOrders", substation, status)
}

func (s *LedgerService) ListTrades(org string) ([]byte, error) {
	return s.evaluate(org, s.energy, MarketContract, "ListTrades")
}
