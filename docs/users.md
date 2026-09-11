# Consulta de usuários

`GET /users` lista usuários com paginação. O contrato completo está em
`docs/openapi.yaml` e é publicado pela API em `/docs/`.

```bash
curl --fail-with-body 'http://localhost:8080/users?limit=20&offset=0'
```

| Parâmetro | Padrão | Valores aceitos |
| --- | --- | --- |
| `limit` | `20` | Inteiro de 1 a 100 |
| `offset` | `0` | Inteiro maior ou igual a zero |

Para a próxima página, incremente `offset` pelo `limit`. A ordenação é crescente
por `createdAt`, com desempate por UUID. Inserções e exclusões entre requisições
podem mudar a posição dos registros na paginação por offset.

Uma página sem usuários retorna `200`, inclusive quando o offset ultrapassa
o último registro:

```json
{"data": [], "limit": 20, "offset": 0}
```

Cada item de `data` contém apenas `id`, `name`, `email`, `createdAt` e `updatedAt`.
As datas da listagem são retornadas em UTC. A consulta não carrega hashes de senha.
Parâmetros vazios, repetidos, não inteiros ou fora dos limites retornam `400`.
Falhas internas retornam `500`, com uma mensagem genérica em `message`.

## Fluxo no código

1. `internal/api/user_list.go` recebe os parâmetros HTTP e monta a resposta pública.
2. `internal/user/list_service.go` valida a paginação e chama o repositório.
3. `internal/postgres/user_list.go` consulta o PostgreSQL usando GORM.

O modelo em `internal/user/user.go` permanece separado do caso de uso. O GET
utiliza a tabela existente e não exige uma nova migration.

## Testes

```bash
go test ./...
```

Para incluir a integração com PostgreSQL, defina `TEST_DATABASE_URL` apontando
para um banco de testes antes de executar `go test -race -count=1 ./...`.
Os testes criam schemas isolados e os removem ao terminar. Sem essa variável,
os testes de integração são pulados localmente; no CI, ela é obrigatória.
