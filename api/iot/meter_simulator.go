// Package iot simulates the field side of the MVP:
//
//   - the secure element (HSM) of each certified IoT meter, which holds an
//     ECDSA P-256 private key and signs every batch of readings;
//   - the physical truth (the energy the PV system really injected), which the
//     simulator knows and the ledger does not;
//   - the utility revenue meter, which reads that physical truth (plus a small
//     measurement error) at the end of the billing cycle.
//
// Keeping the physical truth separate from what the IoT meter reports lets the
// demo show a compromised sensor: its signature is valid, but its values do not
// match the revenue meter.
package iot

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"hash/fnv"
	"math"
	mrand "math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const dateLayout = "2006-01-02"

// Scenarios supported by the simulator.
const (
	ScenarioHonest      = "honest"      // real value, valid signature
	ScenarioCompromised = "compromised" // sensor inflated +40%, valid signature (needs reconciliation to catch)
	ScenarioGross       = "gross"       // implausible yield, valid signature (quarantined)
	ScenarioImpossible  = "impossible"  // above the physical limit (rejected)
	ScenarioTamper      = "tamper"      // value changed after signing (rejected)
	ScenarioForged      = "forged"      // cloned device with an uncertified key (rejected)
	ScenarioReplay      = "replay"      // last accepted batch sent again (rejected)
)

// SignedReading is the telemetry payload produced by a meter.
type SignedReading struct {
	MeterID     string `json:"meterID"`
	ReadingID   string `json:"readingID"`
	KWh         string `json:"kwh"` // value inside the signed message
	PeriodStart string `json:"periodStart"`
	PeriodEnd   string `json:"periodEnd"`
	Message     string `json:"message"`
	Signature   string `json:"signature"`
}

// Batch is what the simulator produced for one call.
type Batch struct {
	Reading      SignedReading `json:"signed"`
	SubmittedKWh string        `json:"submittedKWh"` // value sent to the chaincode
	RealKWh      float64       `json:"realKWh"`      // physical truth for the period
	Scenario     string        `json:"scenario"`
	Cycle        string        `json:"cycle"`
}

// profile is persisted per meter (data/meters/<id>.json).
type profile struct {
	CapacityKWp float64            `json:"capacityKWp"`
	Cursor      string             `json:"cursor"` // next period start
	Truth       map[string]float64 `json:"truth"`  // physical kWh injected per cycle
	Last        *SignedReading     `json:"last,omitempty"`
}

// Status is a public view of a simulated meter.
type Status struct {
	MeterID     string             `json:"meterID"`
	CapacityKWp float64            `json:"capacityKWp"`
	NextPeriod  string             `json:"nextPeriod"`
	Truth       map[string]float64 `json:"physicalTruthKWh"`
	HasKey      bool               `json:"hasKey"`
}

// MeterSimulator stores meter keys and profiles.
type MeterSimulator struct {
	dir string
	mu  sync.Mutex
	rng *mrand.Rand
}

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// NewMeterSimulator creates the simulator, storing files under dir.
func NewMeterSimulator(dir string) (*MeterSimulator, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &MeterSimulator{dir: dir, rng: mrand.New(mrand.NewSource(time.Now().UnixNano()))}, nil
}

// ReadingMessage must match the chaincode: meterID|readingID|kwh|periodStart|periodEnd
func ReadingMessage(meterID, readingID, kwh, start, end string) string {
	return strings.Join([]string{meterID, readingID, kwh, start, end}, "|")
}

// CurrentCycle returns the billing cycle of today (YYYY-MM).
func CurrentCycle() string { return time.Now().Format("2006-01") }

