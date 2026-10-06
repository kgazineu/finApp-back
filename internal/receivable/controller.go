package receivable

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

type CreateReceivableRequest struct {
	Kind         Kind       `json:"kind" binding:"required,oneof=split loan"`
	Debtor       string     `json:"debtor" binding:"required"`
	Description  string     `json:"description" binding:"required"`
	Amount       int64      `json:"amount" binding:"required,gt=0"`
	AmountMode   AmountMode `json:"amountMode" binding:"omitempty,oneof=total installment"` // vazio = total
	InterestRate int        `json:"interestRate" binding:"gte=0"`
	Installments int        `json:"installments" binding:"gte=0,lte=120"`
	FirstDueDate string     `json:"firstDueDate" binding:"required"` // pode ser no passado
}

// UpdateInstallmentRequest: qualquer combinação dos campos. Ponteiros distinguem "não enviado" de false/0.
type UpdateInstallmentRequest struct {
	Paid             *bool   `json:"paid"`
	Amount           *int64  `json:"amount" binding:"omitempty,gt=0"`
	DueDate          *string `json:"dueDate"`
	ApplyToFollowing bool    `json:"applyToFollowing"`
}

type UpdateReceivableRequest struct {
	Debtor      string `json:"debtor" binding:"required"`
	Description string `json:"description" binding:"required"`
}

type InstallmentResponse struct {
	ID      int64      `json:"id"`
	Number  int        `json:"number"`
	Amount  int64      `json:"amount"`
	DueDate string     `json:"dueDate"`
	PaidAt  *time.Time `json:"paidAt"`
	Overdue bool       `json:"overdue"`
}

type ReceivableResponse struct {
	ID           int64                 `json:"id"`
	Kind         Kind                  `json:"kind"`
	Debtor       string                `json:"debtor"`
	Description  string                `json:"description"`
	Amount       int64                 `json:"amount"`
	AmountMode   AmountMode            `json:"amountMode"`
	InterestRate int                   `json:"interestRate"`
	Installments []InstallmentResponse `json:"installments"`
	CreatedAt    time.Time             `json:"createdAt"`
}

type PendingInstallmentResponse struct {
	InstallmentResponse
	ReceivableID int64  `json:"receivableId"`
	Debtor       string `json:"debtor"`
	Description  string `json:"description"`
}

func toInstallmentResponse(i *Installment, now time.Time) InstallmentResponse {
	return InstallmentResponse{
		ID:      i.ID,
		Number:  i.Number,
		Amount:  i.Amount,
		DueDate: i.DueDate.Format(time.DateOnly),
		PaidAt:  i.PaidAt,
		Overdue: i.Overdue(now),
	}
}

func ToPendingInstallmentResponse(p *PendingInstallment, now time.Time) PendingInstallmentResponse {
	return PendingInstallmentResponse{
		InstallmentResponse: toInstallmentResponse(&p.Installment, now),
		ReceivableID:        p.ReceivableID,
		Debtor:              p.Debtor,
		Description:         p.Description,
	}
}

func ToReceivableResponse(r *Receivable, now time.Time) ReceivableResponse {
	installments := make([]InstallmentResponse, len(r.Installments))
	for i, inst := range r.Installments {
		installments[i] = toInstallmentResponse(inst, now)
	}

	return ReceivableResponse{
		ID:           r.ID,
		Kind:         r.Kind,
		Debtor:       r.Debtor,
		Description:  r.Description,
		Amount:       r.Amount,
		AmountMode:   r.AmountMode,
		InterestRate: r.InterestRate,
		Installments: installments,
		CreatedAt:    r.CreatedAt,
	}
}

func (ctrl *Controller) Create(c *gin.Context) {
	var req CreateReceivableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": api.BindError(err)})
		return
	}

	firstDueDate, err := time.Parse(time.DateOnly, req.FirstDueDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "O primeiro vencimento deve estar no formato AAAA-MM-DD"})
		return
	}

	rec, err := ctrl.service.Create(c.Request.Context(), api.UserID(c), &Receivable{
		Kind:         req.Kind,
		Debtor:       req.Debtor,
		Description:  req.Description,
		Amount:       req.Amount,
		AmountMode:   req.AmountMode,
		InterestRate: req.InterestRate,
	}, max(req.Installments, 1), firstDueDate)
	if errors.Is(err, ErrInvalidReceivable) {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	if err != nil {
		api.InternalError(c, err)
		return
	}

	c.JSON(http.StatusCreated, ToReceivableResponse(rec, time.Now()))
}

func (ctrl *Controller) FindPendingInstallments(c *gin.Context) {
	pending, err := ctrl.service.FindPendingInstallments(c.Request.Context(), api.UserID(c))
	if err != nil {
		api.InternalError(c, err)
		return
	}

	now := time.Now()
	res := make([]PendingInstallmentResponse, len(pending))
	for i, p := range pending {
		res[i] = ToPendingInstallmentResponse(p, now)
	}

	c.JSON(http.StatusOK, res)
}

func (ctrl *Controller) FindPaidInstallments(c *gin.Context) {
	now := time.Now()

	paid, err := ctrl.service.FindPaidInstallments(c.Request.Context(), api.UserID(c), now.AddDate(0, 0, -30))
	if err != nil {
		api.InternalError(c, err)
		return
	}

	res := make([]PendingInstallmentResponse, len(paid))
	for i, p := range paid {
		res[i] = ToPendingInstallmentResponse(p, now)
	}

	c.JSON(http.StatusOK, res)
}

func (ctrl *Controller) UpdateInstallment(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id inválido"})
		return
	}

	var req UpdateInstallmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": api.BindError(err)})
		return
	}

	changes := InstallmentChanges{Paid: req.Paid, Amount: req.Amount, ApplyToFollowing: req.ApplyToFollowing}
	if req.DueDate != nil {
		due, err := time.Parse(time.DateOnly, *req.DueDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "O vencimento deve estar no formato AAAA-MM-DD"})
			return
		}
		changes.DueDate = &due
	}

	inst, err := ctrl.service.UpdateInstallment(c.Request.Context(), api.UserID(c), id, changes)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		return
	}
	if errors.Is(err, ErrInvalidReceivable) {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	if err != nil {
		api.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, toInstallmentResponse(inst, time.Now()))
}

func (ctrl *Controller) FindAll(c *gin.Context) {
	receivables, err := ctrl.service.FindAll(c.Request.Context(), api.UserID(c))
	if err != nil {
		api.InternalError(c, err)
		return
	}

	now := time.Now()
	res := make([]ReceivableResponse, len(receivables))
	for i, r := range receivables {
		res[i] = ToReceivableResponse(r, now)
	}

	c.JSON(http.StatusOK, res)
}

func (ctrl *Controller) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id inválido"})
		return
	}

	var req UpdateReceivableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": api.BindError(err)})
		return
	}

	rec, err := ctrl.service.Update(c.Request.Context(), api.UserID(c), id, req.Debtor, req.Description)
	if errors.Is(err, ErrReceivableNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		return
	}
	if err != nil {
		api.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, ToReceivableResponse(rec, time.Now()))
}

func (ctrl *Controller) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id inválido"})
		return
	}

	archived, err := ctrl.service.Delete(c.Request.Context(), api.UserID(c), id)
	if errors.Is(err, ErrReceivableNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		return
	}
	if err != nil {
		api.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"archived": archived})
}
