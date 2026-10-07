package api

import (
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
)

// CORS libera chamadas do navegador vindas dos sites em origins (CORS_ALLOWED_ORIGINS),
// por exemplo o front servido por outro túnel/domínio. Sem origins, nada muda.
// A autenticação é por header Bearer, não por cookie, então não há credenciais implícitas.
func CORS(origins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" || !slices.Contains(origins, origin) {
			c.Next()
			return
		}

		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Add("Vary", "Origin")

		// preflight: o navegador pergunta antes de mandar Authorization/JSON
		if c.Request.Method == http.MethodOptions {
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Max-Age", "600")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
