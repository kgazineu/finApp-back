package main

import (
	"github.com/kgazineu/finApp-back/internal/api"

	"github.com/gin-gonic/gin"
)

func newRouter(server api.ServerInterface) *gin.Engine {
	router := gin.Default()

	api.RegisterHandlers(router, server)
	api.RegisterDocumentation(router)

	return router
}

func main() {
	server := api.NewServer()
	router := newRouter(server)

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}
}
