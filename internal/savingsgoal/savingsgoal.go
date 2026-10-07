// Package savingsgoal guarda a meta de quanto o usuário quer guardar por mês: uma porcentagem do que
// sobra no mês ou um valor fixo. É só um número para a tela de projeção (quanto guardar e quanto sobra
// para gastar); nenhum cálculo da API usa a meta.
package savingsgoal

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kgazineu/finApp-back/internal/api"
)

// Goal tem no máximo um dos dois; os dois nulos = sem meta.
type Goal struct {
	Percent *int   `json:"percent" db:"percent" binding:"omitempty,gte=1,lte=100"` // % do que sobra no mês
	Amount  *int64 `json:"amount" db:"amount" binding:"omitempty,gt=0"`            // valor fixo por mês, em centavos
}

type Module struct {
	db *sqlx.DB
}

func NewModule(db *sqlx.DB) *Module {
	return &Module{db: db}
}

func (m *Module) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("", m.get)
	r.PUT("", m.put)
}

func (m *Module) get(c *gin.Context) {
	var goal Goal
	err := m.db.GetContext(c.Request.Context(), &goal, `select percent, amount from savings_goals where user_id = $1`, api.UserID(c))
	if errors.Is(err, sql.ErrNoRows) {
		goal = Goal{} // o sqlx aloca os ponteiros antes de saber que não há linha: sem isso viriam zeros
	} else if err != nil {
		api.InternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, goal)
}

// put troca a meta; sem porcentagem nem valor, remove.
func (m *Module) put(c *gin.Context) {
	var goal Goal
	if err := c.ShouldBindJSON(&goal); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": api.BindError(err)})
		return
	}
	if goal.Percent != nil && goal.Amount != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Escolha porcentagem ou valor, não os dois"})
		return
	}

	ctx, userID := c.Request.Context(), api.UserID(c)
	var err error
	if goal.Percent == nil && goal.Amount == nil {
		_, err = m.db.ExecContext(ctx, `delete from savings_goals where user_id = $1`, userID)
	} else {
		_, err = m.db.ExecContext(ctx, `
			insert into savings_goals (user_id, percent, amount) values ($1, $2, $3)
			on conflict (user_id) do update set percent = excluded.percent, amount = excluded.amount, updated_at = now()
		`, userID, goal.Percent, goal.Amount)
	}
	if err != nil {
		api.InternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, goal)
}