// Provision generates the key pair of a meter and returns its public key (PEM).
func (s *MeterSimulator) Provision(meterID string, capacityKWp float64) (string, error) {
	if !validID.MatchString(meterID) {
		return "", fmt.Errorf("meterID may only contain letters, numbers, '-' and '_'")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", err
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.WriteFile(s.path(meterID, ".key.pem"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		return "", err
	}
	first := time.Now().Format("2006-01") + "-01"
	if err := s.save(meterID, &profile{CapacityKWp: capacityKWp, Cursor: first, Truth: map[string]float64{}}); err != nil {
		return "", err
	}
	return publicPEM(&key.PublicKey)
}

// Discard removes a provisioned meter (used when on-chain registration fails).
func (s *MeterSimulator) Discard(meterID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	os.Remove(s.path(meterID, ".key.pem"))
	os.Remove(s.path(meterID, ".json"))
}

// Status returns the simulator view of a meter.
func (s *MeterSimulator) Status(meterID string) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.load(meterID)
	if err != nil {
		return Status{}, err
	}
	_, keyErr := os.Stat(s.path(meterID, ".key.pem"))
	return Status{MeterID: meterID, CapacityKWp: p.CapacityKWp, NextPeriod: p.Cursor, Truth: p.Truth, HasKey: keyErr == nil}, nil
}

// NextBatch produces the next weekly batch for a scenario.
// kwhOverride > 0 replaces the simulated physical generation.
func (s *MeterSimulator) NextBatch(meterID, scenario string, kwhOverride float64) (Batch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.load(meterID)
	if err != nil {
		return Batch{}, err
	}
	key, err := s.loadKey(meterID)
	if err != nil {
		return Batch{}, err
	}

	if scenario == ScenarioReplay {
		if p.Last == nil {
			return Batch{}, fmt.Errorf("no accepted batch to replay yet: send an honest reading first")
		}
		return Batch{Reading: *p.Last, SubmittedKWh: p.Last.KWh, Scenario: scenario, Cycle: p.Last.PeriodStart[:7]}, nil
	}

	start, err := time.Parse(dateLayout, p.Cursor)
	if err != nil {
		return Batch{}, err
	}
	monthEnd := time.Date(start.Year(), start.Month()+1, 0, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 6)
	if end.After(monthEnd) {
		end = monthEnd
	}
	days := float64(int(end.Sub(start).Hours()/24) + 1)

	real := kwhOverride
	if real <= 0 {
		// Typical summer yield in Florianópolis: 3.8 to 5.0 kWh/kWp/day.
		real = p.CapacityKWp * days * (3.8 + s.rng.Float64()*1.2)
	}
	real = math.Round(real*10) / 10

	reported := real
	signer := key
	switch scenario {
	case ScenarioHonest, ScenarioTamper:
	case ScenarioCompromised:
		reported = math.Round(real*1.4*10) / 10
	case ScenarioGross:
		reported = math.Round(p.CapacityKWp*days*8.5*10) / 10
	case ScenarioImpossible:
		reported = math.Round(p.CapacityKWp*days*14*10) / 10
	case ScenarioForged:
		signer, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return Batch{}, err
		}
	default:
		return Batch{}, fmt.Errorf("unknown scenario %q", scenario)
	}

	r := SignedReading{
		MeterID:     meterID,
		ReadingID:   fmt.Sprintf("R-%d", time.Now().UnixNano()),
		KWh:         format(reported),
		PeriodStart: start.Format(dateLayout),
		PeriodEnd:   end.Format(dateLayout),
	}
	r.Message = ReadingMessage(r.MeterID, r.ReadingID, r.KWh, r.PeriodStart, r.PeriodEnd)
	if r.Signature, err = sign(signer, r.Message); err != nil {
		return Batch{}, err
	}
	submitted := r.KWh
	if scenario == ScenarioTamper {
		submitted = format(math.Round(reported*100) / 10) // changed after signing (x10)
	}
	return Batch{Reading: r, SubmittedKWh: submitted, RealKWh: real, Scenario: scenario, Cycle: start.Format("2006-01")}, nil
}

// Commit records that a batch was accepted by the ledger: the period is
// consumed and its physical energy is added to the cycle truth.
func (s *MeterSimulator) Commit(b Batch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.load(b.Reading.MeterID)
	if err != nil {
		return err
	}
	end, err := time.Parse(dateLayout, b.Reading.PeriodEnd)
	if err != nil {
		return err
	}
	p.Cursor = end.AddDate(0, 0, 1).Format(dateLayout)
	p.Truth[b.Cycle] = math.Round((p.Truth[b.Cycle]+b.RealKWh)*10) / 10
	last := b.Reading
	p.Last = &last
	return s.save(b.Reading.MeterID, p)
}

// RevenueMeterReading simulates the utility revenue meter for a cycle:
// the physical truth with a measurement error of up to ±0.8%.
func (s *MeterSimulator) RevenueMeterReading(meterID, cycle string) (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.load(meterID)
	if err != nil {
		return 0, err
	}
	noise := 1 + (s.rng.Float64()*1.6-0.8)/100
	return math.Round(p.Truth[cycle]*noise*10) / 10, nil
}

// SimulatedConsumption returns a stable monthly grid consumption (250 to 450 kWh) per holder.
func SimulatedConsumption(owner, cycle string) float64 {
	h := fnv.New32a()
	h.Write([]byte(owner + cycle))
	return float64(250 + h.Sum32()%201)
}

func (s *MeterSimulator) path(meterID, ext string) string {
	return filepath.Join(s.dir, meterID+ext)
}

func (s *MeterSimulator) load(meterID string) (*profile, error) {
	if !validID.MatchString(meterID) {
		return nil, fmt.Errorf("invalid meterID")
	}
	data, err := os.ReadFile(s.path(meterID, ".json"))
	if err != nil {
		return nil, fmt.Errorf("meter %s is not provisioned in this simulator", meterID)
	}
	var p profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	if p.Truth == nil {
		p.Truth = map[string]float64{}
	}
	return &p, nil
}

func (s *MeterSimulator) save(meterID string, p *profile) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(meterID, ".json"), data, 0o600)
}

func (s *MeterSimulator) loadKey(meterID string) (*ecdsa.PrivateKey, error) {
	data, err := os.ReadFile(s.path(meterID, ".key.pem"))
	if err != nil {
		return nil, fmt.Errorf("no key for meter %s in this simulator", meterID)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("corrupted key for meter %s", meterID)
	}
	return x509.ParseECPrivateKey(block.Bytes)
}

func sign(key *ecdsa.PrivateKey, message string) (string, error) {
	digest := sha256.Sum256([]byte(message))
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

func publicPEM(pub *ecdsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), nil
}

func format(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
