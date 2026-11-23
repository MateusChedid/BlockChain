package chaincode

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// MeteringContract is the demonstrated scope of the MVP:
// auditable measurement, reconciliation and a simulated compensation bill.
//
// What a signed reading proves: the data came from a certified device and was
// not changed in transit. What it does NOT prove: that the sensor measured the
// physical truth. That is why every reading is (1) checked against physical
// limits, (2) reconciled with the utility revenue meter, the legal source of
// truth for billing, and (3) compared with neighbors on the same substation.
type MeteringContract struct {
	contractapi.Contract
}

// Reading statuses.
const (
	ReadingAccepted = "ACCEPTED" // plausible, waiting for reconciliation
	ReadingFlagged  = "FLAGGED"  // implausible yield, quarantined until reconciliation

	ReconReconciled  = "RECONCILED"
	ReconDiscrepancy = "DISCREPANCY"

	MeterOK         = "OK"
	MeterFlagged    = "FLAGGED"
	MeterInspection = "INSPECTION_REQUIRED"

	dateLayout = "2006-01-02"
)

// Reading is one signed batch of energy injected over a period (1 to 31 days).
type Reading struct {
	MeterID     string  `json:"meterID"`
	ReadingID   string  `json:"readingID"`
	Owner       string  `json:"owner"`
	Substation  string  `json:"substation"`
	Cycle       string  `json:"cycle"` // billing cycle YYYY-MM
	PeriodStart string  `json:"periodStart"`
	PeriodEnd   string  `json:"periodEnd"`
	Days        int     `json:"days"`
	KWh         float64 `json:"kwh"`
	YieldKWhKWp float64 `json:"yieldKWhKWpDay"` // kWh per kWp per day
	Status      string  `json:"status"`
	Reason      string  `json:"reason"`
	Reconciled  bool    `json:"reconciled"`
	TxID        string  `json:"txID"`
	RecordedAt  int64   `json:"recordedAt"`
}

// MeterState is the health record of a meter kept by the metering contract.
type MeterState struct {
	MeterID       string `json:"meterID"`
	LastPeriodEnd string `json:"lastPeriodEnd"`
	Status        string `json:"status"`
	FlagCount     int    `json:"flagCount"`
	LastReason    string `json:"lastReason"`
	UpdatedAt     int64  `json:"updatedAt"`
}

// Reconciliation compares the IoT readings of a cycle with the utility revenue meter.
type Reconciliation struct {
	MeterID            string  `json:"meterID"`
	Owner              string  `json:"owner"`
	Substation         string  `json:"substation"`
	Cycle              string  `json:"cycle"`
	CapacityKWp        float64 `json:"capacityKWp"`
	IoTKWh             float64 `json:"iotKWh"`
	UtilityInjectedKWh float64 `json:"utilityInjectedKWh"` // revenue meter, legal source of truth
	UtilityConsumedKWh float64 `json:"utilityConsumedKWh"`
	DiffPct            float64 `json:"diffPct"`
	TolerancePct       float64 `json:"tolerancePct"`
	ReadingsCount      int     `json:"readingsCount"`
	FlaggedCount       int     `json:"flaggedCount"`
	Status             string  `json:"status"`
	Note               string  `json:"note"`
	ReconciledBy       string  `json:"reconciledBy"`
	ReconciledAt       int64   `json:"reconciledAt"`
	TxID               string  `json:"txID"`
}

