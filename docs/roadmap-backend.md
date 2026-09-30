# FinControl — plano de conclusão do backend

**Estado de referência:** repositório `finApp-back`, inspecionado em 30/09/2026. O dashboard mensal já tem rota HTTP, contrato OpenAPI gerado, handler, composição e testes HTTP unitários; sua integração com PostgreSQL ainda não foi executada localmente sem `TEST_DATABASE_URL`. Revisar prioridades e decisões à medida que os contratos de produto forem fechados.

## 1. Diretriz técnica definitiva

**Toda a implementação do backend será em Go**, evoluindo a aplicação existente. A stack é Go (versão declarada em `go.mod`), Gin para HTTP, GORM com PostgreSQL para persistência, migrations SQL e testes com `testing` e doubles próprios. Domínio e casos de uso permanecem independentes de Gin e GORM; interfaces definidas junto às funcionalidades separam regras de negócio da infraestrutura.

As etapas deste documento seguem essa decisão: novos serviços, repositórios, handlers, testes e composição de dependências devem usar os padrões Go já adotados no repositório. Mudanças de stack não fazem parte do roadmap.

O frontend React/Vite/TypeScript e seu design system são contexto de integração, **fora deste workspace**. `openapi.yaml` é o contrato-fonte; `GET /dashboard` está presente no código gerado e registrado no servidor HTTP.

## 2. Baseline verificado

| Área | Implementado | Lacuna para o produto |
| --- | --- | --- |
| API | Gin; `GET /health`, `POST /users`, `GET /users` restrito ao próprio perfil, `PATCH /users/{id}` apenas para o titular, `POST /sessions`, `GET/POST /transactions`, `GET/POST /goals`, `POST /goals/{id}/allocations` e `GET /dashboard`. Rotas registradas pelo gerador OpenAPI; `/docs/` e `/openapi.json`. | Faltam filtros avançados e edição/exclusão de transações. |
| Domínio/aplicação | `internal/user`, `internal/auth`, `internal/transaction` e `internal/goal`: sessão Bearer, receitas/despesas detalhadas e metas de saldo virtual (sem transferência). `internal/overview` e o repositório em `internal/postgres` alimentam o dashboard mensal autenticado, composto em `cmd/api/main.go`. | Orçamento periódico, pagamentos reais, previsões e IA não estão implementados. |
| Dados | GORM/PostgreSQL; migrations `000001` a `000005` para usuários, transações, sessões, metas virtuais e enriquecimento dos lançamentos. `cmd/api/main.go` aplica migrations no início e compõe serviços de usuários, sessões, transações, metas e dashboard. | Falta política de retenção/limpeza de sessões expiradas; não há modelagem de contas bancárias, moedas ou liquidação de pagamentos. |
| Qualidade/entrega | Testes HTTP unitários de dashboard com dublês, além de testes Go e integração PostgreSQL condicionada a `TEST_DATABASE_URL`; CI com formatação, vet, OpenAPI, race, build, imagem e deploy homelab. | Teste integrado do dashboard com PostgreSQL ainda não executado localmente sem `TEST_DATABASE_URL`; confirmar execução no CI. AWS é meta, não implantação existente. |

Referências: `../go.mod`, `../cmd/api/main.go`, `../internal/user/`, `../internal/postgres/`, `../migrations/`, `openapi.yaml` e `../.github/workflows/backend.yml`.

## 3. Decisões de produto/contrato antes do código

Registrar decisões ainda pendentes no contrato OpenAPI e nos testes de aceitação antes de implementar novas funcionalidades:

