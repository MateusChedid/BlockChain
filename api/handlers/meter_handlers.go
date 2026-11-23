package handlers

import (
	"net/http"

	"api/iot"
	"api/services"

	"github.com/gin-gonic/gin"
)

// MeterHandler registers IoT meters and submits their signed readings.
type MeterHandler struct {
	ledger    *services.LedgerService
	simulator *iot.MeterSimulator
}

func NewMeterHandler(ledger *services.LedgerService, simulator *iot.MeterSimulator) *MeterHandler {
	return &MeterHandler{ledger: ledger, simulator: simulator}
}

type RegisterMeterRequest struct {
	MeterID      string  `json:"meterID" binding:"required"`
	Owner        string  `json:"owner" binding:"required"`
	ConsumerUnit string  `json:"consumerUnit" binding:"required"`
	Substation   string  `json:"substation" binding:"required"`
	CapacityKWp  float64 `json:"capacityKWp" binding:"required"`
}

type ReadingRequest struct {
	// Scenario: honest | compromised | gross | impossible | tamper | forged | replay
	Scenario string `json:"scenario"`
	// KWh optionally replaces the simulated physical generation of the week.
	KWh float64 `json:"kwh"`
}

// POST /api/meters
// Provisions a key pair in the (simulated) meter HSM and registers the public key on-chain.
func (h *MeterHandler) HandleRegisterMeter(c *gin.Context) {
	var req RegisterMeterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, err.Error())
		return
	}
	publicKey, err := h.simulator.Provision(req.MeterID, req.CapacityKWp)
	if err != nil {
		respondWithError(c, http.StatusBadRequest, err.Error())
		return
	}
	err = h.ledger.RegisterMeter(orgFrom(c), req.MeterID, req.Owner, req.ConsumerUnit, req.Substation, formatFloat(req.CapacityKWp), publicKey)
	if err != nil {
		h.simulator.Discard(req.MeterID)
		respondWithFabricError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "meterID": req.MeterID, "publicKeyPEM": publicKey})
}

// GET /api/meters
func (h *MeterHandler) HandleListMeters(c *gin.Context) {
	respondQuery(c, "meters", "[]", func() ([]byte, error) { return h.ledger.ListMeters(orgFrom(c)) })
}

// GET /api/meters/:meterID
func (h *MeterHandler) HandleGetMeter(c *gin.Context) {
	result, err := h.ledger.GetMeter(orgFrom(c), c.Param("meterID"))
	if err != nil {
		respondWithFabricError(c, err)
		return
	}
	status, simErr := h.simulator.Status(c.Param("meterID"))
	resp := gin.H{"meter": rawJSON(result, "{}")}
	if simErr == nil {
		resp["simulator"] = status
	}
	c.JSON(http.StatusOK, resp)
}

// POST /api/meters/:meterID/deactivate
func (h *MeterHandler) HandleDeactivateMeter(c *gin.Context) {
	if err := h.ledger.DeactivateMeter(orgFrom(c), c.Param("meterID")); err != nil {
		respondWithFabricError(c, err)
		return
	}
	respondWithSuccess(c, http.StatusOK, "meter deactivated")
}

// POST /api/meters/:meterID/readings
// The meter signs the next weekly batch and the oracle gateway submits it.
// Attack scenarios show which checks catch each kind of bad data.
func (h *MeterHandler) HandleSubmitReading(c *gin.Context) {
	meterID := c.Param("meterID")
	var req ReadingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Scenario == "" {
		req.Scenario = iot.ScenarioHonest
	}
	batch, err := h.simulator.NextBatch(meterID, req.Scenario, req.KWh)
	if err != nil {
		respondWithError(c, http.StatusBadRequest, err.Error())
		return
	}
	r := batch.Reading
	result, err := h.ledger.SubmitReading(orgFrom(c), r.MeterID, r.ReadingID, batch.SubmittedKWh, r.PeriodStart, r.PeriodEnd, r.Signature)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"accepted": false, "error": FabricMessage(err), "batch": batch})
		return
	}
	if req.Scenario != iot.ScenarioReplay {
		if err := h.simulator.Commit(batch); err != nil {
			respondWithError(c, http.StatusInternalServerError, err.Error())
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"accepted": true, "reading": rawJSON(result, "{}"), "batch": batch})
}
