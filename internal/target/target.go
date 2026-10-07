// Package target guarda as metas: chegar a um valor até uma data, olhando o saldo total ou o de uma
// conta de saldo (fatura não). A API guarda só o alvo; o progresso e a previsão de quando chega saem do
// último registro de saldos e do crescimento por mês, calculados na tela.
// Diferente de internal/goal, que são envelopes virtuais com depósitos feitos à mão.
package target

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kgazineu/finApp-back/internal/account"
	"github.com/kgazineu/finApp-back/internal/api"
)

type Target struct {
	ID        int64     `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Amount    int64     `json:"amount" db:"amount"`
	Deadline  string    `json:"deadline" db:"deadline"`    // AAAA-MM-DD
	AccountID *int64    `json:"accountId" db:"account_id"` // null = saldo total
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

// Request é o corpo do POST e do PATCH (o PATCH substitui tudo).
type Request struct {
	Name      string `json:"name" binding:"required"`
	Amount    int64  `json:"amount" binding:"required,gt=0"`
	Deadline  string `json:"deadline" binding:"required"`
	AccountID *int64 `json:"accountId"`
}

const columns = `id, name, amount, deadline::text as deadline, account_id, created_at`

type Module struct {
	db *sqlx.DB
}

func NewModule(db *sqlx.DB) *Module {
	return &Module{db: db}
}

func (m *Module) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("", m.list)
	r.POST("", m.create)
	r.PATCH("/:id", m.update)
	r.DELETE("/:id", m.remove)
}

func (m *Module) list(c *gin.Context) {
	targets := []Target{}
	err := m.db.SelectContext(c.Request.Context(), &targets,
		`select `+columns+` from targets where user_id = $1 order by deadline, id`, api.UserID(c))
	if err != nil {
		api.InternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, targets)
}

func (m *Module) create(c *gin.Context) {
	req, ok := m.bind(c)
	if !ok {
		return
	}
	var t Target
	err := m.db.GetContext(c.Request.Context(), &t, `
		insert into targets (user_id, name, amount, deadline, account_id) values ($1, $2, $3, $4, $5)
		returning `+columns, api.UserID(c), req.Name, req.Amount, req.Deadline, req.AccountID)
	if err != nil {
		api.InternalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, t)
}

func (m *Module) update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id inválido"})
		return
	}
	req, ok := m.bind(c)
	if !ok {
		return
	}
	var t Target
	err = m.db.GetContext(c.Request.Context(), &t, `
		update targets set name = $1, amount = $2, deadline = $3, account_id = $4 where id = $5 and user_id = $6
		returning `+columns, req.Name, req.Amount, req.Deadline, req.AccountID, id, api.UserID(c))
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"message": "meta não encontrada"})
		return
	}
	if err != nil {
		api.InternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

func (m *Module) remove(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id inválido"})
		return
	}
	res, err := m.db.ExecContext(c.Request.Context(), `delete from targets where id = $1 and user_id = $2`, id, api.UserID(c))
	if err != nil {
		api.InternalError(c, err)
		return
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "meta não encontrada"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "meta apagada"})
}

// bind valida o corpo: nome, valor > 0, prazo em AAAA-MM-DD e, se vier, uma conta ativa de saldo do usuário.
func (m *Module) bind(c *gin.Context) (Request, bool) {
	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": api.BindError(err)})
		return req, false
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "O campo nome é obrigatório"})
		return req, false
	}
	if _, err := time.Parse(time.DateOnly, req.Deadline); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Prazo inválido: use AAAA-MM-DD"})
		return req, false
	}
	if req.AccountID == nil {
		return req, true
	}

	var kind account.Kind
	err := m.db.GetContext(c.Request.Context(), &kind,
		`select kind from accounts where id = $1 and user_id = $2 and archived_at is null`, *req.AccountID, api.UserID(c))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		c.JSON(http.StatusBadRequest, gin.H{"message": "Conta não encontrada"})
	case err != nil:
		api.InternalError(c, err)
	case kind == account.KindLiability:
		c.JSON(http.StatusBadRequest, gin.H{"message": "A meta precisa ser de uma conta de saldo: fatura não vale"})
	default:
		return req, true
	}
	return req, false
}