// Statement is a SIMULATED compensation bill (Lei 14.300), before taxes.
type Statement struct {
	StatementID      string  `json:"statementID"`
	Owner            string  `json:"owner"`
	Cycle            string  `json:"cycle"`
	Simulated        bool    `json:"simulated"`
	Distributor      string  `json:"distributor"`
	ConsumedKWh      float64 `json:"consumedKWh"`
	InjectedKWh      float64 `json:"injectedKWh"`
	CreditsBeforeKWh float64 `json:"creditsBeforeKWh"`
	CompensatedKWh   float64 `json:"compensatedKWh"`
	CreditsAfterKWh  float64 `json:"creditsAfterKWh"`
	BilledKWh        float64 `json:"billedKWh"`
	TEBRL            float64 `json:"teBRL"`
	TUSDBRL          float64 `json:"tusdBRL"`
	FioBBRL          float64 `json:"fioBBRL"`
	FioBPct          int     `json:"fioBPct"`
	EnergyChargeBRL  float64 `json:"energyChargeBRL"`
	FioBChargeBRL    float64 `json:"fioBChargeBRL"`
	TotalBRL         float64 `json:"totalBRL"`
	WithoutSolarBRL  float64 `json:"withoutSolarBRL"`
	SavingsBRL       float64 `json:"savingsBRL"`
	SandboxNetBRL    float64 `json:"sandboxNetBRL"` // sandbox only, not part of the real bill
	GeneratedBy      string  `json:"generatedBy"`
	GeneratedAt      int64   `json:"generatedAt"`
	TxID             string  `json:"txID"`
}

// AuditRow is one line of the reconciliation and peer audit report.
type AuditRow struct {
	MeterID            string   `json:"meterID"`
	Owner              string   `json:"owner"`
	Substation         string   `json:"substation"`
	CapacityKWp        float64  `json:"capacityKWp"`
	DaysCovered        int      `json:"daysCovered"`
	IoTKWh             float64  `json:"iotKWh"`
	IoTYield           float64  `json:"iotYield"`
	PeerMedianYield    float64  `json:"peerMedianYield"`
	PeerRatio          float64  `json:"peerRatio"`
	UtilityInjectedKWh float64  `json:"utilityInjectedKWh"`
	DiffPct            float64  `json:"diffPct"`
	ReconStatus        string   `json:"reconStatus"`
	MeterStatus        string   `json:"meterStatus"`
	Flags              []string `json:"flags"`
}

// AccountSnapshot is one entry of the immutable ledger history of an account.
type AccountSnapshot struct {
	TxID           string  `json:"txID"`
	Timestamp      int64   `json:"timestamp"`
	PendingKWh     float64 `json:"pendingKWh"`
	QuarantinedKWh float64 `json:"quarantinedKWh"`
	CreditKWh      float64 `json:"creditKWh"`
	Action         string  `json:"action"`
}

// ReadingMessage is the exact payload signed by the meter's secure element.
// Format: meterID|readingID|kwh|periodStart|periodEnd
func ReadingMessage(meterID, readingID, kwh, periodStart, periodEnd string) string {
	return strings.Join([]string{meterID, readingID, kwh, periodStart, periodEnd}, "|")
}

// #####################################
// #   1. Signed measurement            #
// #####################################

