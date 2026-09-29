package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kgazineu/finApp-back/internal/user"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (s *Server) UpdateUser(
	c *gin.Context,
	id openapi_types.UUID,
) {
	mediaType, _, err := mime.ParseMediaType(
		c.GetHeader("Content-Type"),
	)
	if err != nil || mediaType != "application/json" {
		c.JSON(
			http.StatusUnsupportedMediaType,
			ErrorResponse{
				Message: "Use Content-Type application/json",
			},
		)
		return
	}

	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		64*1024,
	)

	decoder := json.NewDecoder(c.Request.Body)

	var request UpdateUserRequest

	if err := decoder.Decode(&request); err != nil {
		writeBodyError(c, err)
		return
	}

	if err := decoder.Decode(new(any)); err != io.EOF {
		writeBodyError(c, err)
		return
	}

	if s.users == nil {
		c.JSON(
			http.StatusInternalServerError,
			ErrorResponse{
				Message: "Não foi possível atualizar o usuário",
			},
		)
		return
	}

	var email *string

	if request.Email != nil {
		value := string(*request.Email)
		email = &value
	}

	input := user.UpdateInput{
		ID:    id,
		Name:  request.Name,
		Email: email,
	}

	ctx, cancel := context.WithTimeout(
		c.Request.Context(),
		10*time.Second,
	)
	defer cancel()

	updated, err := s.users.Update(ctx, input)
	if err != nil {
		status := http.StatusInternalServerError
		message := "Não foi possível atualizar o usuário"

		switch {
		case errors.Is(err, user.ErrInvalidUserID),
			errors.Is(err, user.ErrNoFieldsToUpdate),
			errors.Is(err, user.ErrInvalidName),
			errors.Is(err, user.ErrInvalidEmail):
			status = http.StatusBadRequest
			message = err.Error()

		case errors.Is(err, user.ErrUserNotFound):
			status = http.StatusNotFound
			message = user.ErrUserNotFound.Error()

		case errors.Is(err, user.ErrEmailAlreadyExists):
			status = http.StatusConflict
			message = user.ErrEmailAlreadyExists.Error()
		}

		c.JSON(status, ErrorResponse{Message: message})
		return
	}

	c.JSON(http.StatusOK, UserResponse{
		Id:        updated.ID,
		Name:      updated.Name,
		Email:     openapi_types.Email(updated.Email),
		CreatedAt: updated.CreatedAt,
		UpdatedAt: updated.UpdatedAt,
	})
}
