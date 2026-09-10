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

func (s *Server) CreateUser(c *gin.Context) {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		c.JSON(http.StatusUnsupportedMediaType, ErrorResponse{Message: "Use Content-Type application/json"})
		return
	}

	// Limit the body before decoding, including requests without Content-Length.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024)
	decoder := json.NewDecoder(c.Request.Body)
	var request CreateUserRequest
	if err := decoder.Decode(&request); err != nil {
		writeBodyError(c, err)
		return
	}
	// Accept exactly one JSON value; do not silently ignore a trailing payload.
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeBodyError(c, err)
		return
	}
	if request.Name == "" || request.Email == "" || request.Password == nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Message: "Nome, e-mail e senha são obrigatórios",
		})
		return
	}

	if s.users == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Message: "Não foi possível cadastrar o usuário",
		})
		return
	}

	input := user.CreateInput{
		Name:     request.Name,
		Email:    string(request.Email),
		Password: *request.Password,
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	created, err := s.users.Create(ctx, input)
	if err != nil {
		status, message := http.StatusInternalServerError, "Não foi possível cadastrar o usuário"
		switch {
		case errors.Is(err, user.ErrInvalidName):
			status, message = http.StatusBadRequest, user.ErrInvalidName.Error()
		case errors.Is(err, user.ErrInvalidEmail):
			status, message = http.StatusBadRequest, user.ErrInvalidEmail.Error()
		case errors.Is(err, user.ErrInvalidPassword):
			status, message = http.StatusBadRequest, user.ErrInvalidPassword.Error()
		case errors.Is(err, user.ErrEmailAlreadyExists):
			status, message = http.StatusConflict, user.ErrEmailAlreadyExists.Error()
		}
		c.JSON(status, ErrorResponse{Message: message})
		return
	}

	c.JSON(http.StatusCreated, UserResponse{
		Id:        created.ID,
		Name:      created.Name,
		Email:     openapi_types.Email(created.Email),
		CreatedAt: created.CreatedAt,
		UpdatedAt: created.UpdatedAt,
	})
}

func writeBodyError(c *gin.Context, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{Message: "Corpo da requisição excede 64 KiB"})
		return
	}
	c.JSON(http.StatusBadRequest, ErrorResponse{Message: "Corpo da requisição inválido"})
}
