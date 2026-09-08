package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func newRouter() *gin.Engine {
	router := gin.Default()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	return router
}

func main() {
	router := newRouter()

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}
}
