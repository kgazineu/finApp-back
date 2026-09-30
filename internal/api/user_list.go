package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kgazineu/finApp-back/internal/user"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (s *Server) ListUsers(c *gin.Context, params ListUsersParams) {
	if s.sessions == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível listar os usuários"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	userID, ok := s.sessionUser(c, ctx)
	if !ok {
		return
	}
	if s.users == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível listar os usuários"})
		return
	}
	// Reject empty or repeated parameters instead of silently applying defaults.
	query := c.Request.URL.Query()
	for _, key := range []string{"limit", "offset"} {
		if values, ok := query[key]; ok && (len(values) != 1 || values[0] == "") {
			c.JSON(http.StatusBadRequest, ErrorResponse{Message: user.ErrInvalidPagination.Error()})
			return
		}
	}
	input := user.ListInput{Limit: user.DefaultListLimit}
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
	data := make([]UserResponse, 0, 1)
	if err := ctx.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível listar os usuários"})
		return
	}
	if input.Offset > 0 {
		c.JSON(http.StatusOK, ListUsersResponse{Data: data, Limit: input.Limit, Offset: input.Offset})
		return
	}

	u, err := s.users.Get(ctx, userID)
	if err != nil {
		status, message := http.StatusInternalServerError, "Não foi possível listar os usuários"
		if errors.Is(err, user.ErrUserNotFound) {
			status, message = http.StatusNotFound, user.ErrUserNotFound.Error()
		}
		c.JSON(status, ErrorResponse{Message: message})
		return
	}

	data = append(data, UserResponse{
		Id: u.ID, Name: u.Name, Email: openapi_types.Email(u.Email),
		CreatedAt: u.CreatedAt.UTC(), UpdatedAt: u.UpdatedAt.UTC(),
	})
	c.JSON(http.StatusOK, ListUsersResponse{Data: data, Limit: input.Limit, Offset: input.Offset})
}
