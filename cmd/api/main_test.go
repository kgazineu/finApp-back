package main

import (
	"context"
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

	router := newRouter(api.NewServer(nil))

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
	router := newRouter(api.NewServer(nil))

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
	if _, exists := spec.Paths["/sessions"]; !exists {
		t.Error("POST /sessions não foi encontrado na especificação publicada")
	}
	var transactionOperations map[string]json.RawMessage
	if err := json.Unmarshal(spec.Paths["/transactions"], &transactionOperations); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"get", "post"} {
		if _, exists := transactionOperations[method]; !exists {
			t.Errorf("%s /transactions não foi encontrado na especificação publicada", method)
		}
	}
	// inclui as rotas registradas à mão (fora da interface gerada), documentadas pelo spec.gen.go
	for path, methods := range map[string][]string{
		"/dashboard":                          {"get"},
		"/goals":                              {"get", "post"},
		"/goals/{id}/allocations":             {"post"},
		"/accounts":                           {"get", "post"},
		"/accounts/{id}":                      {"get", "patch", "delete"},
		"/billings":                           {"get", "post"},
		"/recurring-transactions":             {"get", "post"},
		"/recurring-transactions/occurrences": {"get"},
		"/recurring-transactions/{id}":        {"patch", "delete"},
		"/recurring-transactions/installments/{id}": {"patch"},
		"/receivables":                   {"get", "post"},
		"/receivables/{id}":              {"patch", "delete"},
		"/receivables/installments/{id}": {"patch"},
		"/password-resets":               {"post"},
		"/password-resets/verify":        {"post"},
		"/password-resets/confirm":       {"post"},
		"/export":                        {"get"},
		"/savings-goal":                  {"get", "put"},
		"/targets":                       {"get", "post"},
		"/targets/{id}":                  {"patch", "delete"},
		"/import":                        {"post"},
	} {
		var operations map[string]json.RawMessage
		if err := json.Unmarshal(spec.Paths[path], &operations); err != nil {
			t.Fatal(err)
		}
		for _, method := range methods {
			if _, exists := operations[method]; !exists {
				t.Errorf("%s %s não foi encontrado na especificação publicada", method, path)
			}
		}
	}
	var operations map[string]json.RawMessage
	if err := json.Unmarshal(spec.Paths["/users"], &operations); err != nil {
		t.Fatal(err)
	}
	if _, exists := operations["get"]; !exists {
		t.Error("GET /users não foi encontrado na especificação publicada")
	}
	if _, exists := operations["post"]; !exists {
		t.Error("POST /users deve continuar na especificação publicada")
	}
	document, err := api.GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	if err := document.Validate(context.Background()); err != nil {
		t.Fatalf("especificação OpenAPI inválida: %v", err)
	}
}

func TestSwaggerDocs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newRouter(api.NewServer(nil))

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

// o front em outro endereço (túnel) só consegue chamar métodos que o preflight libera
func TestCORSPreflightAllowsEveryMethodTheAPIUses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newRouter(api.NewServer(nil), "http://front.test")
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req := httptest.NewRequest(http.MethodOptions, "/savings-goal", nil)
		req.Header.Set("Origin", "http://front.test")
		req.Header.Set("Access-Control-Request-Method", method)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if allowed := res.Header().Get("Access-Control-Allow-Methods"); res.Code != http.StatusNoContent || !strings.Contains(allowed, method) {
			t.Fatalf("preflight de %s: HTTP %d, métodos liberados %q", method, res.Code, allowed)
		}
	}
}
