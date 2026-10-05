package account

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kgazineu/finApp-back/internal/api"
)

type Controller struct {
	service *Service
}

func NewController(s *Service) *Controller {
	return &Controller{service: s}
}

type AccountResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Kind      Kind      `json:"kind"`
	HasYield  bool      `json:"hasYield"`
	CreatedAt time.Time `json:"createdAt"`
}

func toAccountResponse(a *Account) AccountResponse {
	return AccountResponse{
		ID:        a.ID,
		Name:      a.Name,
		Kind:      a.Kind,
		HasYield:  a.HasYield,
		CreatedAt: a.CreatedAt,
	}
}

type CreateAccountRequest struct {
	Name     string `json:"name" binding:"required"`
	Kind     Kind   `json:"kind" binding:"required,oneof=asset liability"`
	HasYield bool   `json:"hasYield"`
}

type UpdateAccountRequest struct {
	Name     string `json:"name" binding:"required"`
	Kind     Kind   `json:"kind" binding:"required,oneof=asset liability"`
	HasYield bool   `json:"hasYield"`
}

func (ctrl *Controller) Create(c *gin.Context) {
	var req CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	if req.Kind == KindLiability && req.HasYield == true {
		c.JSON(http.StatusBadRequest, gin.H{"message": "liability accounts cannot have yield"})
		return
	}

	a, err := ctrl.service.Create(c.Request.Context(), api.UserID(c), &Account{
		Name:     req.Name,
		Kind:     req.Kind,
		HasYield: req.HasYield,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, toAccountResponse(a))
}

func (ctrl *Controller) FindActive(c *gin.Context) {
	accounts, err := ctrl.service.FindActive(c.Request.Context(), api.UserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	res := make([]AccountResponse, len(accounts))
	for i, a := range accounts {
		res[i] = toAccountResponse(a)
	}

	c.JSON(http.StatusOK, res)
}

func (ctrl *Controller) FindByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	account, err := ctrl.service.FindByID(c.Request.Context(), api.UserID(c), id)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, toAccountResponse(account))
}

func (ctrl *Controller) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	account, err := ctrl.service.Update(c.Request.Context(), api.UserID(c), &Account{
		ID:       id,
		Name:     req.Name,
		Kind:     req.Kind,
		HasYield: req.HasYield,
	})
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		return
	}
	if errors.Is(err, ErrLiabilityWithYield) {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, toAccountResponse(account))
}

func (ctrl *Controller) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	deleteResult, err := ctrl.service.Delete(c.Request.Context(), api.UserID(c), id)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	var responseMessage string
	if deleteResult.Archived == true {
		responseMessage = "account archived"
	} else {
		responseMessage = "account deleted"
	}

	c.JSON(http.StatusOK, gin.H{"message": responseMessage})
}
