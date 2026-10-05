package recurring

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kgazineu/finApp-back/internal/api"
)

const monthLayout = "2006-01"

type Controller struct {
	service *Service
}

func NewController(s *Service) *Controller {
	return &Controller{service: s}
}

type CreateTransactionRequest struct {
	Description    string `json:"description" binding:"required"`
	Kind           Kind   `json:"kind" binding:"required,oneof=income expense"`
	IsFixed        *bool  `json:"isFixed" binding:"required"`
	Amount         int64  `json:"amount" binding:"required,gt=0"`
	StartMonth     string `json:"startMonth" binding:"required"`
	IntervalMonths int    `json:"intervalMonths" binding:"gte=0,lte=120"`
	Installments   int    `json:"installments" binding:"gte=0,lte=600"`
	DayOfMonth     *int   `json:"dayOfMonth" binding:"omitempty,gte=1,lte=31"`
}

type UpdateTransactionRequest struct {
	Description string  `json:"description" binding:"required"`
	Amount      int64   `json:"amount" binding:"required,gt=0"`
	DayOfMonth  *int    `json:"dayOfMonth" binding:"omitempty,gte=1,lte=31"`
	EndMonth    *string `json:"endMonth"`
}

type SetPaidRequest struct {
	Paid *bool `json:"paid" binding:"required"`
}

type InstallmentResponse struct {
	ID            int64      `json:"id"`
	TransactionID int64      `json:"transactionId"`
	Number        int        `json:"number"`
	Amount        int64      `json:"amount"`
	DueDate       string     `json:"dueDate"`
	PaidAt        *time.Time `json:"paidAt"`
	Overdue       bool       `json:"overdue"`
}

type PendingInstallmentResponse struct {
	InstallmentResponse
	Description string `json:"description"`
	Kind        Kind   `json:"kind"`
	IsFixed     bool   `json:"isFixed"`
}

func toInstallmentResponse(i *Installment, now time.Time) InstallmentResponse {
	return InstallmentResponse{
		ID:            i.ID,
		TransactionID: i.TransactionID,
		Number:        i.Number,
		Amount:        i.Amount,
		DueDate:       i.DueDate.Format(time.DateOnly),
		PaidAt:        i.PaidAt,
		Overdue:       i.Overdue(now),
	}
}

func ToPendingInstallmentResponse(p *PendingInstallment, now time.Time) PendingInstallmentResponse {
	return PendingInstallmentResponse{
		InstallmentResponse: toInstallmentResponse(&p.Installment, now),
		Description:         p.Description,
		Kind:                p.Kind,
		IsFixed:             p.IsFixed,
	}
}

type TransactionResponse struct {
	ID             int64     `json:"id"`
	Description    string    `json:"description"`
	Kind           Kind      `json:"kind"`
	IsFixed        bool      `json:"isFixed"`
	Amount         int64     `json:"amount"`
	StartMonth     string    `json:"startMonth"`
	IntervalMonths int       `json:"intervalMonths"`
	EndMonth       *string   `json:"endMonth"`
	DayOfMonth     *int      `json:"dayOfMonth"`
	CreatedAt      time.Time `json:"createdAt"`
}

type OccurrenceResponse struct {
	TransactionID int64   `json:"transactionId"`
	Description   string  `json:"description"`
	Kind          Kind    `json:"kind"`
	IsFixed       bool    `json:"isFixed"`
	Amount        int64   `json:"amount"`
	Date          *string `json:"date"`
	Installment   int     `json:"installment,omitempty"`
	Installments  int     `json:"installments,omitempty"`
}

func toTransactionResponse(t *Transaction) TransactionResponse {
	var endMonth *string
	if t.EndMonth != nil {
		s := t.EndMonth.Format(monthLayout)
		endMonth = &s
	}

	return TransactionResponse{
		ID:             t.ID,
		Description:    t.Description,
		Kind:           t.Kind,
		IsFixed:        t.IsFixed,
		Amount:         t.Amount,
		StartMonth:     t.StartMonth.Format(monthLayout),
		IntervalMonths: t.IntervalMonths,
		EndMonth:       endMonth,
		DayOfMonth:     t.DayOfMonth,
		CreatedAt:      t.CreatedAt,
	}
}

