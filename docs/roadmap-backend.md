# FinControl — plano de conclusão do backend

**Estado de referência:** repositório `finApp-back`, inspecionado em 29/09/2026. Este documento é um plano de execução, não uma declaração de que as funcionalidades futuras já estão implementadas. Revisar prioridades e decisões à medida que os contratos de produto forem fechados.

## 1. Diretriz técnica definitiva

**Toda a implementação do backend será em Go**, evoluindo a aplicação existente. A stack é Go (versão declarada em `go.mod`), Gin para HTTP, GORM com PostgreSQL para persistência, migrations SQL e testes com `testing` e doubles próprios. Domínio e casos de uso permanecem independentes de Gin e GORM; interfaces definidas junto às funcionalidades separam regras de negócio da infraestrutura.

As etapas deste documento seguem essa decisão: novos serviços, repositórios, handlers, testes e composição de dependências devem usar os padrões Go já adotados no repositório. Mudanças de stack não fazem parte do roadmap.

O frontend React/Vite/TypeScript e seu design system são contexto de integração, **não fazem parte do escopo deste repositório**. Os contratos implementados estão em `openapi.yaml`; não presumir rotas além das ali publicadas.

## 2. Baseline verificado

| Área | Implementado | Lacuna para o produto |
| --- | --- | --- |
| API | Gin; `/health`, rotas de usuários, `POST /sessions`, `POST /transactions` e `GET /transactions` definidos em `openapi.yaml`; handlers gerados via `go generate ./internal/api`; `/docs/` e `/openapi.json`. | `GET /users` e `PATCH /users/{id}` ainda não exigem autorização; faltam filtros avançados de transações, categorias e Goals. |
| Domínio/aplicação | `internal/user`, `internal/auth` e `internal/transaction` contêm contratos e serviços; transações validam titular, valor positivo e necessidade de 1 a 5. | Sem regras de orçamento, transferência, categorização e previsibilidade. |
| Dados | `internal/postgres`, GORM e migrations `000001` a `000003` para usuários, transações e sessões; `cmd/api/main.go` compõe serviços e aplica migrations no início. | Falta política de retenção/limpeza das sessões expiradas e tabelas para categorias e Goals. |
| Qualidade/entrega | Testes Go, integração PostgreSQL via `TEST_DATABASE_URL`, CI com formatação, vet, OpenAPI, race, build, imagem e deploy homelab. | Novos testes de integração precisam rodar no CI antes da aprovação; AWS é meta, não implantação existente. |

Referências: `../go.mod`, `../cmd/api/main.go`, `../internal/user/`, `../internal/postgres/`, `../migrations/`, `openapi.yaml` e `../.github/workflows/backend.yml`.

## 3. Decisões de produto/contrato antes do código

Registrar as respostas no contrato OpenAPI e nos testes de aceitação antes da implementação:

1. **Identidade e acesso:** login por e-mail/senha com sessão opaca Bearer de 24 horas e hash do token no banco já implementado. Pendente definir logout/renovação, recuperação de senha, proteção contra força bruta e política de exposição de `GET /users`/`PATCH /users/{id}`. Atualmente essas rotas não têm autorização; restringi-las antes de produção com dados reais.
2. **Transação:** despesa apenas ou também receita/transferência? `NecessityLevel` obrigatório para todas ou só despesas? categoria obrigatória? data da operação vs. criação; moeda, fuso e edição/exclusão. Definir formato de valor (decimal exato/unidades inteiras menores, nunca `float` para dinheiro), arredondamento e moeda suportada.
3. **Categorias inteligentes:** catálogo fixo ou categorias por usuário? “Inteligente” significa regras, sugestão automática ou classificação por modelo? Definir consentimento, possibilidade de correção e métricas antes de introduzir IA ou serviços externos.
4. **Goals/orçamentos:** valor-alvo, limite por categoria ou ambos? periodicidade, tratamento de estornos, cálculo de progresso e comportamento ao ultrapassar o limite.
5. **Previsibilidade:** quais indicadores e horizontes serão entregues no MVP, e com que regras de cálculo e atualização. Não prometer previsões sem dados/regras definidos.