1. **Identidade e acesso:** login por e-mail/senha com sessão opaca Bearer de 24 horas e hash do token no banco está implementado. `GET /users` retorna no máximo o perfil autenticado (com `offset > 0`, `data: []`); `PATCH /users/{id}` exige titularidade e responde `404` para outro ID. Pendente definir logout/revogação, renovação, recuperação de senha, rate limiting e eventual OAuth Google/Apple (fluxo, vinculação de contas e gestão de tokens); OAuth não está implementado.
2. **Transações e moeda:** `income` e `expense` já usam `amountMinor` positivo (`int64`), `description`, `category`, `installments` e `occurredAt`; só despesas exigem `necessityLevel` (1..5) e `paymentMethod` (`card`, `pix`, `debit`), e somente cartão admite mais de uma parcela. Receitas não têm necessidade ou forma de pagamento. O formato legado de despesa (`amountMinor` + `necessityLevel`) permanece compatível. Categoria é texto, sem catálogo; parcelamento é informativo, sem cobrança. **Decidir BRL ou USD**, política de conversão/segregação, arredondamento e exibição antes de agregar valores de moedas distintas; transferência, edição/exclusão, fuso de produto e pagamentos reais não foram definidos/implementados.
3. **Metas virtuais vs. orçamento:** `targetMinor` e `savedMinor` medem uma alocação virtual; depósito/retirada não movimentam conta nem dinheiro real. Pendente decidir limites por categoria, periodicidade, estornos e comportamento ao atingir/superar o alvo, antes de chamar metas de orçamento ou integração financeira.
4. **Dashboard e previsibilidade:** `GET /dashboard?month=AAAA-MM` em UTC (mês atual por padrão) já tem contrato gerado, handler autenticado, composição e testes HTTP unitários: entradas e saídas do mês, saldo líquido histórico *rastreado* (`netTrackedMinor`, não saldo bancário), totais virtuais de metas, até cinco transações recentes do mês e despesas por categoria. Pendente validar o fluxo integrado com PostgreSQL (`TEST_DATABASE_URL`). Previsões, CDI (fonte, taxa, periodicidade e método de cálculo) e indicadores futuros continuam pendentes; não apresentar retorno real ou previsão sem regras e dados.
5. **IA e integrações:** decidir se categorização inteligente usará regras ou IA, consentimento, correção de sugestões, privacidade, custo e métricas. OAuth Google/Apple, IA, pagamentos reais e CDI são decisões de produto/integração futuras, não funcionalidades existentes.

**Contrato de titularidade:** `POST /transactions` retorna `201` na criação, `400` para entrada inválida, `401` sem sessão válida e `500` para falha interna sem detalhes sensíveis. O titular vem do Bearer, nunca de `userId` no corpo. Recursos alheios endereçados por ID, como `PATCH /users/{id}` e ajuste de meta, retornam `404`; `409` é reservado a conflitos definidos (como e-mail já cadastrado).

## 4. Sequência de execução (Go)

### P0 — Fechar arquitetura, autenticação e contrato mínimo

- [ ] Fechar as decisões de produto ainda pendentes da seção 3; manter exemplos de request/response, erros e critérios de autorização em `docs/openapi.yaml`.
- [x] Implementar login por senha, token opaco aleatório de 24 horas, armazenamento somente do hash, identidade via Bearer e erros genéricos de credenciais, com testes.
- [ ] Definir logout/revogação, limpeza de sessões expiradas, rate limiting de login e política de renovação/recuperação de senha.
- [x] Proteger `GET /users` (somente perfil autenticado) e `PATCH /users/{id}` (somente titular, `404` para outro ID); considerar a mudança da antiga listagem global na integração com consumidores. Nenhum endpoint financeiro multiusuário deve ser publicado sem identificação/autorização.

**Aceite pendente de P0:** revisar contrato com o frontend fora deste workspace e cobrir revogação quando logout for implementado. Restrições de titularidade nas rotas de usuários estão no código e em testes; confirmar compatibilidade dos clientes.

### P1 — Domínio e caso de uso de criação de transação (TDD)

- [x] Criar `internal/transaction` com entidade/construtor, erros de domínio, contrato de repositório e serviço de criação; dependências externas entram por interfaces.
- [x] Testar valor positivo e titular válido; `necessityLevel` de 1 a 5 apenas em despesas e ausente em receitas; entrada inválida não chega à persistência.
- [x] Testar serviço com repositório fake, contexto, retorno e erros.
- [x] Ampliar transações com `kind` (`income`/`expense`), descrição, categoria textual, forma de pagamento, parcelas e data da operação, preservando a criação legada de despesas.
- [ ] Definir moeda (BRL/USD), política de fuso e eventual catálogo de categorias antes de expandir relatórios/integrações.

