package handlers

import (
	"net/http"

	"api/gateway"

	"github.com/gin-gonic/gin"
)

// NetworkHandler describes the consortium organizations the API is connected as.
type NetworkHandler struct {
	gw      *gateway.Gateway
	channel string
}

func NewNetworkHandler(gw *gateway.Gateway, channel string) *NetworkHandler {
	return &NetworkHandler{gw: gw, channel: channel}
}

// GET /api/network
func (h *NetworkHandler) HandleNetworkInfo(c *gin.Context) {
	org, _ := h.gw.Org("")
	c.JSON(http.StatusOK, gin.H{
		"channel":    h.channel,
		"orgs":       h.gw.Orgs(),
		"defaultOrg": org.Config.Name,
	})
}
