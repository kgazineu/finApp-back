package billing

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kgazineu/finApp-back/internal/account"
	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/kgazineu/finApp-back/internal/receivable"
	"github.com/kgazineu/finApp-back/internal/recurring"
)

type Controller struct {
	service *Service
}

func NewController(s *Service) *Controller {
	return &Controller{service: s}
}

type BillingEntryResponse struct {
	ID          int64        `json:"id"`
	AccountID   int64        `json:"accountId"`
	AccountName string       `json:"accountName"`
	AccountKind account.Kind `json:"accountKind"`
	Amount      int64        `json:"amount"`
}

type BillingResponse struct {
	ID        int64                  `json:"id"`
	Delta     *int64                 `json:"delta"`
	Total     int64                  `json:"total"`
	Entries   []BillingEntryResponse `json:"entries"`
	CreatedAt time.Time              `json:"createdAt"`
}

type ProjectionResponse struct {
	BillingRegistrations []BillingResponse                       `json:"billingRegistrations"`
	ProjectedFor         string                                  `json:"projectedFor"`
	ProjectedAmount      int64                                   `json:"projectedAmount"`
	MonthlyGrowth        int64                                   `json:"monthlyGrowth"`
	MonthlyReceivables   int64                                   `json:"monthlyReceivables"`
	MonthlyGrowthMonth   string                                  `json:"monthlyGrowthMonth"`
	PendingReceivables   []receivable.PendingInstallmentResponse `json:"pendingReceivables"`
	PendingTransactions  []recurring.PendingInstallmentResponse  `json:"pendingTransactions"`
}

func ToBillingRegistrationResponse(b *BillingRegistration) BillingResponse {
	entries := make([]BillingEntryResponse, len(b.entries))
	for i, e := range b.entries {
		entries[i] = BillingEntryResponse{
			ID:          e.ID,
			AccountID:   e.AccountID,
			AccountName: e.AccountName,
			AccountKind: e.AccountKind,
			Amount:      e.Amount,
		}
	}

	return BillingResponse{
		ID:        b.ID,
		Delta:     b.delta,
		Total:     b.total,
		Entries:   entries,
		CreatedAt: b.CreatedAt,
	}
}

type CreateBillingEntryRequest struct {
	AccountID int64  `json:"accountId" binding:"required"`
	Amount    *int64 `json:"amount" binding:"required,gte=0"`
}

type CreateBillingRequest struct {
	Entries []CreateBillingEntryRequest `json:"entries" binding:"required,min=1,unique=AccountID,dive"`
}

func (ctrl *Controller) NextMonthProjectionController(c *gin.Context) {
	months := 1
	if q := c.Query("months"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 1 || n > 120 {
			c.JSON(http.StatusBadRequest, gin.H{"message": "A quantidade de meses deve ser um número entre 1 e 120"})
			return
		}
		months = n
	}

	ctx := c.Request.Context()
	userID := api.UserID(c)
	now := time.Now()

	billings, err := ctrl.service.FindMany(ctx, userID)
	if err != nil {
		api.InternalError(c, err)
		return
	}

	projection, err := ctrl.service.MonthProjection(ctx, userID, now, months)
	if err != nil {
		api.InternalError(c, err)
		return
	}

	monthly, err := ctrl.service.MonthlyFigures(ctx, userID, now)
	if err != nil {
		api.InternalError(c, err)
		return
	}

	res := ProjectionResponse{
		BillingRegistrations: make([]BillingResponse, len(billings)),
		ProjectedFor:         projection.Date.Format(time.DateOnly),
		ProjectedAmount:      projection.Total,
		MonthlyGrowth:        monthly.Growth,
		MonthlyReceivables:   monthly.Receivables,
		MonthlyGrowthMonth:   monthly.Month.Format("2006-01"),
		PendingReceivables:   make([]receivable.PendingInstallmentResponse, len(projection.Receivables)),
		PendingTransactions:  make([]recurring.PendingInstallmentResponse, len(projection.Transactions)),
	}
	for i, b := range billings {
		res.BillingRegistrations[i] = ToBillingRegistrationResponse(b)
	}
	for i, p := range projection.Receivables {
		res.PendingReceivables[i] = receivable.ToPendingInstallmentResponse(p, now)
	}
	for i, p := range projection.Transactions {
		res.PendingTransactions[i] = recurring.ToPendingInstallmentResponse(p, now)
	}

	c.JSON(http.StatusOK, res)
}

func (ctrl *Controller) CreateController(c *gin.Context) {
	var createBillingRequest CreateBillingRequest

	if err := c.ShouldBindJSON(&createBillingRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": api.BindError(err)})
		return
	}

	br, err := ctrl.service.CreateBillingRegistration(c.Request.Context(), api.UserID(c), createBillingRequest.Entries)
	if errors.Is(err, ErrInvalidEntries) {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	if err != nil {
		api.InternalError(c, err)
		return
	}

	c.JSON(http.StatusCreated, ToBillingRegistrationResponse(br))
}
