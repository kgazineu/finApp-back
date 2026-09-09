package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Server struct{}

var _ ServerInterface = (*Server)(nil)

func NewServer() *Server {
	return &Server{}
}

func (s *Server) GetHealth(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{
		Status: "ok",
	})
}
