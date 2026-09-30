package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/auth"
	"github.com/kgazineu/finApp-back/internal/transaction"
)

func decodeFinancialJSON(c *gin.Context, target any) bool {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		c.JSON(http.StatusUnsupportedMediaType, ErrorResponse{Message: "Use Content-Type application/json"})
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeBodyError(c, err)
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeBodyError(c, err)
		return false
	}
	return true
}

func (s *Server) CreateSession(c *gin.Context) {
	var request CreateSessionRequest
	if !decodeFinancialJSON(c, &request) {
		return
	}
	if request.Email == "" || request.Password == nil || *request.Password == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: "E-mail e senha são obrigatórios"})
		return
	}
	if s.sessions == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível autenticar"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	token, expiresAt, err := s.sessions.Login(ctx, string(request.Email), *request.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "Credenciais inválidas"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível autenticar"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, SessionResponse{Token: token, TokenType: Bearer, ExpiresAt: expiresAt})
}

func (s *Server) CreateTransaction(c *gin.Context) {
	if s.sessions == nil || s.transactions == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível criar a transação"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	userID, ok := s.sessionUser(c, ctx)
	if !ok {
		return
	}
	var request CreateTransactionRequest
	if !decodeFinancialJSON(c, &request) {
		return
	}
	if request.AmountMinor <= 0 || request.NecessityLevel < 1 || request.NecessityLevel > 5 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: "Valor ou nível de necessidade inválido"})
		return
	}
	created, err := s.transactions.Create(ctx, transaction.CreateInput{
		UserID: userID, AmountMinor: request.AmountMinor, NecessityLevel: request.NecessityLevel,
	})
	if err != nil {
		if errors.Is(err, transaction.ErrInvalidAmount) || errors.Is(err, transaction.ErrInvalidNecessityLevel) || errors.Is(err, transaction.ErrInvalidUserID) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível criar a transação"})
		return
	}
	c.JSON(http.StatusCreated, TransactionResponse{
		Id: created.ID, AmountMinor: created.AmountMinor,
		NecessityLevel: created.NecessityLevel, CreatedAt: created.CreatedAt,
	})
}

func (s *Server) sessionUser(c *gin.Context, ctx context.Context) (uuid.UUID, bool) {
	authHeaders := c.Request.Header.Values("Authorization")
	if len(authHeaders) != 1 {
		writeUnauthorized(c)
		return uuid.Nil, false
	}
	scheme, token, ok := strings.Cut(authHeaders[0], " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.ContainsAny(token, " \t\r\n") {
		writeUnauthorized(c)
		return uuid.Nil, false
	}
	userID, err := s.sessions.Authenticate(ctx, token)
	if errors.Is(err, auth.ErrInvalidSession) {
		writeUnauthorized(c)
		return uuid.Nil, false
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Não foi possível autenticar"})
		return uuid.Nil, false
	}
	if userID == uuid.Nil {
		writeUnauthorized(c)
		return uuid.Nil, false
	}
	return userID, true
}

func writeUnauthorized(c *gin.Context) {
	c.Header("WWW-Authenticate", "Bearer")
	c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "Sessão inválida ou expirada"})
}
