package account

import (
	"github.com/gin-gonic/gin"
)

func (m *Module) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("", m.controller.FindActive)
	r.POST("", m.controller.Create)
	r.GET("/:id", m.controller.FindByID)
	r.PATCH("/:id", m.controller.Update)
	r.DELETE("/:id", m.controller.Delete)
}
