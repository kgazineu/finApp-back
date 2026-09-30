package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kgazineu/finApp-back/internal/transaction"
)

func (s *Server) ListTransactions(c *gin.Context, params ListTransactionsParams) {
	if s.sessions == nil || s.transactions == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível listar as transações"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	userID, ok := s.sessionUser(c, ctx)
	if !ok {
		return
	}
	query := c.Request.URL.Query()
	for _, key := range []string{"limit", "offset"} {
		if values, present := query[key]; present && (len(values) != 1 || values[0] == "") {
			c.JSON(http.StatusBadRequest, ErrorResponse{Message: transaction.ErrInvalidPagination.Error()})
			return
		}
	}
	input := transaction.ListInput{UserID: userID, Limit: transaction.DefaultListLimit}
	if params.Limit != nil {
		input.Limit = *params.Limit
	}
	if params.Offset != nil {
		input.Offset = *params.Offset
	}
	if err := input.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}
	items, err := s.transactions.List(ctx, input)
	if err != nil {
		if errors.Is(err, transaction.ErrInvalidPagination) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Message: transaction.ErrInvalidPagination.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível listar as transações"})
		return
	}
	data := make([]TransactionResponse, 0, len(items))
	for _, item := range items {
		data = append(data, transactionResponse(item))
	}
	c.JSON(http.StatusOK, ListTransactionsResponse{Data: data, Limit: input.Limit, Offset: input.Offset})
}
