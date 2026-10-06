# Atalhos de desenvolvimento da API. `make` sem argumentos lista os comandos.
# As variáveis vêm do .env (copie de .env.example).

AIR_VERSION := v1.67.4
AIR := $(shell go env GOPATH)/bin/air

# carrega o .env no shell da receita, não no make: assim um "$" na senha continua intacto
LOAD_ENV = test -f .env || { echo "falta o .env: cp .env.example .env"; exit 1; }; set -a; . ./.env; set +a;

.DEFAULT_GOAL := help
.PHONY: help dev run test migration migrate-up migrate-down migrate-version migrate-force docker-up docker-down docker-logs

help: ## lista os comandos
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  make %-16s %s\n", $$1, $$2}'

dev: ## API local com hot reload (air); instala o air na primeira vez
	@test -x $(AIR) || go install github.com/air-verse/air@$(AIR_VERSION)
	@$(LOAD_ENV) $(AIR)

run: ## API local sem hot reload
	@$(LOAD_ENV) go run ./cmd/api

test: ## testes (integração só com TEST_DATABASE_URL no .env, apontando para um banco descartável)
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi; go test ./...

migration: ## cria o par up/down da próxima migration: make migration name=create_budgets
	@case "$(name)" in ""|*[!a-z0-9_]*) echo "uso: make migration name=create_budgets (letras minúsculas, números e _)"; exit 1;; esac
	@last=$$(ls migrations/*.up.sql | sed -E 's#.*/0*([0-9]+)_.*#\1#' | sort -n | tail -1); \
	next=$$(printf '%06d' $$((last + 1))); \
	touch migrations/$${next}_$(name).up.sql migrations/$${next}_$(name).down.sql; \
	echo "criadas migrations/$${next}_$(name).up.sql e .down.sql"

migrate-up: ## aplica as migrations pendentes (a API também faz isso ao iniciar)
	@$(LOAD_ENV) go run ./cmd/migrate up

migrate-down: ## desfaz a última migration (ou as últimas N: make migrate-down n=3)
	@$(LOAD_ENV) go run ./cmd/migrate down $(or $(n),1)

migrate-version: ## mostra a versão aplicada no banco
	@$(LOAD_ENV) go run ./cmd/migrate version

migrate-force: ## destrava migration que falhou no meio (dirty): make migrate-force version=9
	@test -n "$(version)" || { echo "uso: make migrate-force version=N"; exit 1; }
	@$(LOAD_ENV) go run ./cmd/migrate force $(version)

docker-up: ## sobe o PostgreSQL no Docker (a API roda com make dev)
	docker compose up -d --build

docker-down: ## para o PostgreSQL (os dados ficam no volume)
	docker compose down

docker-logs: ## acompanha os logs do PostgreSQL
	docker compose logs -f db
