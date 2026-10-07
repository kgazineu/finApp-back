# FinApp API

API de controle financeiro pessoal, multiusuário, escrita em Go. Ela tem duas metades que se complementam:

| Metade | Pergunta que responde | Módulos |
|---|---|---|
| **Histórico** | "No que eu gastei e quanto ganhei?" | `transactions` (lançamentos), `goals` (metas), `dashboard` |
| **Saldo e projeção** | "Quanto dinheiro eu vou ter no começo do mês que vem (ou daqui a N meses)?" | `accounts`, `billings`, `recurring-transactions`, `receivables` |

Por baixo das duas está a **identidade**: `users` e `sessions`. Todo dado financeiro pertence a um usuário, e toda rota financeira exige um token Bearer.

---

## Sumário

- [Stack e como rodar](#stack-e-como-rodar)
- [Arquitetura](#arquitetura)
- [Convenções gerais](#convenções-gerais)
- [Autenticação](#autenticação)
- [Módulo: Users](#módulo-users)
- [Módulo: Sessions](#módulo-sessions)
- [Módulo: Password resets (recuperação de senha)](#módulo-password-resets-recuperação-de-senha)
- [Módulo: Transactions (lançamentos)](#módulo-transactions-lançamentos)
- [Módulo: Goals (metas virtuais)](#módulo-goals-metas-virtuais)
- [Módulo: Dashboard](#módulo-dashboard)
- [Módulo: Accounts (contas)](#módulo-accounts-contas)
- [Módulo: Billings (registros de saldo e projeção)](#módulo-billings-registros-de-saldo-e-projeção)
- [Módulo: Recurring transactions (entradas e despesas planejadas)](#módulo-recurring-transactions-entradas-e-despesas-planejadas)
- [Módulo: Receivables (valores a receber)](#módulo-receivables-valores-a-receber)
- [Exportar e importar dados (backup)](#exportar-e-importar-dados-backup)
- [Cache (Redis)](#cache-redis)
- [Fluxos](#fluxos)
- [Uso ideal](#uso-ideal)
- [Modelo de dados](#modelo-de-dados)
- [Comportamentos e limitações conhecidas](#comportamentos-e-limitações-conhecidas)

---

## Stack e como rodar

| Peça | Uso |
|---|---|
| Go + Gin | servidor HTTP |
| PostgreSQL 17 (Docker) | banco de dados |
| `oapi-codegen` | gera rotas, tipos e validação de parâmetros a partir de `docs/openapi.yaml` |
| GORM (driver pgx) | persistência de users, sessions, transactions, goals e dashboard |
| `sqlx` (mesmo pool do GORM) | SQL escrito à mão dos módulos de saldo |
| `golang-migrate` | migrations em `migrations/`, embutidas no binário e aplicadas ao subir a API |
| `argon2id` | hash de senha |
| `air` | hot reload em desenvolvimento |

### Passo a passo

1. Crie o `.env` a partir do exemplo e ajuste a senha:
   ```bash
   cp .env.example .env
   ```
2. Suba o banco e o cache (PostgreSQL e Redis no Docker; o compose lê o `.env`):
   ```bash
   make docker-up
   ```
3. Rode a API com hot reload. `make dev` carrega o `.env` e roda o [air](https://github.com/air-verse/air) (instala a versão fixada no Makefile na primeira vez). As migrations são aplicadas sozinhas quando a API inicia, e salvar um `.go` ou `.sql` recompila a API. Sem SMTP configurado, o código de recuperação de senha aparece neste terminal.
   ```bash
   make dev
   ```
4. Se a porta 5432 já estiver ocupada por outro Postgres, troque `POSTGRES_PORT` no `.env` (o banco e a API usam o mesmo valor). Com a 6379 ocupada por outro Redis, troque `REDIS_PORT` e a porta da `REDIS_URL`.
5. Documentação interativa (Swagger) com **todas** as rotas em `http://localhost:8080/docs/`; a especificação está em `/openapi.json`.

`make` sem argumentos lista todos os comandos.

### Migrations

| Comando | O que faz |
|---|---|
| `make migration name=create_budgets` | cria `migrations/0000NN_create_budgets.up.sql` e `.down.sql` com o próximo número |
| `make migrate-up` | aplica as pendentes (a API também faz isso ao iniciar) |
| `make migrate-down` | desfaz a última; `make migrate-down n=3` desfaz as últimas 3 |
| `make migrate-version` | mostra a versão aplicada no banco |
| `make migrate-force version=9` | marca a versão como aplicada sem rodar SQL |

Os comandos usam `cmd/migrate`, que lê o mesmo `.env`/`DATABASE_URL` da API: não precisa instalar o CLI do golang-migrate. Se uma migration falhar no meio, o banco fica **dirty** e nada mais roda: corrija o banco à mão e use `make migrate-force` com a última versão que está correta.

### Variáveis de ambiente

A API valida tudo ao iniciar e, se algo estiver errado, **não sobe** e lista todos os problemas de uma vez. A validação é na inicialização e não no build: a mesma imagem Docker é construída sem segredos (no CI) e recebe as variáveis só no deploy.

| Variável | Regra |
|---|---|
| `DATABASE_URL` | URL PostgreSQL com host e banco; quando definida, ignora as `POSTGRES_*` |
| `POSTGRES_PASSWORD` | obrigatória sem `DATABASE_URL` |
| `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_DB`, `POSTGRES_SSLMODE` | padrões `localhost`, `5432`, `finapp`, `finapp`, `disable`; a porta precisa ser válida |
| `HTTP_ADDR` | `host:porta`, padrão `:8080` |
| `CORS_ALLOWED_ORIGINS` | sites que podem chamar a API pelo navegador, separados por vírgula (ex.: `https://seu-front.trycloudflare.com`); cada item precisa ser só esquema + host |
| `GIN_MODE=release` | marca produção: aí `SMTP_HOST` passa a ser obrigatório (o código de recuperação de senha não pode ir para o log) |
| `SMTP_HOST`, `SMTP_PORT` | servidor de e-mail; porta padrão 587 |
| `SMTP_FROM` | e-mail remetente (sem nome de exibição); sem ele, usa `SMTP_USERNAME` |
| `SMTP_USERNAME`, `SMTP_PASSWORD` | autenticação; usuário exige senha |
| `TZ` | fuso para "hoje", vencimentos e meses da projeção; a imagem Docker usa `America/Sao_Paulo` |
| `REDIS_URL` | cache das respostas, como `redis://[:senha@]host:6379/0`; vazia, a API funciona sem cache. URL inválida impede a API de subir; Redis fora do ar, não (veja [Cache](#cache-redis)) |

### Testes

```bash
make test
```

Os testes de integração com PostgreSQL só rodam com `TEST_DATABASE_URL` apontando para um **banco descartável** (cada teste cria e apaga o próprio schema). O teste do cache precisa também de `TEST_REDIS_URL` (qualquer Redis; as chaves levam ids novos a cada execução). No CI as duas são obrigatórias.

```bash
TEST_DATABASE_URL='postgres://finapp:change-me@localhost:5432/finapp_test?sslmode=disable' go test -race -count=1 ./...
```

Ao mudar `docs/openapi.yaml`, regenere o código com `go generate ./internal/api`. Ele gera `openapi.gen.go` (rotas e tipos; o CI confere se está em dia) e `spec.gen.go` (spec do `/docs`).

---

## Arquitetura

```
cmd/api/main.go            composição: config → migrations → pool → serviços → rotas
docs/openapi.yaml          contrato de todas as rotas (é o que o /docs mostra)
migrations/                SQL versionado (000001..000012)

internal/api/              handlers das rotas OpenAPI, middleware de sessão e código gerado:
                           openapi.gen.go (rotas e tipos) e spec.gen.go (spec completo do /docs)
internal/auth/             login e validação de token
internal/user/             cadastro, listagem e edição de usuário
internal/transaction/      lançamentos (domínio)
internal/goal/             metas virtuais (domínio)
internal/overview/         dashboard (domínio)
internal/postgres/         repositórios GORM dos módulos acima
internal/password/         hash argon2id

internal/account/          contas                          ┐
internal/billing/          registros de saldo e projeção   │ módulos de saldo:
internal/recurring/        entradas/despesas planejadas    │ controller → service → repository (sqlx)
internal/receivable/       valores a receber               ┘
internal/passwordreset/    recuperação de senha por código
internal/dataexport/       exportação e importação dos dados do usuário
internal/cache/            cache das respostas no Redis (middleware dos módulos de saldo)
```

São dois estilos convivendo:

- **Rotas OpenAPI** (`/users`, `/sessions`, `/transactions`, `/goals`, `/dashboard`): o contrato está em `docs/openapi.yaml`, o `oapi-codegen` gera a interface e o registro das rotas, e os handlers em `internal/api` chamam serviços de domínio que não conhecem Gin nem GORM.
- **Módulos de saldo** (`/accounts`, `/billings`, `/recurring-transactions`, `/receivables`): cada módulo é autocontido (`controller.go`, `service.go`, `repository.go`, `routes.go`) e é montado em `cmd/api/main.go` por `registerBalanceModules`, atrás do middleware `api.Server.RequireSession` (e do cache, quando há `REDIS_URL`). A recuperação de senha (`/password-resets`, pública) e o backup (`/export`, `/import`) seguem o mesmo estilo.

As rotas dos dois estilos estão documentadas no `docs/openapi.yaml`. As registradas à mão usam tags que o gerador de rotas ignora (`exclude-tags` em `internal/api/config.yaml`: Accounts, Billings, Recurring transactions, Receivables, Password resets e Data). O spec completo, com essas tags, é gerado à parte (`internal/api/spec.config.yaml` → `spec.gen.go`) e é o que o `/docs` serve. Ao criar uma rota registrada à mão, documente-a no YAML com uma dessas tags (ou acrescente a tag nova à lista); sem isso, o gerador tentaria registrar a rota de novo.

O pool de conexões é um só: `sqlx.NewDb(pool, "pgx")` reaproveita o `*sql.DB` do GORM.

---

## Convenções gerais

### Dinheiro é sempre em centavos

Todo valor monetário é um **inteiro em centavos** (`int64`): R$ 6.505,50 = `650550`. Nunca ponto flutuante. Nas rotas OpenAPI o campo se chama `amountMinor`/`targetMinor`/`savedMinor`; nos módulos de saldo, `amount`. Converter para reais é papel do cliente.

### Datas

| Formato | Exemplo | Onde |
|---|---|---|
| Mês: `YYYY-MM` | `2026-10` | `startMonth`, `endMonth`, filtros `month` |
| Dia: `YYYY-MM-DD` | `2026-10-31` | vencimentos (`dueDate`, `firstDueDate`), `date`, `projectedFor` |
| Data e hora (RFC 3339) | `2026-10-03T19:47:56Z` | `createdAt`, `paidAt`, `occurredAt`, `expiresAt` |

O dashboard trabalha em UTC. Nos módulos de saldo, "hoje" e "agora" são o relógio do servidor.

### Erros

Todo erro volta como `{"message": "texto"}`, sempre em português.

| Status | Quando |
|---|---|
| `400` | JSON inválido, campo obrigatório faltando, regra de negócio violada |
| `401` | token ausente, inválido ou expirado (com cabeçalho `WWW-Authenticate: Bearer`) |
| `404` | recurso não existe **ou pertence a outro usuário** |
| `409` | e-mail já cadastrado |
| `413` / `415` | corpo acima de 64 KiB / `Content-Type` diferente de `application/json` (só rotas OpenAPI) |
| `500` | erro inesperado |

### Titularidade

O dono de cada registro é **sempre** o usuário do token; nenhuma rota aceita `userId` no corpo. Toda consulta filtra pelo usuário, e endereçar por ID um recurso de outra pessoa responde `404` (não `403`), para não revelar que ele existe.

### Apagar vs. arquivar (módulos de saldo)

Registro com histórico financeiro nunca é apagado de verdade:

- **Sem histórico** (nada pago, recebido ou registrado ligado a ele): é **apagado**.
- **Com histórico**: é **arquivado** (`archived_at` preenchido). Some das listagens, da projeção e da geração de parcelas, mas continua no banco.

Quem decide é o banco: as chaves estrangeiras de histórico não têm `ON DELETE CASCADE`, então apagar algo referenciado falha e a API arquiva no lugar.

---

## Autenticação

```
POST /users      { name, email, password }        → cria a conta
POST /sessions   { email, password }               → { token, tokenType: "Bearer", expiresAt }

demais rotas     Authorization: Bearer <token>
```

- O token é opaco: 32 bytes aleatórios em base64url (43 caracteres). O banco guarda **só o SHA-256** dele, na tabela `sessions`.
- A sessão vale **24 horas**. Não há renovação nem logout: quando expira, faça login de novo.
- Credencial errada sempre responde `401 Credenciais inválidas`, sem dizer se o e-mail existe.

Como cada tipo de rota valida o token:

```
                          ┌─ rotas OpenAPI ──────► handler chama s.sessionUser(c) ──► serviço de domínio ──► GORM
requisição ──► Gin ───────┤
                          └─ módulos de saldo ───► middleware RequireSession ──► controller ──► service ──► sqlx
                                                   (guarda o user ID no contexto;
                                                    o controller lê com api.UserID(c))
```

`RequireSession` (em `internal/api/session_middleware.go`) usa exatamente a mesma validação dos handlers OpenAPI: um único cabeçalho `Authorization`, esquema `Bearer`, token válido e não expirado. Se falhar, a requisição para ali com `401`.

---

## Módulo: Users

| Método | Rota | Auth | Descrição |
|---|---|---|---|
| `POST` | `/users` | não | Cadastra um usuário |
| `GET` | `/users?limit=&offset=` | sim | Retorna **só o próprio perfil** (com `offset > 0`, `data` vem vazio) |
| `PATCH` | `/users/{id}` | sim | Edita nome e/ou e-mail do **próprio** usuário; outro ID → `404` |

**Regras**

- Nome não pode ser vazio; e-mail tem que ser um endereço simples válido (sem nome de exibição).
- Senha: 8 a 128 caracteres, com letra maiúscula, minúscula, número e caractere especial. A mensagem de erro lista tudo o que falta.
- E-mail é único (`409` se já existir).
- Senha e hash nunca aparecem em resposta.

```json
{ "name": "Kaian", "email": "kaian@example.com", "password": "Uma-senha-123!" }
```

---

## Módulo: Sessions

`POST /sessions` com `{"email": "...", "password": "..."}` responde:

```json
{ "token": "q1w2...43 caracteres", "tokenType": "Bearer", "expiresAt": "2026-10-06T12:00:00Z" }
```

A resposta tem `Cache-Control: no-store`. Detalhes na seção [Autenticação](#autenticação).

---

## Módulo: Password resets (recuperação de senha)

Recupera o acesso por e-mail com um **código de 6 dígitos** (OTC). Rotas públicas, montadas em `cmd/api/main.go` a partir de `internal/passwordreset`.

| Método | Rota | Corpo | Resposta |
|---|---|---|---|
| `POST` | `/password-resets` | `{ "email" }` | `202` sempre, exista ou não a conta |
| `POST` | `/password-resets/verify` | `{ "email", "code" }` | `200` código válido · `400 código inválido ou expirado` |
| `POST` | `/password-resets/confirm` | `{ "email", "code", "password" }` | `200` senha trocada · `400` código ou senha inválidos |

**Regras**

- Um código ativo por usuário (tabela `password_resets`, só o SHA-256 do código fica no banco).
- Vale **15 minutos** e aceita **5 verificações** (certas ou erradas): verificar e confirmar gastam uma cada.
- Pedir de novo troca o código, mas só depois de **1 minuto** do anterior (anti-spam); antes disso a resposta é a mesma e nada é enviado.
- A senha nova segue a mesma política do cadastro e é validada **antes** de gastar tentativa.
- Confirmar troca o hash, apaga o código e **encerra todas as sessões** do usuário.
- O e-mail sai em segundo plano, então o tempo de resposta não revela se a conta existe.

**Envio**: com `SMTP_HOST` definido (mais `SMTP_PORT`, padrão 587, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM`), usa SMTP. Sem ele, o e-mail **com o código** vai para o log da API: serve para desenvolvimento, nunca para produção.

---

## Módulo: Transactions (lançamentos)

Registro do que **já aconteceu**: cada lançamento é uma receita ou despesa avulsa, com categoria e data. É a base do dashboard.

| Método | Rota | Descrição |
|---|---|---|
| `POST` | `/transactions` | Cria um lançamento |
| `GET` | `/transactions?limit=&offset=` | Lista do mais recente para o mais antigo (`occurredAt`, depois `id`) |

**Regras**

| Campo | Receita (`income`) | Despesa (`expense`) |
|---|---|---|
| `amountMinor` | > 0 | > 0 |
| `description` | 1 a 200 caracteres | 1 a 200 caracteres |
| `category` | 1 a 80 caracteres (texto livre) | 1 a 80 caracteres |
| `necessityLevel` | não pode ser enviado | obrigatório, 1 a 5 |
| `paymentMethod` | não pode ser enviado | obrigatório: `card`, `pix` ou `debit` |
| `installments` | sempre 1 | 1 a 60; mais de 1 só com `card` |
| `occurredAt` | obrigatório | obrigatório |

- O parcelamento aqui é **só informativo**: não gera parcelas nem cobranças.
- Formato legado aceito: `{"amountMinor": 1299, "necessityLevel": 3}` vira uma despesa "Lançamento", categoria "Outros", agora.
- Paginação: `limit` de 1 a 100 (padrão 20), `offset` ≥ 0. Página vazia vem como `"data": []`.

```json
{
  "kind": "expense", "amountMinor": 1299, "necessityLevel": 3,
  "description": "Mercado", "category": "Alimentação",
  "paymentMethod": "pix", "installments": 1, "occurredAt": "2026-09-30T12:00:00Z"
}
```

---

## Módulo: Goals (metas virtuais)

"Envelopes" de dinheiro: você define um alvo e vai separando valores para ele. **Tudo é virtual**: depositar numa meta não move dinheiro de conta nenhuma.

| Método | Rota | Descrição |
|---|---|---|
| `POST` | `/goals` | Cria com `{ name, targetMinor }` e saldo inicial 0 |
| `GET` | `/goals?limit=&offset=` | Lista da mais nova para a mais antiga |
| `POST` | `/goals/{id}/allocations` | `{ "direction": "deposit" \| "withdraw", "amountMinor": 5000 }` |

- `targetMinor` e `amountMinor` > 0; nome não vazio.
- Retirar mais do que está guardado é recusado (`400`). Meta de outro usuário → `404`.
- Passar do alvo é permitido.

---

## Módulo: Dashboard

`GET /dashboard?month=YYYY-MM` (padrão: mês atual em UTC) devolve um resumo calculado a partir de **transactions** e **goals**:

| Campo | Significado |
|---|---|
| `incomeMinor`, `expenseMinor` | receitas e despesas lançadas no mês |
| `netTrackedMinor` | receitas − despesas de **todo o histórico** de lançamentos (não é saldo bancário) |
| `goalsSavedMinor`, `goalsTargetMinor` | soma do guardado e dos alvos de todas as metas |
| `recentTransactions` | até 5 lançamentos do mês, mais recentes primeiro |
| `expensesByCategory` | até 10 categorias de despesa do mês, maior total primeiro |

O dashboard **não** enxerga contas, registros de saldo, transactions planejadas nem receivables. Para saldo real e projeção, use `GET /billings`.

---

## Módulo: Accounts (contas)

Cadastro das contas cujo saldo você acompanha: conta corrente, carteira digital, cartão de crédito.

| Termo | Significado |
|---|---|
| `asset` (ativo) | saldo é dinheiro seu (conta do banco, Mercado Pago). Soma no total. |
| `liability` (passivo) | saldo é dívida (fatura do cartão). Subtrai do total. |

### Regras de negócio

1. Toda conta tem **nome** e **tipo** (`asset` ou `liability`).
2. `hasYield` marca se a conta rende. **Conta `liability` nunca tem rendimento**: a API recusa com `400` (na criação e na edição, olhando o tipo **novo**) e o banco garante com a `CHECK liability_without_yield`.
3. `hasYield` hoje é **só informativo**: não entra em cálculo nenhum.
4. Contas **arquivadas** não aparecem na listagem, não participam de novos registros de saldo e não podem ser editadas.
5. **Apagar** uma conta que já apareceu num registro de saldo **arquiva** em vez de apagar.

### Endpoints

| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/accounts` | Lista as contas ativas, por id |
| `GET` | `/accounts/:id` | Uma conta (inclusive arquivada) |
| `POST` | `/accounts` | Cria |
| `PATCH` | `/accounts/:id` | Edita nome, tipo e rendimento (exige os três campos) |
| `DELETE` | `/accounts/:id` | Apaga ou arquiva: `{"message": "conta apagada"}` ou `{"message": "conta arquivada"}` |

```json
{ "name": "Fatura Nubank", "kind": "liability", "hasYield": false }
```

Resposta:

```json
{ "id": 3, "name": "Fatura Nubank", "kind": "liability", "hasYield": false, "createdAt": "2026-10-02T03:45:00Z" }
```

---

## Módulo: Billings (registros de saldo e projeção)

É o centro da metade de saldo: guarda o histórico do seu patrimônio e calcula a projeção.

### Registro de saldos

1. Um registro é um **retrato completo**: exatamente **um lançamento para cada conta ativa sua**. Nem mais, nem menos, nem conta repetida.
   - Faltou uma conta → `400 lançamentos inválidos: falta o saldo da conta <nome>`
   - Quantidade diferente do número de contas ativas → `400 lançamentos inválidos: era esperado um saldo para cada uma das N contas ativas, mas vieram M`
   - Mesma conta duas vezes → `400`
   - Conta de outro usuário nunca bate com as suas → `400`
2. Cada valor é o **saldo da conta**, em centavos, **sempre ≥ 0**. A fatura do cartão vai positiva: quem diz que ela subtrai é o tipo da conta.
3. **Total** = soma das contas `asset` − soma das contas `liability`.
4. **Delta** = total deste registro − total do seu registro anterior. O **primeiro registro não tem delta** (`null`): não há com o que comparar.
5. O **nome da conta é copiado** para o lançamento (`accountName`): renomear a conta depois não muda o histórico. A resposta traz também `accountKind`, o tipo **atual** da conta, que separa o saldo das contas do valor das faturas (o tipo não é copiado: mudar o tipo de uma conta muda como os registros antigos aparecem, mas não o total gravado).
6. Registro e lançamentos são gravados numa **transação de banco**.
7. Registros não têm edição nem exclusão.

| Registro | Inter (asset) | Mercado Pago (asset) | Fatura (liability) | Total | Delta |
|---|---|---|---|---|---|
| 1º | 0 | 10000 | 5000 | 5000 | — |
| 2º | 0 | 10050 | 5500 | 4550 | −450 |
| 3º | 0 | 16050 | 5500 | 10550 | 6000 |

(É o caso coberto por `internal/billing/service_test.go`.)

### Projeção

`GET /billings?months=N` (N de 1 a 120, padrão 1) calcula quanto você terá no **dia 1 do N-ésimo mês depois do atual**, contando **tudo que vence até o fim desse mês como se fosse pago nesse dia 1**. Assim a projeção para 01/01 já inclui o salário (e as contas) de janeiro:

| Hoje | `months` | `projectedFor` | Considera pendências até | Salários contados (todo mês) |
|---|---|---|---|---|
| 06/10/2026 | 1 | 2026-11-01 | 30/11/2026 | novembro |
| 06/10/2026 | 3 | 2027-01-01 | 31/01/2027 | novembro, dezembro e janeiro |

(O de outubro também entra se ainda estiver pendente, como qualquer atrasado.)

```
projectedAmount =
    total do seu último registro de saldos (0 se não houver)
  + parcelas de recurring transactions "income" não pagas com vencimento até a data limite
  − parcelas de recurring transactions "expense" não pagas com vencimento até a data limite
  + parcelas de receivables não recebidas com vencimento até a data limite
```

- **Parcelas atrasadas entram**: se não foram marcadas como pagas, a API assume que o dinheiro ainda não saiu (ou entrou).
- Antes de calcular, a API **gera as parcelas** de recurring transactions que ainda não existem até a data limite.
- **Crescimento por mês** (`monthlyGrowth`, do mês em `monthlyGrowthMonth`): uma noção de quanto você cresce por mês, olhando o **mês que vem**: entradas + valores a receber − **despesas fixas**. Vem das regras cadastradas, pagas ou não (marcar algo adiantado não muda o número); despesas variáveis (compras, parcelamentos) ficam de fora. Não muda com `months`.
- A projeção parte do **último registro de saldos**. Ao registrar um saldo novo, marque como pagas as parcelas que já saíram da conta; senão elas são descontadas duas vezes.

### Endpoints

| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/billings?months=N` | Seus registros de saldo + projeção |
| `POST` | `/billings` | Cria um registro de saldos |

```json
{ "entries": [ { "accountId": 1, "amount": 0 }, { "accountId": 2, "amount": 10050 }, { "accountId": 3, "amount": 5500 } ] }
```

Resposta do `POST`:

```json
{ "id": 2, "delta": -450, "total": 4550,
  "entries": [ { "id": 4, "accountId": 1, "accountName": "Inter", "accountKind": "asset", "amount": 0 } ],
  "createdAt": "2026-10-03T19:00:00Z" }
```

Resposta do `GET`:

```json
{
  "billingRegistrations": [ /* do mais antigo para o mais novo */ ],
  "projectedFor": "2026-11-01",
  "projectedAmount": 123450,
  "monthlyGrowth": 566600,
  "monthlyGrowthMonth": "2026-11",
  "pendingTransactions": [ /* parcelas de recurring transactions consideradas */ ],
  "pendingReceivables": [ /* parcelas de receivables consideradas */ ]
}
```

---

## Módulo: Recurring transactions (entradas e despesas planejadas)

O que **vai** acontecer: uma recurring transaction é uma **regra de recorrência** — "R$ X, de entrada ou despesa, a cada N meses, a partir do mês M, até o mês F (ou sem fim)". As **parcelas** são as ocorrências concretas dessa regra, cada uma com vencimento e checkbox de pago.

> Não confundir com [`/transactions`](#módulo-transactions-lançamentos), que registra o que já aconteceu. Ver [Uso ideal](#uso-ideal).

| Termo | Significado |
|---|---|
| **Fixa** | sem quantidade definida de vezes (salário, aluguel) |
| **Variável** | com fim: uma vez só ou parcelada (conserto, celular em 10x) |
| **Ocorrência** | a regra "acontecendo" num mês (cálculo, não fica salvo) |
| **Parcela** | ocorrência materializada no banco, com vencimento e `paidAt` |
| **Atrasada** (`overdue`) | parcela não paga cujo vencimento é anterior a hoje (no próprio dia ainda não está) |

### Campos

| Campo | Regra |
|---|---|
| `description` | obrigatório |
| `kind` | `income` ou `expense` |
| `isFixed` | obrigatório (`true` = fixa, `false` = variável) |
| `amount` | centavos, **> 0** (o sinal vem do `kind`) |
| `startMonth` | `YYYY-MM`, primeiro mês em que acontece |
| `intervalMonths` | a cada quantos meses; `0` ou omitido = todo mês; máx. 120 |
| `installments` | quantas vezes acontece (só na criação); máx. 600 |
| `dayOfMonth` | 1 a 31, opcional; sem dia = vence no último dia do mês |

### Regras de negócio

**1. Quando termina (`endMonth`)**, calculado na criação:

| Tipo | `installments` | Resultado |
|---|---|---|
| Variável | 0 (omitido) | **uma vez só** (`endMonth = startMonth`) |
| Fixa | 0 (omitido) | **sem fim** (`endMonth = null`) |
| Qualquer | N > 0 | `endMonth = startMonth + (N − 1) × intervalMonths` |

Celular em 10x a partir de 2026-10 → termina em 2027-07. IPVA fixo 3 vezes a cada 12 meses a partir de 2026-10 → termina em 2028-10.

**2. Meses em que acontece**: o `startMonth` e a cada `intervalMonths` depois, sem passar do `endMonth`.

**3. Vencimento**: no `dayOfMonth`; se o mês não tiver esse dia, no último dia do mês (31 em fevereiro → 28/02). Sem `dayOfMonth`, no último dia do mês.

**4. Numeração "x de N"**: só em variáveis com mais de uma ocorrência (o celular mostra 3/10).

**5. Editar** (`PATCH`) só altera `description`, `amount`, `dayOfMonth` e `endMonth`.
- `endMonth` antes do `startMonth` → `400` (também garantido por `CHECK`).
- **Parcelas pagas não mudam**; **parcelas não pagas são refeitas** com os valores novos, inclusive as atrasadas.
- O `PATCH` **substitui** `dayOfMonth` e `endMonth`: omitir `endMonth` torna a regra sem fim e omitir `dayOfMonth` remove o dia fixo. Mande sempre os valores atuais.

**6. Apagar**: parcelas não pagas são apagadas. Se sobrar alguma paga, a regra é **arquivada**; senão, apagada. Resposta: `{"archived": true|false}`.

### Geração de parcelas

As parcelas **não nascem junto com a regra**: são geradas sob demanda sempre que você consulta as pendentes (`/installments/pending` ou `GET /billings`).

1. Para cada regra ativa sua, calcula as ocorrências desde o **`startMonth`** até a data limite, mesmo que ela tenha sido cadastrada depois.
2. **Começou no passado? As parcelas já vencidas nascem atrasadas**: o seguro em 12x que começou em fevereiro, cadastrado em outubro, aparece com as parcelas de fevereiro a setembro em atraso, e você marca as que já pagou. Nada é dado como pago sem você marcar; as não marcadas entram na projeção.
3. Cada parcela guarda **uma cópia do valor** no momento em que é gerada.
4. Parcela que já existe não é recriada nem alterada.

### Marcar como pago

`PATCH /recurring-transactions/installments/:id` com `{"paid": true}` ou `{"paid": false}` (`paid` é obrigatório).

- `true`: preenche `paidAt` com agora; se já estava paga, mantém a data original.
- `false`: limpa o `paidAt`.

### Endpoints

| Método | Rota | Descrição |
|---|---|---|
| `POST` | `/recurring-transactions` | Cria |
| `GET` | `/recurring-transactions` | Lista as ativas (por dia do mês, sem dia por último) |
| `GET` | `/recurring-transactions/occurrences?month=YYYY-MM` | O que acontece no mês (padrão: mês atual) |
| `PATCH` | `/recurring-transactions/:id` | Edita |
| `DELETE` | `/recurring-transactions/:id` | Apaga ou arquiva |
| `GET` | `/recurring-transactions/installments/pending` | Parcelas não pagas até o fim do mês atual (inclui atrasadas) |
| `GET` | `/recurring-transactions/installments/paid` | Parcelas pagas nos últimos 30 dias |
| `PATCH` | `/recurring-transactions/installments/:id` | Marca/desmarca como paga |

```json
{
  "description": "Celular", "kind": "expense", "isFixed": false, "amount": 15000,
  "startMonth": "2026-11", "intervalMonths": 1, "installments": 10, "dayOfMonth": 10
}
```

Editar:

```json
{ "description": "Celular", "amount": 15500, "dayOfMonth": 10, "endMonth": "2027-08" }
```

Ocorrência (`/occurrences`) — `date` é `null` sem dia definido; `installment`/`installments` só aparecem com numeração:

```json
{ "transactionId": 7, "description": "Celular", "kind": "expense", "isFixed": false,
  "amount": 15000, "date": "2026-11-10", "installment": 1, "installments": 10 }
```

> `/occurrences` é **só um cálculo** a partir das regras: não olha parcelas, não sabe o que foi pago. Serve para "o que acontece em tal mês". Para "o que falta pagar", use `/installments/pending`.

Parcela pendente:

```json
{ "id": 31, "transactionId": 7, "number": 1, "amount": 15000, "dueDate": "2026-11-10",
  "paidAt": null, "overdue": false, "description": "Celular", "kind": "expense", "isFixed": false }
```

---

## Módulo: Receivables (valores a receber)

Dinheiro que alguém te deve. Diferente das recurring transactions, **todas as parcelas são criadas na hora**, porque valor total e número de parcelas são conhecidos desde o início.

| Tipo | Uso | Juros | Parcelas |
|---|---|---|---|
| `split` | conta dividida (pizza, viagem) | sempre 0 | sempre 1 |
| `loan` | empréstimo (relógio, bicicleta) | opcional | 1 a 120 |

`split` com juros ou mais de uma parcela → `400` (o banco também garante que `split` não tem juros).

### Regras de negócio

**1. Valor das parcelas**: na criação, `amountMode` diz o que é o `amount`:

| `amountMode` | O `amount` é | Exemplo |
|---|---|---|
| `total` (padrão) | o valor total, **dividido** entre as parcelas | emprestou R$ 1.000 em 3x |
| `installment` | o valor de **cada** parcela | assinatura de R$ 50 por mês, 12 meses |

Com `installment`, cada parcela vale exatamente o `amount` e **não há juros** (`interestRate` diferente de 0 → `400`; informe a parcela já com os juros). As regras abaixo são do modo `total`:

- `amount` é o valor **sem juros**; `interestRate` é um **percentual inteiro, juros simples sobre o total** (`5` = 5%).
- Total com juros = `amount + amount × interestRate / 100` (divisão inteira).
- Cada parcela = total ÷ parcelas (divisão inteira); **o resto vai para a última**, para a soma bater.

| Caso | Valor | Juros | Parcelas | Resultado |
|---|---|---|---|---|
| Pizza | 25 | 0% | 1 | 25 |
| Bicicleta | 1000 | 0% | 2 | 500, 500 |
| Bicicleta com juros | 1000 | 5% | 2 | 525, 525 |
| Divisão com resto | 1000 | 0% | 3 | 333, 333, 334 |

**2. Vencimentos**: a 1ª parcela vence em `firstDueDate`; as seguintes, mês a mês, sempre calculadas **a partir da primeira data** e sem transbordar: 31/10 → 30/11 → 31/12.

`firstDueDate` **pode ser no passado** (empréstimo que começou em agosto, cadastrado em outubro): as parcelas já vencidas nascem atrasadas e a própria pessoa marca as que já foram recebidas. As que não forem marcadas continuam pendentes e entram na projeção.

**3. Criação atômica**: receivable e parcelas numa transação de banco.

**4. Editar**: `PATCH /receivables/:id` altera `debtor` e `description`. Valor e vencimento são editados **por parcela** (`PATCH /receivables/installments/:id`):

```json
{ "amount": 6000, "dueDate": "2026-10-20", "applyToFollowing": true }
```

- Campos opcionais, em qualquer combinação, junto ou não com `paid`. Sem nenhum → `400`.
- `applyToFollowing: true` leva o novo valor e o novo vencimento (mês a mês a partir da nova data) para as **parcelas seguintes ainda não recebidas** — o caso do aumento de uma assinatura.
- Parcelas recebidas nunca mudam em lote; a parcela editada muda mesmo se já recebida (correção explícita).
- Tudo numa transação de banco; parcela de outro usuário → `404`.

**5. Apagar**: parcelas não recebidas são apagadas; sobrando alguma recebida, o receivable é **arquivado**. Resposta: `{"archived": true|false}`.

**6. Marcar como recebido** e **atrasada**: mesmas regras das parcelas de recurring transactions.

### Endpoints

| Método | Rota | Descrição |
|---|---|---|
| `POST` | `/receivables` | Cria com as parcelas |
| `GET` | `/receivables` | Lista os ativos com as parcelas (mais novo primeiro) |
| `PATCH` | `/receivables/:id` | Edita devedor e descrição |
| `DELETE` | `/receivables/:id` | Apaga ou arquiva |
| `GET` | `/receivables/installments/pending` | **Todas** as parcelas não recebidas (sem limite de data) |
| `GET` | `/receivables/installments/paid` | Parcelas recebidas nos últimos 30 dias |
| `PATCH` | `/receivables/installments/:id` | Marca/desmarca como recebida (`{"paid": true}`) e/ou edita valor e vencimento |

```json
{
  "kind": "loan", "debtor": "João", "description": "Bicicleta",
  "amount": 100000, "amountMode": "total", "interestRate": 5, "installments": 2, "firstDueDate": "2026-10-31"
}
```

`kind`, `debtor`, `description`, `amount` (> 0) e `firstDueDate` são obrigatórios; `installments` omitido ou `0` = 1 parcela.

```json
{
  "id": 4, "kind": "loan", "debtor": "João", "description": "Bicicleta",
  "amount": 100000, "amountMode": "total", "interestRate": 5,
  "installments": [
    { "id": 9, "number": 1, "amount": 52500, "dueDate": "2026-10-31", "paidAt": null, "overdue": false },
    { "id": 10, "number": 2, "amount": 52500, "dueDate": "2026-11-30", "paidAt": null, "overdue": false }
  ],
  "createdAt": "2026-10-03T19:24:00Z"
}
```

---

## Exportar e importar dados (backup)

Para levar os dados de um usuário para outro servidor/banco sem cadastrar tudo de novo. Ambas as rotas exigem sessão.

| Método | Rota | O que faz |
|---|---|---|
| `GET` | `/export` | devolve um JSON (`format: "finapp-export"`, `version: 1`) com contas (inclusive arquivadas), registros de saldo com lançamentos, transações planejadas e valores a receber com as parcelas (pagas ou não), lançamentos e metas |
| `POST` | `/import` | recebe esse JSON e recria tudo na conta da sessão; `?replace=true` troca os dados que já existem |

- Conta **sem dados** importa direto.
- Conta **com dados** (qualquer conta, registro, transação, valor a receber, lançamento ou meta) recebe `409`, a menos que venha `?replace=true`: aí a API **apaga todos os seus dados** e grava o arquivo no lugar. Nada é misturado nem duplicado. O web pede confirmação antes de mandar o `replace`.
- É tudo ou nada: uma transação de banco, que inclui a limpeza do `replace`. Arquivo de outro formato ou com dados que violam as regras do banco → `400`, e os dados antigos continuam como estavam; acima de 20 MB → `413`.
- O `GET /export` responde `Cache-Control: no-store`: o arquivo nunca fica em cache.
- Os ids não viajam: cada registro ganha id novo e as ligações (conta do lançamento, pai da parcela) são refeitas. Datas de criação, arquivamento e pagamento são mantidas, então a geração de parcelas e a projeção continuam iguais.
- No web, fica em **Perfil → Seus dados**. Para migrar o banco inteiro (todos os usuários) de uma vez, `pg_dump`/`pg_restore` continua sendo o caminho.

---

## Cache (Redis)

Com `REDIS_URL` definida, as respostas `GET` dos módulos de saldo (`/accounts`, `/billings`, `/recurring-transactions`, `/receivables` e as rotas de parcelas) ficam no Redis, separadas por usuário, por até 10 minutos.

- **Escrita invalida na hora.** Qualquer `POST`, `PATCH` ou `DELETE` do usuário nesses módulos (inclusive `/import`) incrementa a versão do cache dele, que faz parte da chave: o que estava guardado deixa de ser usado e expira sozinho. Invalida mesmo quando a escrita falha, porque uma edição que quebra no meio pode já ter mudado dados.
- **O dia faz parte da chave**: "atrasada" e os meses da projeção mudam à meia-noite (no fuso `TZ`).
- **Cada usuário tem o seu**: a chave leva o id do usuário da sessão.
- A resposta traz `X-Cache: HIT` (veio do Redis) ou `MISS` (foi ao banco e ficou guardada).
- **Não é guardado**: resposta de erro e o `GET /export`.
- Ao iniciar, a API apaga as chaves `finapp:*`, porque o formato das respostas pode ter mudado entre versões. Só essas chaves: dá para usar um Redis compartilhado.
- **Redis fora do ar não derruba a API**: cada operação faz uma tentativa curta (300 ms, sem repetir) e, se falhar, a requisição vai direto ao banco, com um aviso no log.
- Mudança feita direto no banco, sem passar pela API, aparece em até 10 minutos ou na próxima escrita do usuário.

Em produção, o `compose.prod.yaml` sobe um Redis só em memória (sem persistência), na rede interna, e a API usa `redis://redis:6379/0`. Localmente, `make docker-up` sobe um igual na porta 6379.

---

## Fluxos

### 1. Primeiro acesso

```
POST /users                      cria a conta
POST /sessions                   pega o token (guarde e mande em toda requisição)
```

### 2. Cadastro inicial do planejamento (uma vez)

```
POST /accounts                   Inter (asset), Mercado Pago (asset), Fatura (liability)
POST /recurring-transactions     salário (fixa, income, dia 5)
                                 aluguel (fixa, expense, dia 10)
                                 celular (variável, expense, 10x)
POST /billings                   primeiro retrato: um saldo por conta ativa
```

### 3. Rotina de atualização de saldo

```
1. GET   /recurring-transactions/installments/pending   o que está pendente (e atrasado)
2. PATCH /recurring-transactions/installments/:id        {"paid": true} no que já saiu/entrou da conta
   PATCH /receivables/installments/:id                   {"paid": true} no que já foi recebido
3. POST  /billings                                       anota o saldo atual de cada conta
4. GET   /billings?months=1                              quanto terei no dia 1º do mês que vem
```

A **ordem importa**: o registro de saldos já reflete o que saiu ou entrou nas contas. Uma parcela que já foi debitada mas continua pendente é descontada de novo na projeção.

### 4. Emprestou dinheiro ou dividiu uma conta

```
POST /receivables                as parcelas já nascem prontas e entram na projeção
```

### 5. Registro do dia a dia e metas

```
POST /transactions               cada gasto/receita real, com categoria
POST /goals  +  /allocations     separar dinheiro (virtualmente) para objetivos
GET  /dashboard?month=2026-10    para onde foi o dinheiro no mês
```

---

## Uso ideal

As duas metades **não se conversam**: o dashboard não lê contas nem planejamento, e a projeção não lê lançamentos nem metas. Cada uma responde uma pergunta:

| Quero... | Use |
|---|---|
| saber quanto vou ter no mês que vem / daqui a N meses | `GET /billings?months=N` |
| acompanhar a evolução do meu patrimônio (total e delta) | `POST /billings` periódico + `GET /billings` |
| não esquecer contas fixas, parcelas e o que me devem | `recurring-transactions` + `receivables` (pendentes e atrasadas) |
| entender **em que** gastei (categorias, necessidade, forma de pagamento) | `POST /transactions` + `GET /dashboard` |
| separar dinheiro para um objetivo | `goals` |

Recomendações:

- **Comece pela metade de saldo** se o objetivo é previsibilidade: cadastre as contas, as regras fixas e tire o primeiro retrato. Ela funciona bem com atualizações semanais ou quinzenais.
- **Registre saldos com frequência** e sempre depois de marcar o que foi pago: quanto mais recente o último registro, mais confiável a projeção.
- Use **recurring transactions** para o que se repete ou já está comprometido (salário, aluguel, assinaturas, compras parceladas). Use **transactions** para o histórico do que de fato aconteceu, se quiser análise por categoria. Uma compra parcelada pode existir nas duas, com papéis diferentes: planejamento lá, histórico aqui.
- Não apague contas, regras ou receivables para "limpar" a tela: com histórico eles são arquivados de qualquer forma; sem histórico, somem.
- Metas são envelopes virtuais: não reduzem o saldo das contas nem a projeção.

---

## Modelo de dados

```
users ─────────────────────────────────────────────────────────────────────────┐
  id (uuid), name, email (único), password_hash                                │ user_id em todas
                                                                               │ as tabelas abaixo
sessions          token_hash (sha256), user_id, expires_at                     │
transactions      id (uuid), user_id, kind, amount_minor, description,         │
                  category, payment_method, installments, necessity_level,     │
                  occurred_at                                                  │
goals             id (uuid), user_id, name, target_minor, saved_minor  ◄───────┤
                                                                               │
accounts ──────────────────┐ (sem cascade)                             ◄───────┤
  id, user_id, name, kind, │                                                   │
  has_yield, archived_at   ▼                                                   │
billing_registrations ◄── billing_entries                              ◄───────┤
  id, user_id, delta,       id, billing_registration_id (cascade),             │
  total                     account_id, account_name (cópia), amount           │
                            UNIQUE (billing_registration_id, account_id)       │
                                                                               │
recurring_transactions ◄── recurring_transaction_installments          ◄───────┤
  id, user_id, description,   id, transaction_id (sem cascade),                │
  kind, is_fixed, amount,     number, amount (cópia), due_date, paid_at        │
  start_month,                UNIQUE (transaction_id, number)                  │
  interval_months, end_month,                                                  │
  day_of_month, archived_at                                                    │
                                                                               │
receivables ◄───────────── receivable_installments                     ◄───────┘
  id, user_id, kind, debtor,  id, receivable_id (sem cascade),
  description, amount,        number, amount (com juros), due_date, paid_at
  amount_mode, interest_rate, UNIQUE (receivable_id, number)
  archived_at
```

Parcelas e lançamentos de saldo não têm `user_id`: pertencem ao usuário dono do registro pai, e toda consulta a eles faz o `join` para checar isso.

Regras garantidas pelo banco:

| Regra | Onde |
|---|---|
| E-mail único | `users_email_unique` |
| Passivo não tem rendimento | `accounts`: `CHECK (kind = 'asset' OR has_yield = false)` |
| Conta dividida não tem juros | `receivables`: `CHECK (kind = 'loan' OR interest_rate = 0)` |
| Intervalo > 0 e dia do mês entre 1 e 31 | `recurring_transactions` |
| Fim não antes do início | `recurring_transactions`: `CHECK (end_month IS NULL OR end_month >= start_month)` |
| Uma parcela por número | `UNIQUE` nas duas tabelas de parcelas |
| Um lançamento por conta por registro | `UNIQUE` em `billing_entries` |
| Histórico não some | FKs de parcelas e lançamentos de saldo sem `ON DELETE CASCADE` |
| Regras de lançamento (tipo, valor, parcelas, forma de pagamento, necessidade) | `CHECK`s em `transactions` |
| Alvo > 0 e guardado ≥ 0 | `CHECK`s em `goals` |

`paid_at` nulo = não pago/recebido. `archived_at` nulo = ativo.

Migrations: `000001`–`000005` (users, transactions, sessions, goals), `000006`–`000009` (accounts, billings, recurring transactions, receivables), `000010` (password resets), `000011` (modo do valor dos receivables) e `000012` (primeiro registro de saldo sem delta).

---

## Comportamentos e limitações conhecidas

**Geral**

- Não há logout no servidor (o cliente descarta o token), renovação de sessão nem limpeza de sessões expiradas ou de códigos de recuperação vencidos.
- Os módulos de saldo e a recuperação de senha usam o binding do Gin: não exigem `Content-Type`, não limitam o tamanho do corpo e ignoram campos desconhecidos (as rotas OpenAPI fazem os três). Erros de validação voltam traduzidos (`api.BindError`); erros `500` respondem uma mensagem genérica em português e a causa vai para o log (`api.InternalError`).
- As rotas não têm barra no final. `/accounts/` redireciona para `/accounts` (301 em `GET`, 307 nos demais).
- `transactions` e `recurring-transactions` são independentes: marcar uma parcela como paga não cria lançamento, e lançar um gasto não baixa parcela.

**Billings**

- O delta lê o último registro **fora** da transação de gravação: dois registros do mesmo usuário criados ao mesmo tempo podem calcular o delta contra o mesmo anterior.

**Recurring transactions**

- Editar e apagar fazem duas operações separadas (apagar parcelas não pagas, depois atualizar), sem transação de banco. Se a segunda falhar, as parcelas são regeradas na próxima consulta de pendentes.