// SubmitReading records a signed batch of injected energy.
// Checks: certified and active meter, valid ECDSA signature, no overlap with
// periods already reported (replay), physical limit and plausibility limit.
func (m *MeteringContract) SubmitReading(
	ctx contractapi.TransactionContextInterface,
	meterID, readingID, kwh, periodStart, periodEnd, signatureB64 string,
) (*Reading, error) {
	stub := ctx.GetStub()

	amount, err := strconv.ParseFloat(kwh, 64)
	if err != nil || amount < 0 {
		return nil, fmt.Errorf("invalid kWh amount")
	}
	start, err1 := time.Parse(dateLayout, periodStart)
	end, err2 := time.Parse(dateLayout, periodEnd)
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("periods must use the YYYY-MM-DD format")
	}
	if end.Before(start) {
		return nil, fmt.Errorf("periodEnd is before periodStart")
	}
	if start.Format("2006-01") != end.Format("2006-01") {
		return nil, fmt.Errorf("a reading cannot cross billing cycles")
	}
	days := int(end.Sub(start).Hours()/24) + 1

	meter, err := fetchMeter(ctx, meterID)
	if err != nil {
		return nil, err
	}
	if !meter.Active {
		return nil, fmt.Errorf("meter %s is deactivated", meterID)
	}

	// Authenticity: the reading was produced by the certified device.
	if err := verifyReadingSignature(meter.PublicKeyPEM, ReadingMessage(meterID, readingID, kwh, periodStart, periodEnd), signatureB64); err != nil {
		return nil, err
	}

	// Replay / double counting: periods must move forward.
	state := &MeterState{MeterID: meterID, Status: MeterOK}
	if _, err := getJSON(ctx, state, MeterStateObjectType, meterID); err != nil {
		return nil, err
	}
	if state.LastPeriodEnd != "" && periodStart <= state.LastPeriodEnd {
		return nil, fmt.Errorf("replay or overlapping period rejected: meter %s already reported up to %s", meterID, state.LastPeriodEnd)
	}

	// Physical limit: a PV system cannot exceed MaxPhysicalYield kWh/kWp/day.
	yield := amount / meter.CapacityKWp / float64(days)
	if yield > MaxPhysicalYield {
		return nil, fmt.Errorf("physically impossible reading rejected: %.2f kWh/kWp/day for a %.2f kWp system (limit %.1f)", yield, meter.CapacityKWp, MaxPhysicalYield)
	}

	ts, err := stub.GetTxTimestamp()
	if err != nil {
		return nil, err
	}

	reading := &Reading{
		MeterID: meterID, ReadingID: readingID, Owner: meter.Owner, Substation: meter.Substation,
		Cycle: start.Format("2006-01"), PeriodStart: periodStart, PeriodEnd: periodEnd, Days: days,
		KWh: round3(amount), YieldKWhKWp: round3(yield), Status: ReadingAccepted,
		TxID: stub.GetTxID(), RecordedAt: ts.Seconds,
	}

	book := newAccountBook(ctx)
	acc, err := book.get(meter.Owner)
	if err != nil {
		return nil, err
	}

	// Plausibility: an authentic signature does not prove physical truth.
	switch {
	case yield > PlausibleYield:
		reading.Status = ReadingFlagged
		reading.Reason = fmt.Sprintf("yield %.2f kWh/kWp/day above the plausible %.1f", yield, PlausibleYield)
	case state.Status == MeterInspection:
		reading.Status = ReadingFlagged
		reading.Reason = "meter under inspection after a reconciliation discrepancy"
	}
	if reading.Status == ReadingFlagged {
		acc.QuarantinedKWh += reading.KWh
		state.FlagCount++
		state.LastReason = reading.Reason
		if state.Status == MeterOK {
			state.Status = MeterFlagged
		}
		book.note(meter.Owner, fmt.Sprintf("FLAGGED %.3f kWh (%s to %s): %s", reading.KWh, periodStart, periodEnd, reading.Reason))
	} else {
		acc.PendingKWh += reading.KWh
		book.note(meter.Owner, fmt.Sprintf("READING %.3f kWh (%s to %s) from %s", reading.KWh, periodStart, periodEnd, meterID))
	}

	state.LastPeriodEnd = periodEnd
	state.UpdatedAt = ts.Seconds
	if err := book.flush(); err != nil {
		return nil, err
	}
	if err := putJSON(ctx, MeterStateObjectType, []string{meterID}, state); err != nil {
		return nil, err
	}
	if err := putJSON(ctx, ReadingObjectType, []string{meterID, periodStart, readingID}, reading); err != nil {
		return nil, err
	}
	bz, _ := json.Marshal(reading)
	if err := stub.SetEvent("ReadingRecorded", bz); err != nil {
		return nil, err
	}
	return reading, nil
}

// #####################################
// #   2. Reconciliation                #
// #####################################

