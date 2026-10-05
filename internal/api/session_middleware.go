package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const userIDKey = "userID"

// RequireSession is the Gin middleware for routes registered outside the OpenAPI spec
// (accounts, billings, recurring transactions and receivables). It validates the Bearer
// token exactly like the generated handlers do and exposes the owner through UserID.
func (s *Server) RequireSession(c *gin.Context) {
	if s.sessions == nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível autenticar"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	userID, ok := s.sessionUser(c, ctx)
	if !ok {
		c.Abort()
		return
	}
	c.Set(userIDKey, userID)
}

// UserID returns the authenticated user set by RequireSession.
func UserID(c *gin.Context) uuid.UUID {
	return c.MustGet(userIDKey).(uuid.UUID)
}