**Aceite:** testes unitários cobrem receita/despesa, valor e necessidade inválidos, regras de parcelas/pagamento, entrada HTTP e falha do repositório; nenhum pacote do domínio importa Gin ou GORM.

### P2 — Persistência e composição

- [x] Criar migrations `000002` para transações e `000003` para sessões, com FK, constraints, índices e arquivos de reversão.
- [x] Adicionar `000004_create_goals` (`goals`: titular, nome, alvo/saldo em unidades menores, timestamps, checks e índice por titular) e `000005_enrich_transactions` (`kind`, descrição, categoria, forma de pagamento, parcelas e `occurred_at`; checks e índice por titular/data). A `000005` converte registros antigos em `expense`, usa valores padrão e copia `created_at` para `occurred_at`.
- [x] Persistir metas virtuais e ajustes por titular; nenhum depósito/saque de meta é pagamento ou transferência real.
- [x] Criar repositórios PostgreSQL de transações e sessões; dinheiro em unidades menores (`BIGINT`/`int64`) e invariantes essenciais também no banco.
- [x] Compor repositórios e serviços em `cmd/api/main.go` com injeção explícita.
- [x] Adicionar listagem paginada de transações com filtro obrigatório por titular e ordenação estável.
- [ ] Planejar rollback com dados reais: `000004` remove metas; `000005.down` descarta os novos campos e transforma necessidade nula de receitas em `1`, perdendo a semântica original. Exigir backup e plano de recuperação antes de reverter em produção.

**Aceite:** testes de integração com PostgreSQL devem provar persistência exata, constraints, isolamento entre titulares e aplicação das cinco migrations em banco vazio; confirmar também rollback seguro em dados reais antes de produção.

### P3 — HTTP e contrato de transações

- [x] Especificar sessões, transações detalhadas, rotas de usuários restritas, metas e `GET /dashboard` em `docs/openapi.yaml`, com rotas correspondentes em `internal/api/openapi.gen.go`. **Não editar o arquivo gerado manualmente.**
- [x] Implementar handlers Gin com autenticação obrigatória para criar transações, validação de corpo e Content-Type, limite de tamanho, timeout, erros HTTP padronizados e `201` só após persistência.
- [x] Cobrir handlers com testes HTTP e teste de ponta a ponta com PostgreSQL para login, criação, expiração e segregação por titular.
- [ ] Validar o fluxo com o frontend e confirmar a execução dos testes de integração no CI.

**Aceite parcial:** testes de transações demonstram `201`/`400`/`401`/`500` e isolamento; o dashboard agora consta no contrato e nas rotas geradas, com cobertura HTTP unitária. Ainda é necessário executar a integração PostgreSQL com banco configurado antes de declarar validação ponta a ponta.

**APIs disponíveis:** `GET /health` é público; `POST /users` cadastra usuário; `POST /sessions` recebe `{"email":"pessoa@example.com","password":"senha"}` e retorna `token`, `tokenType: "Bearer"` e `expiresAt`. Enviar `Authorization: Bearer <token>` para `GET /users` (somente próprio perfil), `PATCH /users/{id}` (próprio ID), `GET/POST /transactions`, `GET/POST /goals`, `POST /goals/{id}/allocations` e `GET /dashboard?month=AAAA-MM` (parâmetro opcional; padrão é o mês UTC atual). `GET /transactions?limit=20&offset=0` e `GET /goals?limit=20&offset=0` paginam por titular (`data`, `limit`, `offset`; lista vazia como `[]`). Transações ordenam por `occurredAt` e ID decrescentes; metas por criação e ID decrescentes. A identidade vem exclusivamente do token.

**Exemplos financeiros:** `POST /transactions` aceita despesa legada `{"amountMinor":1299,"necessityLevel":3}` ou formato detalhado `{"kind":"expense","amountMinor":1299,"necessityLevel":3,"description":"Mercado","category":"Alimentação","paymentMethod":"pix","installments":1,"occurredAt":"2026-09-30T12:00:00Z"}`; receita detalhada usa `kind: "income"`, sem necessidade/forma de pagamento, e `installments: 1`. A resposta `201` inclui `id`, `kind`, `amountMinor`, `description`, `category`, `installments`, `occurredAt`, `createdAt` e os campos pertinentes ao tipo. `POST /goals` aceita `{"name":"Reserva","targetMinor":100000}` (saldo inicial zero); `POST /goals/{id}/allocations` aceita `{"direction":"deposit","amountMinor":5000}` ou `withdraw` e responde com `savedMinor` atualizado. Tudo é virtual; ver detalhes e erros em `docs/openapi.yaml`.