// ReconcileCycle compares the IoT readings of a cycle with the values read by
// the utility's revenue meter. The revenue meter is the legal source of truth:
// official credits always equal utilityInjectedKWh. A difference above the
// tolerance marks the meter for physical inspection. Only the utility may call it.
func (m *MeteringContract) ReconcileCycle(
	ctx contractapi.TransactionContextInterface, meterID, cycle, utilityInjectedKWh, utilityConsumedKWh string,
) (*Reconciliation, error) {
	callerMSP, err := requireUtility(ctx, "reconcile cycles")
	if err != nil {
		return nil, err
	}
	if _, err := time.Parse("2006-01", cycle); err != nil {
		return nil, fmt.Errorf("cycle must use the YYYY-MM format")
	}
	injected, err1 := strconv.ParseFloat(utilityInjectedKWh, 64)
	consumed, err2 := strconv.ParseFloat(utilityConsumedKWh, 64)
	if err1 != nil || err2 != nil || injected < 0 || consumed < 0 {
		return nil, fmt.Errorf("invalid revenue meter values")
	}

	existing := &Reconciliation{}
	found, err := getJSON(ctx, existing, ReconciliationObjectType, meterID, cycle)
	if err != nil {
		return nil, err
	}
	if found {
		return nil, fmt.Errorf("cycle %s of meter %s was already reconciled", cycle, meterID)
	}

	meter, err := fetchMeter(ctx, meterID)
	if err != nil {
		return nil, err
	}
	tariffs, err := fetchTariffs(ctx)
	if err != nil {
		return nil, err
	}
	readings, err := loadReadings(ctx, meterID)
	if err != nil {
		return nil, err
	}

	var iot, pending, quarantined float64
	count, flagged := 0, 0
	stub := ctx.GetStub()
	for i := range readings {
		r := &readings[i]
		if r.Cycle != cycle || r.Reconciled {
			continue
		}
		count++
		iot += r.KWh
		if r.Status == ReadingFlagged {
			flagged++
			quarantined += r.KWh
		} else {
			pending += r.KWh
		}
		r.Reconciled = true
		if err := putJSON(ctx, ReadingObjectType, []string{r.MeterID, r.PeriodStart, r.ReadingID}, r); err != nil {
			return nil, err
		}
	}

	ts, err := stub.GetTxTimestamp()
	if err != nil {
		return nil, err
	}
	rec := &Reconciliation{
		MeterID: meterID, Owner: meter.Owner, Substation: meter.Substation, Cycle: cycle,
		CapacityKWp: meter.CapacityKWp, IoTKWh: round3(iot), UtilityInjectedKWh: round3(injected),
		UtilityConsumedKWh: round3(consumed), TolerancePct: tariffs.TolerancePct,
		ReadingsCount: count, FlaggedCount: flagged, ReconciledBy: callerMSP,
		ReconciledAt: ts.Seconds, TxID: stub.GetTxID(),
	}
	switch {
	case injected == 0 && iot == 0:
		rec.DiffPct = 0
	case injected == 0:
		rec.DiffPct = 100
	default:
		rec.DiffPct = round2((iot - injected) / injected * 100)
	}

	state := &MeterState{MeterID: meterID, Status: MeterOK}
	if _, err := getJSON(ctx, state, MeterStateObjectType, meterID); err != nil {
		return nil, err
	}
	if count == 0 {
		rec.Status = ReconDiscrepancy
		rec.Note = "no IoT data for this cycle"
	} else if math.Abs(rec.DiffPct) <= tariffs.TolerancePct {
		rec.Status = ReconReconciled
		rec.Note = "IoT and revenue meter agree"
		if state.Status == MeterFlagged {
			state.Status = MeterOK // the flagged values were confirmed by the revenue meter
		}
	} else {
		rec.Status = ReconDiscrepancy
		rec.Note = fmt.Sprintf("IoT differs %.2f%% from the revenue meter: possible compromised sensor or meter fault", rec.DiffPct)
		state.Status = MeterInspection
		state.LastReason = rec.Note
	}
	state.MeterID = meterID
	state.UpdatedAt = ts.Seconds

	// Official credits always come from the revenue meter.
	book := newAccountBook(ctx)
	acc, err := book.get(meter.Owner)
	if err != nil {
		return nil, err
	}
	acc.PendingKWh = math.Max(0, acc.PendingKWh-pending)
	acc.QuarantinedKWh = math.Max(0, acc.QuarantinedKWh-quarantined)
	acc.CreditKWh += injected
	acc.InjectedKWh += injected
	book.note(meter.Owner, fmt.Sprintf("RECONCILED %s: IoT %.3f vs revenue meter %.3f kWh (%+.2f%%, %s), +%.3f kWh official credits",
		cycle, iot, injected, rec.DiffPct, rec.Status, injected))
	if err := book.flush(); err != nil {
		return nil, err
	}
	if err := putJSON(ctx, MeterStateObjectType, []string{meterID}, state); err != nil {
		return nil, err
	}
	if err := putJSON(ctx, ReconciliationObjectType, []string{meterID, cycle}, rec); err != nil {
		return nil, err
	}
	bz, _ := json.Marshal(rec)
	if err := stub.SetEvent("CycleReconciled", bz); err != nil {
		return nil, err
	}
	return rec, nil
}

