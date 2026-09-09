package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/swaggest/swgui/v5emb"
)

func RegisterDocumentation(router gin.IRouter) {
	router.GET("/openapi.json", func(c *gin.Context) {
		spec, err := GetSpecJSON()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "não foi possível carregar a especificação OpenAPI",
			})
			return
		}

		c.Data(http.StatusOK, "application/json; charset=utf-8", spec)
	})

	router.GET("/docs", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/docs/")
	})

	docsHandler := v5emb.New(
		"FinApp API",
		"/openapi.json",
		"/docs/",
	)
	router.GET("/docs/*any", gin.WrapH(docsHandler))
}