### P4 — Completar o MVP financeiro

- [x] Acrescentar listagem paginada por titular, com ordem estável e página vazia explícita.
- [ ] Acrescentar filtros por período/categoria, consulta por ID e edição/exclusão se aprovadas; cada operação sempre restrita ao titular.
- [x] Implementar metas virtuais por titular com criação, listagem e ajuste de saldo, migration e contrato; não confundir com orçamento por período nem pagamentos reais.
- [x] Implementar dashboard mensal: `GET /dashboard` no OpenAPI gerado e nas rotas HTTP, handler autenticado com validação de `month` (`AAAA-MM` em UTC, padrão mês atual), `overview.Service`/`OverviewRepository` compostos em `cmd/api/main.go` e testes HTTP unitários com dublês para autenticação, mês, resposta e erros.
- [ ] Validar dashboard integrado com PostgreSQL usando `TEST_DATABASE_URL` e confirmar execução no CI; teste integrado ainda não executado localmente sem essa variável.
- [ ] Definir orçamento por período/categoria e previsões com regras e histórico mínimo. Para categorização por IA, decidir estratégia e testar correção de sugestões; CDI e pagamentos reais requerem desenho próprio.

**Aceite parcial:** usuário registra e consulta receitas/despesas, acompanha metas virtuais e consulta o dashboard mensal pela API; testes HTTP unitários cobrem o endpoint. Falta verificar números e isolamento ponta a ponta contra PostgreSQL com `TEST_DATABASE_URL`. Orçamento periódico, IA e previsões não fazem parte do escopo já entregue.

### P5 — Integração, segurança e operação

- [ ] Entregar ao frontend (fora deste workspace) o OpenAPI **sincronizado com a API gerada** e exemplos de login, perfis, transações e metas; validar fluxo ponta a ponta em ambiente de teste. Configurar CORS por origem conhecida e política de credenciais conforme a autenticação escolhida; não alterar componentes de UI neste repositório.
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

Para executar **também** testes de integração PostgreSQL, configurar `TEST_DATABASE_URL` para um **banco de testes descartável**; sem ela, os testes de integração são ignorados localmente, e no CI a ausência é erro. O teste integrado do dashboard ainda não foi executado localmente sem essa variável. Com o banco disponível, executar `go test -race -count=1 -coverprofile=coverage.out ./...`. O OpenAPI já foi gerado para `/dashboard`; ao regenerar futuramente, conferir o diff do código gerado.

## 6. Riscos e dependências

- **Consistência técnica:** seguir a stack Go estabelecida e evitar serviços paralelos ou adaptações que dupliquem as responsabilidades do backend existente.
- **Privacidade:** rotas financeiras e consulta/edição de perfil exigem Bearer e restringem titularidade; confirmar ausência de regressões em clientes que dependiam da antiga listagem global de usuários.
- **Validação do dashboard:** `GET /dashboard` está no YAML, no código gerado e no servidor; os testes HTTP unitários usam dublês. Falta executar localmente a integração com PostgreSQL usando `TEST_DATABASE_URL` e confirmar sua execução no CI antes de afirmar validação ponta a ponta.
- **Moeda e integrações:** valores em unidades menores não carregam código de moeda; decidir BRL/USD antes de misturar lançamentos ou calcular retornos. OAuth Google/Apple, IA, pagamentos reais e CDI dependem de decisões de produto, segurança e fontes externas. Metas e `netTrackedMinor` não representam saldo bancário.
- **Migrações:** reversão de `000004`/`000005` pode perder dados; planejar backup/restore e compatibilidade dos consumidores.
- **Infraestrutura:** o CI publica imagem e faz deploy no homelab; “cloud-ready/AWS” requer arquitetura e validação próprias, não deve ser declarado concluído pelo Docker atual.