// #####################################
// #   3. Simulated compensation bill   #
// #####################################

// SimulateBill builds a SIMULATED compensation statement following Lei 14.300
// (net metering with partial Fio B). It only uses reconciled revenue meter data
// and does not touch any real billing system. Only the utility may call it.
func (m *MeteringContract) SimulateBill(
	ctx contractapi.TransactionContextInterface, owner, cycle string,
) (*Statement, error) {
	callerMSP, err := requireUtility(ctx, "generate bills")
	if err != nil {
		return nil, err
	}
	existing := &Statement{}
	found, err := getJSON(ctx, existing, StatementObjectType, owner, cycle)
	if err != nil {
		return nil, err
	}
	if found {
		return nil, fmt.Errorf("statement for %s in %s already exists", owner, cycle)
	}

	recs, err := loadReconciliations(ctx)
	if err != nil {
		return nil, err
	}
	var consumed, injected float64
	n := 0
	for _, r := range recs {
		if r.Owner == owner && r.Cycle == cycle {
			consumed += r.UtilityConsumedKWh
			injected += r.UtilityInjectedKWh
			n++
		}
	}
	if n == 0 {
		return nil, fmt.Errorf("no reconciled cycle %s for %s: reconcile the meter first", cycle, owner)
	}
	t, err := fetchTariffs(ctx)
	if err != nil {
		return nil, err
	}

	book := newAccountBook(ctx)
	acc, err := book.get(owner)
	if err != nil {
		return nil, err
	}

	full := t.TEBRL + t.TUSDBRL
	creditsBefore := acc.CreditKWh // already includes this cycle's injection
	compensable := math.Max(0, consumed-t.AvailabilityKWh)
	compensated := math.Min(creditsBefore, compensable)
	billed := consumed - compensated
	energyCharge := billed * full
	fioBCharge := compensated * t.FioBBRL * float64(t.FioBPct) / 100
	total := energyCharge + fioBCharge
	withoutSolar := consumed * full
	sandboxNet := acc.SandboxPayableBRL - acc.SandboxReceivableBRL

	ts, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return nil, err
	}
	s := &Statement{
		StatementID: "STM-" + shortTxID(ctx), Owner: owner, Cycle: cycle, Simulated: true,
		Distributor: t.Distributor, ConsumedKWh: round3(consumed), InjectedKWh: round3(injected),
		CreditsBeforeKWh: round3(creditsBefore), CompensatedKWh: round3(compensated),
		CreditsAfterKWh: round3(creditsBefore - compensated), BilledKWh: round3(billed),
		TEBRL: t.TEBRL, TUSDBRL: t.TUSDBRL, FioBBRL: t.FioBBRL, FioBPct: t.FioBPct,
		EnergyChargeBRL: round2(energyCharge), FioBChargeBRL: round2(fioBCharge), TotalBRL: round2(total),
		WithoutSolarBRL: round2(withoutSolar), SavingsBRL: round2(withoutSolar - total),
		SandboxNetBRL: round2(sandboxNet), GeneratedBy: callerMSP, GeneratedAt: ts.Seconds, TxID: ctx.GetStub().GetTxID(),
	}

	acc.CreditKWh = creditsBefore - compensated
	acc.CompensatedKWh += compensated
	acc.SandboxPayableBRL = 0
	acc.SandboxReceivableBRL = 0
	book.note(owner, fmt.Sprintf("SIMULATED BILL %s: %.3f kWh compensated, total R$ %.2f (savings R$ %.2f)", cycle, compensated, s.TotalBRL, s.SavingsBRL))
	if err := book.flush(); err != nil {
		return nil, err
	}
	if err := putJSON(ctx, StatementObjectType, []string{owner, cycle}, s); err != nil {
		return nil, err
	}
	bz, _ := json.Marshal(s)
	if err := ctx.GetStub().SetEvent("BillSimulated", bz); err != nil {
		return nil, err
	}
	return s, nil
}

