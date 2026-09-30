package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/goal"
)

func goalResponse(item goal.Goal) GoalResponse {
	return GoalResponse{
		Id: item.ID, Name: item.Name, TargetMinor: item.TargetMinor, SavedMinor: item.SavedMinor,
		CreatedAt: item.CreatedAt.UTC(), UpdatedAt: item.UpdatedAt.UTC(),
	}
}

func (s *Server) CreateGoal(c *gin.Context) {
	if s.sessions == nil || s.goals == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível criar a meta"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	userID, ok := s.sessionUser(c, ctx)
	if !ok {
		return
	}
	var request CreateGoalRequest
	if !decodeFinancialJSON(c, &request) {
		return
	}
	if strings.TrimSpace(request.Name) == "" || request.TargetMinor <= 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: "Nome e valor-alvo positivo são obrigatórios"})
		return
	}
	item, err := s.goals.Create(ctx, goal.CreateInput{UserID: userID, Name: request.Name, TargetMinor: request.TargetMinor})
	if err != nil {
		if errors.Is(err, goal.ErrInvalidName) || errors.Is(err, goal.ErrInvalidTarget) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível criar a meta"})
		return
	}
	c.JSON(http.StatusCreated, goalResponse(item))
}

func (s *Server) ListGoals(c *gin.Context, params ListGoalsParams) {
	if s.sessions == nil || s.goals == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível listar as metas"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	userID, ok := s.sessionUser(c, ctx)
	if !ok {
		return
	}
	for _, key := range []string{"limit", "offset"} {
		if values, present := c.Request.URL.Query()[key]; present && (len(values) != 1 || values[0] == "") {
			c.JSON(http.StatusBadRequest, ErrorResponse{Message: goal.ErrInvalidPagination.Error()})
			return
		}
	}
	input := goal.ListInput{UserID: userID, Limit: goal.DefaultListLimit}
	if params.Limit != nil {
		input.Limit = *params.Limit
	}
	if params.Offset != nil {
		input.Offset = *params.Offset
	}
	if input.Limit < 1 || input.Limit > goal.MaxListLimit || input.Offset < 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: goal.ErrInvalidPagination.Error()})
		return
	}
	items, err := s.goals.List(ctx, input)
	if err != nil {
		if errors.Is(err, goal.ErrInvalidPagination) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Message: goal.ErrInvalidPagination.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível listar as metas"})
		return
	}
	data := make([]GoalResponse, 0, len(items))
	for _, item := range items {
		data = append(data, goalResponse(item))
	}
	c.JSON(http.StatusOK, ListGoalsResponse{Data: data, Limit: input.Limit, Offset: input.Offset})
}

func (s *Server) AdjustGoal(c *gin.Context, id uuid.UUID) {
	if s.sessions == nil || s.goals == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível ajustar a meta"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	userID, ok := s.sessionUser(c, ctx)
	if !ok {
		return
	}
	if id == uuid.Nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Message: goal.ErrNotFound.Error()})
		return
	}
	var request AdjustGoalRequest
	if !decodeFinancialJSON(c, &request) {
		return
	}
	if request.AmountMinor <= 0 || (request.Direction != Deposit && request.Direction != Withdraw) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: "Operação ou valor inválido"})
		return
	}
	var item goal.Goal
	var err error
	if request.Direction == Deposit {
		item, err = s.goals.Deposit(ctx, userID, id, request.AmountMinor)
	} else {
		item, err = s.goals.Withdraw(ctx, userID, id, request.AmountMinor)
	}
	if err != nil {
		switch {
		case errors.Is(err, goal.ErrNotFound):
			c.JSON(http.StatusNotFound, ErrorResponse{Message: goal.ErrNotFound.Error()})
		case errors.Is(err, goal.ErrInvalidAmount), errors.Is(err, goal.ErrInsufficientFunds):
			c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível ajustar a meta"})
		}
		return
	}
	c.JSON(http.StatusOK, goalResponse(item))
}