**Contrato preliminar, sujeito às decisões acima:** `POST /transactions` deve resultar em `201` na criação, `400` para entrada inválida e `401` sem identidade válida; `403`/`404` conforme a política de recurso alheio e `500` para falha interna sem detalhes sensíveis. `409` apenas para conflito de negócio definido. Nunca confiar em `userId` enviado no corpo para atribuir titularidade.

## 4. Sequência de execução (Go)

### P0 — Fechar arquitetura, autenticação e contrato mínimo

- [ ] Fechar as decisões de produto da seção 3; versionar exemplos de request/response, erros e critérios de autorização em `docs/openapi.yaml`.
- [x] Implementar login por senha, token opaco aleatório de 24 horas, armazenamento somente do hash, identidade via Bearer e erros genéricos de credenciais, com testes.
- [ ] Definir logout/revogação, limpeza de sessões expiradas, rate limiting de login e política de renovação/recuperação de senha.
- [ ] Proteger as rotas de usuários existentes conforme a política definida; planejar compatibilidade dos consumidores antes de alterar seu comportamento. Nenhum endpoint financeiro multiusuário deve ser publicado sem identificação/autorização.

**Aceite pendente de P0:** revisar contrato com o frontend, impedir exposição de dados pelas rotas de usuários existentes e cobrir revogação quando o logout for implementado. Login e criação de transações já têm testes para credenciais inválidas, sessão expirada e titularidade.

### P1 — Domínio e caso de uso de criação de transação (TDD)

- [x] Criar `internal/transaction` com entidade/construtor, erros de domínio, contrato de repositório e serviço de criação; dependências externas entram por interfaces.
- [x] Testar valor positivo, `NecessityLevel` inteiro de 1 a 5 e titular válido; entrada inválida não chega à persistência.
- [x] Testar serviço com repositório fake, contexto, retorno e erros.
- [ ] Fechar categoria, data da operação e moeda antes de ampliar o domínio.

**Aceite:** testes unitários de casos válidos, bordas (0, negativo, 1, 5, fora de 1..5 e valores não inteiros no limite HTTP) e falha do repositório; nenhum pacote do domínio importa Gin ou GORM.

### P2 — Persistência e composição

- [x] Criar migrations `000002` para transações e `000003` para sessões, com FK, constraints, índices e arquivos de reversão.
- [x] Criar repositórios PostgreSQL de transações e sessões; dinheiro em unidades menores (`BIGINT`/`int64`) e invariantes essenciais também no banco.
- [x] Compor repositórios e serviços em `cmd/api/main.go` com injeção explícita.
- [x] Adicionar listagem paginada de transações com filtro obrigatório por titular e ordenação estável.
- [ ] Planejar rollback com dados reais.

**Aceite:** testes de integração com PostgreSQL provam criação, persistência exata, constraints e isolamento entre dois usuários; migrations aplicam em banco vazio e a composição inicia sem regressões.

### P3 — HTTP e contrato de transações

- [x] Especificar `POST /sessions`, `POST /transactions` e `GET /transactions` em `docs/openapi.yaml` e regenerar `internal/api/openapi.gen.go`. **Não editar o arquivo gerado manualmente.**
- [x] Implementar handlers Gin com autenticação obrigatória para criar transações, validação de corpo e Content-Type, limite de tamanho, timeout, erros HTTP padronizados e `201` só após persistência.
- [x] Cobrir handlers com testes HTTP e teste de ponta a ponta com PostgreSQL para login, criação, expiração e segregação por titular.
- [ ] Validar o fluxo com o frontend e confirmar a execução dos testes de integração no CI.

**Aceite:** contrato gerado corresponde ao YAML, testes demonstram `201`/`400`/`401`/`500` e isolamento; `go test ./...` passa com banco de integração configurado.

