package config_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/kgazineu/finApp-back/internal/config"
)

func cleanEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"DATABASE_URL", "HTTP_ADDR", "POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB", "POSTGRES_SSLMODE"} {
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
