package passwordreset

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/kgazineu/finApp-back/internal/user"
)

type Controller struct {
	service *Service
}

func NewController(s *Service) *Controller {
	return &Controller{service: s}
}

type RequestRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type VerifyRequest struct {
	Email string `json:"email" binding:"required,email"`
	Code  string `json:"code" binding:"required,len=6,numeric"`
}

type ConfirmRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Code     string `json:"code" binding:"required,len=6,numeric"`
	Password string `json:"password" binding:"required"`
}

func (ctrl *Controller) Request(c *gin.Context) {
	var req RequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": api.BindError(err)})
		return
	}

	if err := ctrl.service.Request(c.Request.Context(), req.Email); err != nil {
		api.InternalError(c, err)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "Se o e-mail estiver cadastrado, você vai receber um código de 6 dígitos"})
}

func (ctrl *Controller) Verify(c *gin.Context) {
	var req VerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": api.BindError(err)})
		return
	}

	err := ctrl.service.Verify(c.Request.Context(), req.Email, req.Code)
	if errors.Is(err, ErrInvalidCode) {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	if err != nil {
		api.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Código válido"})
}

func (ctrl *Controller) Confirm(c *gin.Context) {
	var req ConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": api.BindError(err)})
		return
	}

	err := ctrl.service.Confirm(c.Request.Context(), req.Email, req.Code, req.Password)
	if errors.Is(err, ErrInvalidCode) || errors.Is(err, user.ErrInvalidPassword) {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	if err != nil {
		api.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Senha alterada, entre com a nova senha"})
}
