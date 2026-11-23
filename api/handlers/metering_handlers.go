package handlers

import (
	"net/http"

	"api/iot"
	"api/services"

	"github.com/gin-gonic/gin"
)

// MeteringHandler exposes the demonstrated MVP scope: readings, reconciliation,
// audit, accounts and the simulated compensation bill.
type MeteringHandler struct {
	ledger    *services.LedgerService
	simulator *iot.MeterSimulator
}

func NewMeteringHandler(ledger *services.LedgerService, simulator *iot.MeterSimulator) *MeteringHandler {
	return &MeteringHandler{ledger: ledger, simulator: simulator}
}

type ReconcileRequest struct {
	MeterID string `json:"meterID" binding:"required"`
	Cycle   string `json:"cycle"`
	// Optional: if omitted, the simulated revenue meter reads the physical truth.
	UtilityInjectedKWh *float64 `json:"utilityInjectedKWh"`
	UtilityConsumedKWh *float64 `json:"utilityConsumedKWh"`
}

type BillRequest struct {
	Owner string `json:"owner" binding:"required"`
	Cycle string `json:"cycle"`
}

// GET /api/readings?meterID=
func (h *MeteringHandler) HandleListReadings(c *gin.Context) {
	respondQuery(c, "readings", "[]", func() ([]byte, error) { return h.ledger.ListReadings(orgFrom(c), c.Query("meterID")) })
}

// GET /api/meter-states
func (h *MeteringHandler) HandleListMeterStates(c *gin.Context) {
	respondQuery(c, "states", "[]", func() ([]byte, error) { return h.ledger.ListMeterStates(orgFrom(c)) })
}

// POST /api/reconciliations
// Reads the (simulated) utility revenue meter and reconciles the cycle on-chain.
func (h *MeteringHandler) HandleReconcile(c *gin.Context) {
	var req ReconcileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Cycle == "" {
		req.Cycle = iot.CurrentCycle()
	}
	injected := 0.0
	if req.UtilityInjectedKWh != nil {
		injected = *req.UtilityInjectedKWh
	} else {
		v, err := h.simulator.RevenueMeterReading(req.MeterID, req.Cycle)
		if err != nil {
			respondWithError(c, http.StatusBadRequest, err.Error())
			return
		}
		injected = v
	}
	consumed := 0.0
	if req.UtilityConsumedKWh != nil {
		consumed = *req.UtilityConsumedKWh
	} else {
		meterJSON, err := h.ledger.GetMeter(orgFrom(c), req.MeterID)
		if err != nil {
			respondWithFabricError(c, err)
			return
		}
		consumed = iot.SimulatedConsumption(ownerOf(meterJSON), req.Cycle)
	}
	result, err := h.ledger.ReconcileCycle(orgFrom(c), req.MeterID, req.Cycle, formatFloat(injected), formatFloat(consumed))
	if err != nil {
		respondWithFabricError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"reconciliation": rawJSON(result, "{}")})
}

// GET /api/reconciliations
func (h *MeteringHandler) HandleListReconciliations(c *gin.Context) {
	respondQuery(c, "reconciliations", "[]", func() ([]byte, error) { return h.ledger.ListReconciliations(orgFrom(c)) })
}

// GET /api/audit?cycle=2026-11&substation=SUB-01
func (h *MeteringHandler) HandleAudit(c *gin.Context) {
	cycle := c.Query("cycle")
	if cycle == "" {
		cycle = iot.CurrentCycle()
	}
	respondQuery(c, "rows", "[]", func() ([]byte, error) { return h.ledger.AuditReport(orgFrom(c), cycle, c.Query("substation")) })
}

// POST /api/billing/simulate
func (h *MeteringHandler) HandleSimulateBill(c *gin.Context) {
	var req BillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Cycle == "" {
		req.Cycle = iot.CurrentCycle()
	}
	result, err := h.ledger.SimulateBill(orgFrom(c), req.Owner, req.Cycle)
	if err != nil {
		respondWithFabricError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"statement": rawJSON(result, "{}")})
}

// GET /api/billing/statements
func (h *MeteringHandler) HandleListStatements(c *gin.Context) {
	respondQuery(c, "statements", "[]", func() ([]byte, error) { return h.ledger.ListStatements(orgFrom(c)) })
}

// GET /api/accounts
func (h *MeteringHandler) HandleListAccounts(c *gin.Context) {
	respondQuery(c, "accounts", "[]", func() ([]byte, error) { return h.ledger.ListAccounts(orgFrom(c)) })
}

// GET /api/accounts/:owner
func (h *MeteringHandler) HandleGetAccount(c *gin.Context) {
	respondQuery(c, "account", "{}", func() ([]byte, error) { return h.ledger.GetAccount(orgFrom(c), c.Param("owner")) })
}

// GET /api/accounts/:owner/history
func (h *MeteringHandler) HandleGetAccountHistory(c *gin.Context) {
	respondQuery(c, "history", "[]", func() ([]byte, error) { return h.ledger.GetAccountHistory(orgFrom(c), c.Param("owner")) })
}

// GET /api/simulator/:meterID
func (h *MeteringHandler) HandleSimulatorStatus(c *gin.Context) {
	status, err := h.simulator.Status(c.Param("meterID"))
	if err != nil {
		respondWithError(c, http.StatusNotFound, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"simulator": status, "cycle": iot.CurrentCycle()})
}