// #####################################
// #   4. Audit and queries             #
// #####################################

// AuditReport compares every meter of a cycle with the revenue meter and with
// its neighbors on the same substation (yield per installed kWp). A compromised
// sensor that keeps a valid signature still stands out against its peers.
func (m *MeteringContract) AuditReport(ctx contractapi.TransactionContextInterface, cycle, substation string) ([]AuditRow, error) {
	meters, err := fetchMeters(ctx)
	if err != nil {
		return nil, err
	}
	recs, err := loadReconciliations(ctx)
	if err != nil {
		return nil, err
	}
	recByMeter := map[string]Reconciliation{}
	for _, r := range recs {
		if r.Cycle == cycle {
			recByMeter[r.MeterID] = r
		}
	}

	rows := []AuditRow{}
	for _, mt := range meters {
		if substation != "" && mt.Substation != substation {
			continue
		}
		readings, err := loadReadings(ctx, mt.MeterID)
		if err != nil {
			return nil, err
		}
		row := AuditRow{MeterID: mt.MeterID, Owner: mt.Owner, Substation: mt.Substation, CapacityKWp: mt.CapacityKWp, Flags: []string{}}
		flaggedReadings := false
		for _, r := range readings {
			if r.Cycle != cycle {
				continue
			}
			row.DaysCovered += r.Days
			row.IoTKWh += r.KWh
			if r.Status == ReadingFlagged {
				flaggedReadings = true
			}
		}
		row.IoTKWh = round3(row.IoTKWh)
		if row.DaysCovered > 0 && mt.CapacityKWp > 0 {
			row.IoTYield = round3(row.IoTKWh / mt.CapacityKWp / float64(row.DaysCovered))
		}
		state := &MeterState{Status: MeterOK}
		if _, err := getJSON(ctx, state, MeterStateObjectType, mt.MeterID); err != nil {
			return nil, err
		}
		row.MeterStatus = state.Status
		if r, ok := recByMeter[mt.MeterID]; ok {
			row.UtilityInjectedKWh = r.UtilityInjectedKWh
			row.DiffPct = r.DiffPct
			row.ReconStatus = r.Status
		} else {
			row.ReconStatus = "PENDING"
		}
		if flaggedReadings {
			row.Flags = append(row.Flags, "IMPLAUSIBLE_READING")
		}
		if row.ReconStatus == ReconDiscrepancy {
			row.Flags = append(row.Flags, "REVENUE_METER_MISMATCH")
		}
		if state.Status == MeterInspection {
			row.Flags = append(row.Flags, "INSPECTION_REQUIRED")
		}
		rows = append(rows, row)
	}

	// Peer comparison per substation (median yield of the other meters).
	for i := range rows {
		var peers []float64
		for j := range rows {
			if i != j && rows[j].Substation == rows[i].Substation && rows[j].IoTYield > 0 {
				peers = append(peers, rows[j].IoTYield)
			}
		}
		if len(peers) < 2 || rows[i].IoTYield == 0 {
			continue
		}
		med := median(peers)
		rows[i].PeerMedianYield = round3(med)
		rows[i].PeerRatio = round2(rows[i].IoTYield / med)
		if rows[i].PeerRatio > 1.25 {
			rows[i].Flags = append(rows[i].Flags, "ABOVE_PEERS")
		} else if rows[i].PeerRatio < 0.7 {
			rows[i].Flags = append(rows[i].Flags, "UNDERPERFORMING")
		}
	}
	return rows, nil
}

// GetAccount returns the credit account of a holder.
func (m *MeteringContract) GetAccount(ctx contractapi.TransactionContextInterface, owner string) (*Account, error) {
	return readAccount(ctx, owner)
}

