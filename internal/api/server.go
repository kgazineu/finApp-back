package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Server struct {
	users UserService
}

var _ ServerInterface = (*Server)(nil)

func NewServer(users UserService) *Server {
	return &Server{
		users: users,
	}
}

func (s *Server) GetHealth(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{
		Status: "ok",
	})
}
