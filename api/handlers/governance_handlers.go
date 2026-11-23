package handlers

import (
	"net/http"
	"strconv"

	"api/services"

	"github.com/gin-gonic/gin"
)

// GovernanceHandler exposes governance-cc (members, tariffs and the sandbox switch).
type GovernanceHandler struct {
	ledger *services.LedgerService
}

func NewGovernanceHandler(ledger *services.LedgerService) *GovernanceHandler {
	return &GovernanceHandler{ledger: ledger}
}

type OnboardMemberRequest struct {
	MemberID    string `json:"memberID" binding:"required"`
	MSPID       string `json:"mspID" binding:"required"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

type TariffRequest struct {
	Distributor     string  `json:"distributor"`
	TEBRL           float64 `json:"teBRL" binding:"required"`
	TUSDBRL         float64 `json:"tusdBRL" binding:"required"`
	FioBBRL         float64 `json:"fioBBRL"`
	FioBPct         int     `json:"fioBPct"`
	AvailabilityKWh float64 `json:"availabilityKWh"`
	TolerancePct    float64 `json:"tolerancePct" binding:"required"`
}

type SandboxRequest struct {
	Enabled   bool   `json:"enabled"`
	Reference string `json:"reference"`
}

// POST /api/governance/members
func (h *GovernanceHandler) HandleOnboardMember(c *gin.Context) {
	var req OnboardMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.ledger.OnboardMember(orgFrom(c), req.MemberID, req.MSPID, req.DisplayName, req.Role); err != nil {
		respondWithFabricError(c, err)
		return
	}
	respondWithSuccess(c, http.StatusCreated, "member onboarded")
}

// GET /api/governance/members
func (h *GovernanceHandler) HandleListMembers(c *gin.Context) {
	respondQuery(c, "members", "[]", func() ([]byte, error) { return h.ledger.ListMembers(orgFrom(c)) })
}

// GET /api/governance/tariffs
func (h *GovernanceHandler) HandleGetTariffs(c *gin.Context) {
	respondQuery(c, "tariffs", "{}", func() ([]byte, error) { return h.ledger.GetTariffs(orgFrom(c)) })
}

// PUT /api/governance/tariffs
func (h *GovernanceHandler) HandleUpdateTariffs(c *gin.Context) {
	var req TariffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, err.Error())
		return
	}
	err := h.ledger.UpdateTariffs(orgFrom(c), req.Distributor, formatFloat(req.TEBRL), formatFloat(req.TUSDBRL),
		formatFloat(req.FioBBRL), strconv.Itoa(req.FioBPct), formatFloat(req.AvailabilityKWh), formatFloat(req.TolerancePct))
	if err != nil {
		respondWithFabricError(c, err)
		return
	}
	respondWithSuccess(c, http.StatusOK, "tariffs updated")
}

// GET /api/governance/sandbox
func (h *GovernanceHandler) HandleGetSandbox(c *gin.Context) {
	respondQuery(c, "sandbox", "{}", func() ([]byte, error) { return h.ledger.GetSandboxConfig(orgFrom(c)) })
}

// PUT /api/governance/sandbox
func (h *GovernanceHandler) HandleSetSandbox(c *gin.Context) {
	var req SandboxRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.ledger.SetSandboxMode(orgFrom(c), strconv.FormatBool(req.Enabled), req.Reference); err != nil {
		respondWithFabricError(c, err)
		return
	}
	respondWithSuccess(c, http.StatusOK, "sandbox mode updated")
}

// GET /api/governance/price-band
func (h *GovernanceHandler) HandleGetPriceBand(c *gin.Context) {
	respondQuery(c, "priceBand", "{}", func() ([]byte, error) { return h.ledger.GetPriceBand(orgFrom(c)) })
}