func ToOccurrenceResponse(o Occurrence) OccurrenceResponse {
	var date *string
	if o.Date != nil {
		s := o.Date.Format(time.DateOnly)
		date = &s
	}

	return OccurrenceResponse{
		TransactionID: o.ID,
		Description:   o.Description,
		Kind:          o.Kind,
		IsFixed:       o.IsFixed,
		Amount:        o.Amount,
		Date:          date,
		Installment:   o.Installment,
		Installments:  o.Installments,
	}
}

func (ctrl *Controller) Create(c *gin.Context) {
	var req CreateTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	startMonth, err := time.Parse(monthLayout, req.StartMonth)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "startMonth must be YYYY-MM"})
		return
	}

	t, err := ctrl.service.Create(c.Request.Context(), api.UserID(c), &Transaction{
		Description:    req.Description,
		Kind:           req.Kind,
		IsFixed:        *req.IsFixed,
		Amount:         req.Amount,
		StartMonth:     startMonth,
		IntervalMonths: max(req.IntervalMonths, 1),
		DayOfMonth:     req.DayOfMonth,
	}, req.Installments)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, toTransactionResponse(t))
}

func (ctrl *Controller) FindByMonth(c *gin.Context) {
	month := time.Now()
	if m := c.Query("month"); m != "" {
		parsed, err := time.Parse(monthLayout, m)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "month must be YYYY-MM"})
			return
		}
		month = parsed
	}

	occurrences, err := ctrl.service.FindByMonth(c.Request.Context(), api.UserID(c), month)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	res := make([]OccurrenceResponse, len(occurrences))
	for i, o := range occurrences {
		res[i] = ToOccurrenceResponse(o)
	}

	c.JSON(http.StatusOK, res)
}

func (ctrl *Controller) FindAll(c *gin.Context) {
	transactions, err := ctrl.service.FindAll(c.Request.Context(), api.UserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	res := make([]TransactionResponse, len(transactions))
	for i, t := range transactions {
		res[i] = toTransactionResponse(t)
	}

	c.JSON(http.StatusOK, res)
}

func (ctrl *Controller) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	var req UpdateTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	t := &Transaction{ID: id, Description: req.Description, Amount: req.Amount, DayOfMonth: req.DayOfMonth}
	if req.EndMonth != nil {
		endMonth, err := time.Parse(monthLayout, *req.EndMonth)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "endMonth must be YYYY-MM"})
			return
		}
		t.EndMonth = &endMonth
	}

	updated, err := ctrl.service.Update(c.Request.Context(), api.UserID(c), t)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		return
	}
	if errors.Is(err, ErrInvalidTransaction) {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, toTransactionResponse(updated))
}

func (ctrl *Controller) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	archived, err := ctrl.service.Delete(c.Request.Context(), api.UserID(c), id)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"archived": archived})
}

func (ctrl *Controller) FindPendingInstallments(c *gin.Context) {
	now := time.Now()
	y, m, _ := now.Date()
	endOfMonth := time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC)

	pending, err := ctrl.service.FindPendingInstallments(c.Request.Context(), api.UserID(c), endOfMonth)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

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
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	res := make([]PendingInstallmentResponse, len(paid))
	for i, p := range paid {
		res[i] = ToPendingInstallmentResponse(p, now)
	}

	c.JSON(http.StatusOK, res)
}

func (ctrl *Controller) SetInstallmentPaid(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	var req SetPaidRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	inst, err := ctrl.service.SetInstallmentPaid(c.Request.Context(), api.UserID(c), id, *req.Paid)
	if errors.Is(err, ErrInstallmentNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"message": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, toInstallmentResponse(inst, time.Now()))
}