**Contrato atual de integração:** `POST /sessions` recebe `{"email":"pessoa@example.com","password":"senha"}` e retorna `token`, `tokenType: "Bearer"` e `expiresAt`. Enviar `Authorization: Bearer <token>` em `POST /transactions` com `{"amountMinor":1299,"necessityLevel":3}`. A resposta `201` contém `id`, `amountMinor`, `necessityLevel` e `createdAt`. `GET /transactions?limit=20&offset=0` retorna `data`, `limit` e `offset` para o mesmo titular, em ordem decrescente por data e ID; páginas vazias trazem `data: []`. O titular é determinado pelo token, nunca pelo JSON ou query string.

### P4 — Completar o MVP financeiro

- [x] Acrescentar listagem paginada por titular, com ordem estável e página vazia explícita.
- [ ] Acrescentar filtros por período/categoria, consulta por ID e edição/exclusão se aprovadas; cada operação sempre restrita ao titular.
- [ ] Implementar categorias e Goals com regras de período, limites e cálculo de progresso aprovadas; migrations, contratos OpenAPI, testes de domínio/integração/HTTP para cada feature.
- [ ] Implementar resumos e previsões somente depois de fechar fórmula, entradas mínimas e comportamento para histórico insuficiente. Para categorização inteligente, começar pela estratégia acordada e testar como corrigir sugestões incorretas.

**Aceite:** usuário consegue registrar, consultar e organizar gastos, acompanhar o orçamento e obter indicadores definidos, sem acesso cruzado nem inconsistências de cálculo.

### P5 — Integração, segurança e operação

- [ ] Entregar ao frontend o OpenAPI atualizado e exemplos de login, criação, listagem e erros; validar fluxo ponta a ponta do login e do modal “Add Transaction” com ambiente de teste. Configurar CORS por origem conhecida e política de credenciais conforme a autenticação escolhida; não alterar componentes de UI neste repositório.
- [ ] Verificar proteção de segredos, limites de requisição, logs sem dados financeiros/senhas, migração e recuperação de dados, métricas/healthchecks e índices de consultas frequentes. Definir estratégia de backup/restore e testar restauração.
- [ ] Usar CI existente como gate (formatação, vet, OpenAPI gerado, testes com `-race`, build); estabelecer meta de cobertura **para regras críticas** e medir com `go tool cover`, sem tratar percentual global isolado como garantia. Documentar infraestrutura, TLS, banco gerenciado e migração de deploy caso AWS seja aprovada; o deploy atual é homelab.

**Aceite de conclusão:** contrato consumido pelo frontend; fluxos principais, autorização e cenários de erro automatizados; migrations e backup/restore testados; CI verde; checklist de segurança e deploy do ambiente escolhido aprovado.

## 5. Comandos de verificação

No diretório raiz do repositório, com Go da versão especificada em `go.mod`:

```sh
go generate ./internal/api
gofmt -l .
go vet ./...
go test ./...
go build ./cmd/api
```

Para executar **também** testes de integração (sem `TEST_DATABASE_URL`, são ignorados localmente), disponibilizar PostgreSQL e configurar `TEST_DATABASE_URL` para um **banco de testes descartável**, então executar `go test -race -count=1 -coverprofile=coverage.out ./...`. O CI já usa essa variável e falha se ela estiver ausente. Conferir o diff do código gerado após `go generate`.

## 6. Riscos e dependências

- **Consistência técnica:** seguir a stack Go estabelecida e evitar serviços paralelos ou adaptações que dupliquem as responsabilidades do backend existente.
- **Privacidade:** `POST /transactions` exige sessão válida, mas `GET /users` e `PATCH /users/{id}` ainda não têm autorização. Restringir essas rotas e definir compatibilidade antes de produção.
- **Contratos em aberto:** sem acordar dinheiro, moeda, receitas, categorias e Goals, schema e endpoints podem exigir migrations quebrando clientes.
- **Infraestrutura:** o CI publica imagem e faz deploy no homelab; “cloud-ready/AWS” requer arquitetura e validação próprias, não deve ser declarado concluído pelo Docker atual.
