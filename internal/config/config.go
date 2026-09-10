package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL string
	HTTPAddr    string
}

// Load reads configuration from the process environment. It never reads .env files.
func Load() (Config, error) {
	cfg := Config{HTTPAddr: value("HTTP_ADDR", ":8080")}
	if _, _, err := net.SplitHostPort(cfg.HTTPAddr); err != nil {
		return Config{}, errors.New("HTTP_ADDR deve conter host:porta")
	}
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		u, err := url.Parse(dsn)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || len(u.Path) < 2 {
			return Config{}, errors.New("DATABASE_URL deve ser uma URL PostgreSQL com host e banco")
		}
		q := u.Query()
		if q.Get("connect_timeout") == "" {
			q.Set("connect_timeout", "5")
		}
		u.RawQuery = q.Encode()
		cfg.DatabaseURL = u.String()
		return cfg, nil
	}
	password := os.Getenv("POSTGRES_PASSWORD")
	if password == "" {
		return Config{}, errors.New("defina DATABASE_URL ou POSTGRES_PASSWORD")
	}
	port := value("POSTGRES_PORT", "5432")
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return Config{}, errors.New("POSTGRES_PORT inválida")
	}
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(value("POSTGRES_USER", "finapp"), password),
		Host:   net.JoinHostPort(value("POSTGRES_HOST", "localhost"), port),
		Path:   "/" + value("POSTGRES_DB", "finapp"),
	}
	q := url.Values{"sslmode": {value("POSTGRES_SSLMODE", "disable")}, "connect_timeout": {"5"}}
	u.RawQuery = q.Encode()
	cfg.DatabaseURL = u.String()
	return cfg, nil
}

func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