// ListAccounts returns every account.
func (m *MeteringContract) ListAccounts(ctx contractapi.TransactionContextInterface) ([]Account, error) {
	iter, err := ctx.GetStub().GetStateByPartialCompositeKey(AccountObjectType, []string{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	out := []Account{}
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		var a Account
		if err := json.Unmarshal(kv.Value, &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// GetAccountHistory reads the immutable history of an account from the ledger.
func (m *MeteringContract) GetAccountHistory(ctx contractapi.TransactionContextInterface, owner string) ([]AccountSnapshot, error) {
	key, err := ctx.GetStub().CreateCompositeKey(AccountObjectType, []string{owner})
	if err != nil {
		return nil, err
	}
	iter, err := ctx.GetStub().GetHistoryForKey(key)
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	history := []AccountSnapshot{}
	for iter.HasNext() {
		mod, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if mod.IsDelete || len(mod.Value) == 0 {
			continue
		}
		var a Account
		if err := json.Unmarshal(mod.Value, &a); err != nil {
			return nil, err
		}
		history = append(history, AccountSnapshot{
			TxID: mod.TxId, Timestamp: mod.Timestamp.GetSeconds(), PendingKWh: a.PendingKWh,
			QuarantinedKWh: a.QuarantinedKWh, CreditKWh: a.CreditKWh, Action: a.LastAction,
		})
	}
	sort.SliceStable(history, func(i, j int) bool { return history[i].Timestamp > history[j].Timestamp })
	return history, nil
}

// ListReadings returns the readings of one meter, or of every meter if meterID is empty.
func (m *MeteringContract) ListReadings(ctx contractapi.TransactionContextInterface, meterID string) ([]Reading, error) {
	readings, err := loadReadings(ctx, meterID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(readings, func(i, j int) bool { return readings[i].RecordedAt > readings[j].RecordedAt })
	return readings, nil
}

// ListMeterStates returns the health record of every meter that reported.
func (m *MeteringContract) ListMeterStates(ctx contractapi.TransactionContextInterface) ([]MeterState, error) {
	iter, err := ctx.GetStub().GetStateByPartialCompositeKey(MeterStateObjectType, []string{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	out := []MeterState{}
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		var s MeterState
		if err := json.Unmarshal(kv.Value, &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// ListReconciliations returns every reconciliation record.
func (m *MeteringContract) ListReconciliations(ctx contractapi.TransactionContextInterface) ([]Reconciliation, error) {
	recs, err := loadReconciliations(ctx)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].ReconciledAt > recs[j].ReconciledAt })
	return recs, nil
}

// ListStatements returns every simulated bill.
func (m *MeteringContract) ListStatements(ctx contractapi.TransactionContextInterface) ([]Statement, error) {
	iter, err := ctx.GetStub().GetStateByPartialCompositeKey(StatementObjectType, []string{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	out := []Statement{}
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		var s Statement
		if err := json.Unmarshal(kv.Value, &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].GeneratedAt > out[j].GeneratedAt })
	return out, nil
}

// ##########################################
// #               Helpers                  #
// ##########################################

func loadReadings(ctx contractapi.TransactionContextInterface, meterID string) ([]Reading, error) {
	attrs := []string{}
	if meterID != "" {
		attrs = []string{meterID}
	}
	iter, err := ctx.GetStub().GetStateByPartialCompositeKey(ReadingObjectType, attrs)
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	out := []Reading{}
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		var r Reading
		if err := json.Unmarshal(kv.Value, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func loadReconciliations(ctx contractapi.TransactionContextInterface) ([]Reconciliation, error) {
	iter, err := ctx.GetStub().GetStateByPartialCompositeKey(ReconciliationObjectType, []string{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	out := []Reconciliation{}
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		var r Reconciliation
		if err := json.Unmarshal(kv.Value, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func median(values []float64) float64 {
	s := append([]float64{}, values...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// verifyReadingSignature checks an ASN.1 ECDSA P-256 signature over SHA-256(message).
func verifyReadingSignature(publicKeyPEM, message, signatureB64 string) error {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return fmt.Errorf("meter public key is not valid PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("meter public key is invalid: %v", err)
	}
	ecPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("meter public key is not ECDSA")
	}
	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("signature is not valid base64")
	}
	digest := sha256.Sum256([]byte(message))
	if !ecdsa.VerifyASN1(ecPub, digest[:], sig) {
		return fmt.Errorf("invalid meter signature: data was altered or did not come from the certified device")
	}
	return nil
}
