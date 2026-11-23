package handlers

import (
	"net/http"

	"api/services"

	"github.com/gin-gonic/gin"
)

// MarketHandler exposes the P2P order book (energy-cc "market" contract).
// FUTURE capability: every call is refused by the chaincode unless the
// regulatory sandbox is enabled in governance-cc.
type MarketHandler struct {
	ledger *services.LedgerService
}

func NewMarketHandler(ledger *services.LedgerService) *MarketHandler {
	return &MarketHandler{ledger: ledger}
}

type OrderRequest struct {
	Owner      string  `json:"owner" binding:"required"`
	Substation string  `json:"substation" binding:"required"`
	Quantity   float64 `json:"quantity" binding:"required"`
	PriceBRL   float64 `json:"priceBRL" binding:"required"`
}

type MatchRequest struct {
	Substation string `json:"substation" binding:"required"`
}

// POST /api/sandbox/asks  (prosumer sells credits)
func (h *MarketHandler) HandleSubmitAsk(c *gin.Context) {
	h.submit(c, h.ledger.SubmitAsk)
}

// POST /api/sandbox/bids  (consumer buys credits)
func (h *MarketHandler) HandleSubmitBid(c *gin.Context) {
	h.submit(c, h.ledger.SubmitBid)
}

func (h *MarketHandler) submit(c *gin.Context, fn func(org, owner, substation, qty, price string) ([]byte, error)) {
	var req OrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := fn(orgFrom(c), req.Owner, req.Substation, formatFloat(req.Quantity), formatFloat(req.PriceBRL))
	if err != nil {
		respondWithFabricError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"result": rawJSON(result, "{}")})
}

// POST /api/sandbox/orders/:orderID/cancel
func (h *MarketHandler) HandleCancelOrder(c *gin.Context) {
	if err := h.ledger.CancelOrder(orgFrom(c), c.Param("orderID")); err != nil {
		respondWithFabricError(c, err)
		return
	}
	respondWithSuccess(c, http.StatusOK, "order cancelled")
}

// POST /api/sandbox/match
func (h *MarketHandler) HandleMatch(c *gin.Context) {
	var req MatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.ledger.MatchOrders(orgFrom(c), req.Substation)
	if err != nil {
		respondWithFabricError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"trades": rawJSON(result, "[]")})
}

// GET /api/sandbox/orders?substation=SUB-01&status=OPEN
func (h *MarketHandler) HandleListOrders(c *gin.Context) {
	result, err := h.ledger.ListOrders(orgFrom(c), c.Query("substation"), c.Query("status"))
	if err != nil {
		respondWithFabricError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"orders": rawJSON(result, "[]")})
}

// GET /api/sandbox/trades
func (h *MarketHandler) HandleListTrades(c *gin.Context) {
	result, err := h.ledger.ListTrades(orgFrom(c))
	if err != nil {
		respondWithFabricError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"trades": rawJSON(result, "[]")})
}
