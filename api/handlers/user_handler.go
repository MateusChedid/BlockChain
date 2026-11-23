package handlers

import (
	"net/http"
	"strings"

	"api/store"

	"github.com/gin-gonic/gin"
)

// UserHandler manages off-chain household profiles.
type UserHandler struct {
	store *store.UserStore
}

func NewUserHandler(store *store.UserStore) *UserHandler {
	return &UserHandler{store: store}
}

type CreateUserRequest struct {
	ID           string `json:"id" binding:"required"`
	DisplayName  string `json:"displayName" binding:"required"`
	Type         string `json:"type" binding:"required"`
	Substation   string `json:"substation" binding:"required"`
	ConsumerUnit string `json:"consumerUnit"`
}

// GET /api/users
func (h *UserHandler) HandleListUsers(c *gin.Context) {
	users, err := h.store.List()
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users})
}

// POST /api/users
func (h *UserHandler) HandleCreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, err.Error())
		return
	}
	user := store.User{
		ID:           strings.TrimSpace(req.ID),
		DisplayName:  strings.TrimSpace(req.DisplayName),
		Type:         strings.ToLower(strings.TrimSpace(req.Type)),
		Substation:   strings.TrimSpace(req.Substation),
		ConsumerUnit: strings.TrimSpace(req.ConsumerUnit),
	}
	if err := h.store.Create(user); err != nil {
		respondWithError(c, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "user": user})
}
