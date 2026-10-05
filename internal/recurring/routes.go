package recurring

import (
	"github.com/gin-gonic/gin"
)

func (m *Module) RegisterRoutes(r *gin.RouterGroup) {
	r.POST("", m.controller.Create)
	r.GET("", m.controller.FindAll)
	r.GET("/occurrences", m.controller.FindByMonth)
	r.PATCH("/:id", m.controller.Update)
	r.DELETE("/:id", m.controller.Delete)
	r.GET("/installments/pending", m.controller.FindPendingInstallments)
	r.GET("/installments/paid", m.controller.FindPaidInstallments)
	r.PATCH("/installments/:id", m.controller.SetInstallmentPaid)
}
