// Package cache guarda no Redis as respostas GET das rotas de saldo (contas, registros,
// transações planejadas, valores a receber), separadas por usuário.
//
// Invalidação: cada usuário tem um número de versão que entra na chave. Qualquer escrita dele
// (POST, PATCH, DELETE) incrementa a versão depois de gravar no banco; o que estava guardado
// deixa de ser encontrado e expira sozinho. Não há varredura de chaves nem resposta velha
// depois de uma alteração feita pela API.
//
// Redis fora do ar não derruba nada: a requisição segue direto para o banco.
package cache

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/redis/go-redis/v9"
)

const prefix = "finapp:"

// ponytail: TTL fixo. A invalidação por escrita já garante dados novos; o TTL só limita a
// memória e quanto demora para aparecer uma mudança feita direto no banco.
const ttl = 10 * time.Minute

type Cache struct {
	rdb *redis.Client
}

// New só valida a URL: a conexão abre sob demanda e volta sozinha se o Redis cair.
func New(url string) (*Cache, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, errors.New("REDIS_URL inválida: use redis://[:senha@]host:6379/0")
	}
	// Redis lento ou fora do ar não pode segurar a API: uma tentativa curta, sem repetir, e vai ao banco
	opt.DialTimeout = 300 * time.Millisecond
	opt.ReadTimeout = 300 * time.Millisecond
	opt.WriteTimeout = 300 * time.Millisecond
	opt.DialerRetries = 1
	opt.MaxRetries = -1
	return &Cache{rdb: redis.NewClient(opt)}, nil
}

func (c *Cache) Close() error {
	return c.rdb.Close()
}

// Clear apaga o que a versão anterior da API guardou (o formato das respostas pode ter mudado).
// Só as chaves finapp:*, então dá para dividir o Redis com outros projetos.
func (c *Cache) Clear(ctx context.Context) error {
	iter := c.rdb.Scan(ctx, 0, prefix+"*", 1000).Iterator()
	for iter.Next(ctx) {
		if err := c.rdb.Del(ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}
	return iter.Err()
}

// Middleware vem depois de api.Server.RequireSession, que identifica o usuário.
func (c *Cache) Middleware(ctx *gin.Context) {
	user := api.UserID(ctx).String()
	versionKey := prefix + "version:" + user

	if ctx.Request.Method != http.MethodGet {
		ctx.Next()
		// mesmo se a escrita falhou: uma edição que quebra no meio pode já ter mudado dados
		if err := c.rdb.Incr(context.WithoutCancel(ctx.Request.Context()), versionKey).Err(); err != nil {
			slog.Warn("cache: não foi possível invalidar", "error", err)
		}
		return
	}

	reqCtx := ctx.Request.Context()
	version, err := c.rdb.Get(reqCtx, versionKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		slog.Warn("cache indisponível", "error", err)
		return // segue para o banco
	}

	// o dia entra na chave: "atrasada" e os meses da projeção mudam com a data
	key := prefix + strings.Join([]string{"response", user, version, time.Now().Format(time.DateOnly), ctx.Request.URL.RequestURI()}, ":")
	body, err := c.rdb.Get(reqCtx, key).Bytes()
	if err == nil {
		ctx.Header("X-Cache", "HIT")
		ctx.Data(http.StatusOK, "application/json; charset=utf-8", body)
		ctx.Abort()
		return
	}
	if !errors.Is(err, redis.Nil) {
		slog.Warn("cache indisponível", "error", err)
		return
	}

	recorder := &bodyRecorder{ResponseWriter: ctx.Writer}
	ctx.Writer = recorder
	ctx.Header("X-Cache", "MISS")
	ctx.Next()

	// só sucesso; e quem pede no-store (o arquivo de exportação) nunca é guardado
	if recorder.Status() != http.StatusOK || strings.Contains(recorder.Header().Get("Cache-Control"), "no-store") {
		return
	}
	if err := c.rdb.Set(reqCtx, key, recorder.body.Bytes(), ttl).Err(); err != nil {
		slog.Warn("cache: não foi possível guardar", "error", err)
	}
}

// bodyRecorder copia o corpo da resposta enquanto ele vai para o cliente.
type bodyRecorder struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (r *bodyRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r *bodyRecorder) WriteString(s string) (int, error) {
	r.body.WriteString(s)
	return r.ResponseWriter.WriteString(s)
}
