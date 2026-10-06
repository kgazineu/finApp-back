package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kgazineu/finApp-back/internal/api"
)

func TestCORSAllowsOnlyConfiguredOrigins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newRouter(api.NewServer(nil), "https://web.example.com")

	request := func(method, path, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}

	// preflight numa rota só com POST (o Gin não tem OPTIONS registrado)
	pre := request(http.MethodOptions, "/sessions", "https://web.example.com")
	if pre.Code != http.StatusNoContent || pre.Header().Get("Access-Control-Allow-Origin") != "https://web.example.com" ||
		pre.Header().Get("Access-Control-Allow-Headers") != "Authorization, Content-Type" {
		t.Fatalf("preflight: HTTP %d, headers %v", pre.Code, pre.Header())
	}

	if got := request(http.MethodGet, "/health", "https://web.example.com").Header().Get("Access-Control-Allow-Origin"); got != "https://web.example.com" {
		t.Errorf("resposta real sem Allow-Origin: %q", got)
	}

	other := request(http.MethodOptions, "/sessions", "https://evil.example.com")
	if other.Header().Get("Access-Control-Allow-Origin") != "" || other.Code == http.StatusNoContent {
		t.Errorf("origem não configurada liberada: HTTP %d", other.Code)
	}
}
