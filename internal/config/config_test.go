package config_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/kgazineu/finApp-back/internal/config"
)

func cleanEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"DATABASE_URL", "HTTP_ADDR", "POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB", "POSTGRES_SSLMODE",
		"GIN_MODE", "CORS_ALLOWED_ORIGINS", "SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM"} {
		t.Setenv(key, "")
	}
}

func TestLoadEscapesDatabaseCredentials(t *testing.T) {
	cleanEnv(t)
	password := "test@password:/?#%"
	t.Setenv("POSTGRES_PASSWORD", password)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := u.User.Password()
	if got != password {
		t.Error("senha alterada ao montar URL")
	}
	if cfg.HTTPAddr != ":8080" || u.Host != "localhost:5432" || u.Path != "/finapp" {
		t.Error("defaults inesperados")
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	for _, tt := range []struct{ name, key, value string }{
		{"credenciais ausentes", "POSTGRES_PASSWORD", ""},
		{"URL inválida", "DATABASE_URL", "postgres://user:secret%zz@localhost/db"},
		{"porta inválida", "POSTGRES_PORT", "-1"},
		{"endereço HTTP inválido", "HTTP_ADDR", "8080"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cleanEnv(t)
			t.Setenv("POSTGRES_PASSWORD", "test-password")
			t.Setenv(tt.key, tt.value)
			_, err := config.Load()
			if err == nil {
				t.Fatal("esperado erro")
			}
			if strings.Contains(err.Error(), "secret") {
				t.Error("credenciais expostas no erro")
			}
		})
	}
}

func TestDatabaseURLTakesPrecedence(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DATABASE_URL", "postgres://finapp:password@localhost:5432/finapp_test?sslmode=disable")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(cfg.DatabaseURL)
	if u.Path != "/finapp_test" || u.Query().Get("connect_timeout") != "5" {
		t.Error("URL não preservada")
	}
}

func TestLoadValidatesSMTP(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  map[string]string
		ok   bool
	}{
		{"desenvolvimento sem SMTP: código vai para o log", nil, true},
		{"produção sem SMTP", map[string]string{"GIN_MODE": "release"}, false},
		{"produção com SMTP", map[string]string{"GIN_MODE": "release", "SMTP_HOST": "smtp.example.com", "SMTP_USERNAME": "finapp@example.com", "SMTP_PASSWORD": "secret"}, true},
		{"sem remetente", map[string]string{"SMTP_HOST": "smtp.example.com"}, false},
		{"remetente com nome de exibição", map[string]string{"SMTP_HOST": "smtp.example.com", "SMTP_FROM": "FinApp <finapp@example.com>"}, false},
		{"usuário sem senha", map[string]string{"SMTP_HOST": "smtp.example.com", "SMTP_USERNAME": "finapp@example.com"}, false},
		{"porta inválida", map[string]string{"SMTP_HOST": "smtp.example.com", "SMTP_FROM": "finapp@example.com", "SMTP_PORT": "smtp"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cleanEnv(t)
			t.Setenv("POSTGRES_PASSWORD", "test-password")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			cfg, err := config.Load()
			if (err == nil) != tt.ok {
				t.Fatalf("ok esperado %v, erro %v", tt.ok, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Error("credenciais expostas no erro")
			}
			if tt.ok && tt.env["SMTP_USERNAME"] != "" && cfg.SMTP.From != tt.env["SMTP_USERNAME"] {
				t.Error("sem SMTP_FROM o remetente deve ser SMTP_USERNAME")
			}
		})
	}
}

func TestLoadReportsAllProblemsAtOnce(t *testing.T) {
	cleanEnv(t)
	t.Setenv("HTTP_ADDR", "8080")
	t.Setenv("POSTGRES_PORT", "-1")
	t.Setenv("GIN_MODE", "release")
	_, err := config.Load()
	if err == nil {
		t.Fatal("esperado erro")
	}
	for _, want := range []string{"HTTP_ADDR", "POSTGRES_PASSWORD", "POSTGRES_PORT", "SMTP_HOST"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("erro deveria citar %s: %v", want, err)
		}
	}
}

func TestLoadCORSOrigins(t *testing.T) {
	cleanEnv(t)
	t.Setenv("POSTGRES_PASSWORD", "test-password")
	t.Setenv("CORS_ALLOWED_ORIGINS", " https://web.example.com/ , http://localhost:5173")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.CORSOrigins) != 2 || cfg.CORSOrigins[0] != "https://web.example.com" || cfg.CORSOrigins[1] != "http://localhost:5173" {
		t.Fatalf("origens inesperadas: %q", cfg.CORSOrigins)
	}

	t.Setenv("CORS_ALLOWED_ORIGINS", "https://web.example.com/app")
	if _, err := config.Load(); err == nil {
		t.Error("origem com caminho deveria ser recusada")
	}
}
