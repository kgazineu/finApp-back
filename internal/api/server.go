package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Server struct {
	users        UserService
	sessions     SessionService
	transactions TransactionService
	goals        GoalService
	overview     OverviewService
}

var _ ServerInterface = (*Server)(nil)

type ServerOption func(*Server)

func WithFinancialServices(sessions SessionService, transactions TransactionService) ServerOption {
	return func(s *Server) {
		s.sessions = sessions
		s.transactions = transactions
	}
}

func WithGoalService(goals GoalService) ServerOption {
	return func(s *Server) { s.goals = goals }
}

func WithOverviewService(overview OverviewService) ServerOption {
	return func(s *Server) { s.overview = overview }
}

func NewServer(users UserService, options ...ServerOption) *Server {
	s := &Server{users: users}
	for _, option := range options {
		option(s)
	}
	return s
}

func (s *Server) GetHealth(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{
		Status: "ok",
	})
}
