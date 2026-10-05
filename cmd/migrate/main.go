// Command migrate aplica ou desfaz as migrations embutidas, com a mesma configuração de banco
// da API (DATABASE_URL ou POSTGRES_*). A API já roda "up" sozinha ao iniciar; este comando
// existe para desfazer, consultar e destravar migrations. Use pelo Makefile (make migrate-*).
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/kgazineu/finApp-back/internal/config"
	"github.com/kgazineu/finApp-back/migrations"
)

const usage = `uso: migrate up | down [N] | version | force VERSÃO
  up          aplica as migrations pendentes
  down [N]    desfaz as últimas N migrations (padrão 1)
  version     mostra a versão aplicada
  force V     marca a versão V como aplicada sem rodar SQL (destrava "dirty")`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || len(args) > 2 {
		return errors.New(usage)
	}
	dsn, err := config.DatabaseURL()
	if err != nil {
		return err
	}

	number := func(fallback int) (int, error) {
		if len(args) < 2 {
			if fallback < 0 {
				return 0, errors.New(usage)
			}
			return fallback, nil
		}
		n, err := strconv.Atoi(args[1])
		if err != nil {
			return 0, fmt.Errorf("número inválido %q\n%s", args[1], usage)
		}
		return n, nil
	}

	switch args[0] {
	case "up":
		err = migrations.Up(dsn)
	case "down":
		var n int
		if n, err = number(1); err == nil {
			err = migrations.Down(dsn, n)
		}
	case "force":
		var v int
		if v, err = number(-1); err == nil {
			err = migrations.Force(dsn, v)
		}
	case "version":
		// só informa; o resumo abaixo mostra a versão
	default:
		return errors.New(usage)
	}
	if err != nil {
		return err
	}

	version, dirty, err := migrations.Version(dsn)
	if err != nil {
		return err
	}
	fmt.Printf("versão do banco: %d", version)
	if dirty {
		fmt.Print(" (dirty: a última migration falhou no meio; corrija o banco e use make migrate-force version=N)")
	}
	fmt.Println()
	return nil
}
