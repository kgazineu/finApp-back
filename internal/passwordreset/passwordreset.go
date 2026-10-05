// Package passwordreset recovers access by e-mail: a 6-digit one-time code (OTC)
// is mailed to the user, verified, and exchanged for a new password.
package passwordreset

import (
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type Module struct {
	controller *Controller
}

func NewModule(db *sqlx.DB, mailer Mailer, hasher Hasher) *Module {
	return &Module{controller: NewController(NewService(NewRepository(db), mailer, hasher))}
}

// RegisterRoutes mounts public routes: whoever forgot the password has no session.
func (m *Module) RegisterRoutes(r *gin.RouterGroup) {
	r.POST("", m.controller.Request)
	r.POST("/verify", m.controller.Verify)
	r.POST("/confirm", m.controller.Confirm)
}
