package config

import (
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL string
	HTTPAddr    string
	// Production vem de GIN_MODE=release (compose.prod.yaml).
	Production bool
	SMTP       SMTP
	// CORSOrigins: sites que podem chamar a API pelo navegador (CORS_ALLOWED_ORIGINS, separados por vírgula)
	CORSOrigins []string
}

// SMTP configura o envio do código de recuperação de senha. Host vazio = e-mail vai para o log.
type SMTP struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

// Load reads configuration from the process environment. It never reads .env files.
// Every problem is reported at once, so a misconfigured deploy fails at startup listing all of them.
func Load() (Config, error) {
	cfg := Config{HTTPAddr: value("HTTP_ADDR", ":8080"), Production: os.Getenv("GIN_MODE") == "release"}
	var problems []error
	if _, _, err := net.SplitHostPort(cfg.HTTPAddr); err != nil {
		problems = append(problems, errors.New("HTTP_ADDR deve conter host:porta"))
	}

	dsn, err := DatabaseURL()
	problems = append(problems, err)
	cfg.DatabaseURL = dsn

	cfg.SMTP, err = loadSMTP(cfg.Production)
	problems = append(problems, err)

	cfg.CORSOrigins, err = loadCORSOrigins()
	problems = append(problems, err)

	if err := errors.Join(problems...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// DatabaseURL monta a URL do PostgreSQL a partir de DATABASE_URL ou de POSTGRES_*.
// Também é usada pelo comando de migrations, que não precisa do resto da configuração.
func DatabaseURL() (string, error) {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		u, err := url.Parse(dsn)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || len(u.Path) < 2 {
			return "", errors.New("DATABASE_URL deve ser uma URL PostgreSQL com host e banco")
		}
		q := u.Query()
		if q.Get("connect_timeout") == "" {
			q.Set("connect_timeout", "5")
		}
		u.RawQuery = q.Encode()
		return u.String(), nil
	}

	var problems []error
	password := os.Getenv("POSTGRES_PASSWORD")
	if password == "" {
		problems = append(problems, errors.New("defina DATABASE_URL ou POSTGRES_PASSWORD"))
	}
	port := value("POSTGRES_PORT", "5432")
	if !validPort(port) {
		problems = append(problems, errors.New("POSTGRES_PORT inválida"))
	}
	if err := errors.Join(problems...); err != nil {
		return "", err
	}

	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(value("POSTGRES_USER", "finapp"), password),
		Host:   net.JoinHostPort(value("POSTGRES_HOST", "localhost"), port),
		Path:   "/" + value("POSTGRES_DB", "finapp"),
	}
	q := url.Values{"sslmode": {value("POSTGRES_SSLMODE", "disable")}, "connect_timeout": {"5"}}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func loadSMTP(production bool) (SMTP, error) {
	s := SMTP{
		Host:     os.Getenv("SMTP_HOST"),
		Port:     value("SMTP_PORT", "587"),
		Username: os.Getenv("SMTP_USERNAME"),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     value("SMTP_FROM", os.Getenv("SMTP_USERNAME")),
	}

	if s.Host == "" {
		if production {
			return s, errors.New("SMTP_HOST é obrigatório em produção (GIN_MODE=release): sem ele os códigos de recuperação de senha iriam para o log")
		}
		return s, nil
	}

	var problems []error
	if !validPort(s.Port) {
		problems = append(problems, errors.New("SMTP_PORT inválida"))
	}
	if addr, err := mail.ParseAddress(s.From); err != nil || addr.Address != s.From {
		problems = append(problems, errors.New("SMTP_FROM deve ser o e-mail remetente (ou defina SMTP_USERNAME com um e-mail)"))
	}
	if s.Username != "" && s.Password == "" {
		problems = append(problems, errors.New("SMTP_PASSWORD é obrigatório quando SMTP_USERNAME está definido"))
	}
	return s, errors.Join(problems...)
}

// loadCORSOrigins aceita só origens (esquema + host[:porta]), como o navegador manda no header Origin.
func loadCORSOrigins() ([]string, error) {
	var origins []string
	for _, raw := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		origin := strings.TrimSuffix(strings.TrimSpace(raw), "/")
		if origin == "" {
			continue
		}
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" || u.RawQuery != "" {
			return nil, fmt.Errorf("CORS_ALLOWED_ORIGINS: %q deve ser só o endereço do site, como https://exemplo.com", raw)
		}
		origins = append(origins, origin)
	}
	return origins, nil
}

func validPort(port string) bool {
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
