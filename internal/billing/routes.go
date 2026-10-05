package billing

import (
	"github.com/gin-gonic/gin"
)

func (m *Module) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("", m.controller.NextMonthProjectionController)
	r.POST("", m.controller.CreateController)
}
