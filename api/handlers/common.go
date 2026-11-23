// Package handlers translates HTTP calls into Fabric transactions:
// HTTP -> handler -> LedgerService -> Fabric Gateway -> chaincode.
package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	gatewaypb "github.com/hyperledger/fabric-protos-go-apiv2/gateway"
	"google.golang.org/grpc/status"
)

// OrgHeader selects which organization signs the transaction.
const OrgHeader = "X-Org"

func orgFrom(c *gin.Context) string {
	return c.GetHeader(OrgHeader)
}

// rawJSON returns chaincode JSON as-is, using fallback for empty payloads.
func rawJSON(payload []byte, fallback string) json.RawMessage {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" || trimmed == "null" {
		return json.RawMessage(fallback)
	}
	return json.RawMessage(payload)
}

func respondWithError(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"error": msg})
}

func respondWithSuccess(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"success": true, "message": msg})
}

// respondWithFabricError extracts the chaincode error message from the gRPC
// status details so the UI can show messages such as "outside the governed corridor".
func respondWithFabricError(c *gin.Context, err error) {
	msg := FabricMessage(err)
	code := http.StatusInternalServerError
	if st, ok := status.FromError(err); ok && len(st.Details()) > 0 {
		code = http.StatusUnprocessableEntity // rejected by the chaincode logic
	}
	if strings.Contains(msg, "access denied") {
		code = http.StatusForbidden
	}
	c.JSON(code, gin.H{"error": msg, "details": err.Error()})
}

// FabricMessage returns the most useful error message of a Fabric error.
func FabricMessage(err error) string {
	if err == nil {
		return ""
	}
	st, ok := status.FromError(err)
	if !ok {
		return err.Error()
	}
	for _, d := range st.Details() {
		if detail, ok := d.(*gatewaypb.ErrorDetail); ok && detail.GetMessage() != "" {
			return cleanChaincodeMessage(detail.GetMessage())
		}
	}
	return st.Message()
}

func cleanChaincodeMessage(msg string) string {
	const prefix = "chaincode response 500, "
	if idx := strings.LastIndex(msg, prefix); idx >= 0 {
		msg = msg[idx+len(prefix):]
	}
	return strings.TrimSpace(msg)
}

// ownerOf extracts the "owner" field of a meter JSON document.
func ownerOf(meterJSON []byte) string {
	var m struct {
		Owner string `json:"owner"`
	}
	_ = json.Unmarshal(meterJSON, &m)
	return m.Owner
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// respondQuery runs a read-only query and wraps its JSON under key.
func respondQuery(c *gin.Context, key, fallback string, query func() ([]byte, error)) {
	result, err := query()
	if err != nil {
		respondWithFabricError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{key: rawJSON(result, fallback)})
}
