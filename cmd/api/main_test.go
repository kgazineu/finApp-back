package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kgazineu/finApp-back/internal/api"

	"github.com/gin-gonic/gin"
)

func TestHealthCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := newRouter(api.NewServer())

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Errorf(
			"status esperado %d, recebido %d",
			http.StatusOK,
			response.Code,
		)
	}

	var body struct {
		Status string `json:"status"`
	}

	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("não foi possível interpretar a resposta: %v", err)
	}

	if body.Status != "ok" {
		t.Errorf("status esperado %q, recebido %q", "ok", body.Status)
	}
}

func TestOpenAPISpec(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newRouter(api.NewServer())

	request := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"status esperado %d, recebido %d",
			http.StatusOK,
			response.Code,
		)
	}

	var spec struct {
		OpenAPI string                     `json:"openapi"`
		Paths   map[string]json.RawMessage `json:"paths"`
	}

	if err := json.Unmarshal(response.Body.Bytes(), &spec); err != nil {
		t.Fatalf("não foi possível interpretar a especificação: %v", err)
	}

	if spec.OpenAPI != "3.0.3" {
		t.Errorf("versão OpenAPI esperada %q, recebida %q", "3.0.3", spec.OpenAPI)
	}

	if _, exists := spec.Paths["/health"]; !exists {
		t.Error("a rota /health não foi encontrada na especificação")
	}
}

func TestSwaggerDocs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newRouter(api.NewServer())

	request := httptest.NewRequest(http.MethodGet, "/docs/", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"status esperado %d, recebido %d",
			http.StatusOK,
			response.Code,
		)
	}

	if !strings.Contains(response.Body.String(), "FinApp API") {
		t.Error("a página da documentação não contém o título da API")
	}
}
