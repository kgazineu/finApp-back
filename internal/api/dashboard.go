package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *Server) GetDashboard(c *gin.Context, params GetDashboardParams) {
	if s.sessions == nil || s.overview == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível consultar o resumo"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	userID, ok := s.sessionUser(c, ctx)
	if !ok {
		return
	}
	month := time.Now().UTC()
	if values, present := c.Request.URL.Query()["month"]; present {
		if len(values) != 1 || len(values[0]) != len("2006-01") {
			c.JSON(http.StatusBadRequest, ErrorResponse{Message: "Mês inválido: use YYYY-MM"})
			return
		}
		parsed, err := time.Parse("2006-01", values[0])
		if err != nil || parsed.IsZero() || parsed.Year() < 1 || parsed.Format("2006-01") != values[0] {
			c.JSON(http.StatusBadRequest, ErrorResponse{Message: "Mês inválido: use YYYY-MM"})
			return
		}
		month = parsed
	}
	snapshot, err := s.overview.Get(ctx, userID, month)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível consultar o resumo"})
		return
	}
	response := DashboardResponse{
		Month: snapshot.Month.UTC().Format("2006-01"), IncomeMinor: snapshot.IncomeMinor,
		ExpenseMinor: snapshot.ExpenseMinor, NetTrackedMinor: snapshot.NetTrackedMinor,
		GoalsSavedMinor: snapshot.GoalsSavedMinor, GoalsTargetMinor: snapshot.GoalsTargetMinor,
		RecentTransactions: make([]TransactionResponse, 0, len(snapshot.RecentTransactions)),
		ExpensesByCategory: make([]CategoryTotalResponse, 0, len(snapshot.ExpensesByCategory)),
	}
	for _, item := range snapshot.RecentTransactions {
		response.RecentTransactions = append(response.RecentTransactions, transactionResponse(item))
	}
	for _, item := range snapshot.ExpensesByCategory {
		response.ExpensesByCategory = append(response.ExpensesByCategory, CategoryTotalResponse{Category: item.Category, TotalMinor: item.TotalMinor})
	}
	c.JSON(http.StatusOK, response)
}
