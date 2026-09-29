# FinControl — plano de conclusão do backend

**Estado de referência:** repositório `finApp-back`, inspecionado em 29/09/2026. Este documento é um plano de execução, não uma declaração de que as funcionalidades futuras já estão implementadas. Revisar prioridades e decisões à medida que os contratos de produto forem fechados.

## 1. Diretriz técnica definitiva

**Toda a implementação do backend será em Go**, evoluindo a aplicação existente. A stack é Go (versão declarada em `go.mod`), Gin para HTTP, GORM com PostgreSQL para persistência, migrations SQL e testes com `testing` e doubles próprios. Domínio e casos de uso permanecem independentes de Gin e GORM; interfaces definidas junto às funcionalidades separam regras de negócio da infraestrutura.

As etapas deste documento seguem essa decisão: novos serviços, repositórios, handlers, testes e composição de dependências devem usar os padrões Go já adotados no repositório. Mudanças de stack não fazem parte do roadmap.

O frontend React/Vite/TypeScript e seu design system são contexto de integração, **não fazem parte do escopo deste repositório**. Endpoints de login e transações também não existem aqui: não conectar o Axios a rotas presumidas.

## 2. Baseline verificado

| Área | Implementado | Lacuna para o produto |
| --- | --- | --- |
| API | Gin; `/health`, `POST /users`, `GET /users`, `PATCH /users/{id}` definidos em `docs/openapi.yaml`; handlers gerados via `go generate ./internal/api`; `/docs/` e `/openapi.json`. | Sem login, sessão ou proteção das rotas; sem endpoints de transações, categorias e Goals. |
| Domínio/aplicação | `internal/user` contém validação, serviço e contrato `Repository`; criação testada com doubles. | Sem domínio `Transaction`, `NecessityLevel`, casos de uso financeiros ou regras de orçamento. |
| Dados | `internal/postgres`, GORM e migration `000001_create_users`; `cmd/api/main.go` liga repositório, serviço e API e aplica migrations no início. | Sem tabelas de transações, categorias e Goals; sem segregação de dados por titular. |
| Qualidade/entrega | Testes Go, integração PostgreSQL com `TEST_DATABASE_URL`, CI com formatação, vet, geração OpenAPI, race, build, publicação de imagem e deploy homelab. | Sem testes de fluxos financeiros/autorização; AWS é uma meta, não uma implantação existente. |

Referências: `../go.mod`, `../cmd/api/main.go`, `../internal/user/`, `../internal/postgres/`, `../migrations/`, `openapi.yaml` e `../.github/workflows/backend.yml`.

## 3. Decisões de produto/contrato antes do código

Registrar as respostas no contrato OpenAPI e nos testes de aceitação antes da implementação:

1. **Identidade e acesso:** login por e-mail/senha? token ou cookie? expiração, logout/renovação, recuperação de senha e política de exposição de `GET /users`/`PATCH /users/{id}`. Atualmente essas rotas não têm autorização; restringi-las antes de produção com dados reais.
2. **Transação:** despesa apenas ou também receita/transferência? `NecessityLevel` obrigatório para todas ou só despesas? categoria obrigatória? data da operação vs. criação; moeda, fuso e edição/exclusão. Definir formato de valor (decimal exato/unidades inteiras menores, nunca `float` para dinheiro), arredondamento e moeda suportada.
3. **Categorias inteligentes:** catálogo fixo ou categorias por usuário? “Inteligente” significa regras, sugestão automática ou classificação por modelo? Definir consentimento, possibilidade de correção e métricas antes de introduzir IA ou serviços externos.
4. **Goals/orçamentos:** valor-alvo, limite por categoria ou ambos? periodicidade, tratamento de estornos, cálculo de progresso e comportamento ao ultrapassar o limite.
5. **Previsibilidade:** quais indicadores e horizontes serão entregues no MVP, e com que regras de cálculo e atualização. Não prometer previsões sem dados/regras definidos.

**Contrato preliminar, sujeito às decisões acima:** `POST /transactions` deve resultar em `201` na criação, `400` para entrada inválida e `401` sem identidade válida; `403`/`404` conforme a política de recurso alheio e `500` para falha interna sem detalhes sensíveis. `409` apenas para conflito de negócio definido. Nunca confiar em `userId` enviado no corpo para atribuir titularidade.

## 4. Sequência de execução (Go)

### P0 — Fechar arquitetura, autenticação e contrato mínimo

- [ ] Fechar as decisões de produto da seção 3; versionar exemplos de request/response, erros e critérios de autorização em `docs/openapi.yaml`.
- [ ] Desenhar e testar o fluxo de login/autenticação e a obtenção da identidade no request; verificar hash de senha existente com o componente em `internal/password`. Definir armazenamento/rotação de segredo e tratamento de credenciais inválidas, rate limiting e respostas sem vazamento de informação.
- [ ] Proteger as rotas de usuários existentes conforme a política definida; planejar compatibilidade dos consumidores antes de alterar seu comportamento. Nenhum endpoint financeiro multiusuário deve ser publicado sem identificação/autorização.

**Aceite:** contrato de login revisado com frontend; chamadas não autenticadas ou de outro usuário não expõem dados; testes cobrem credenciais inválidas, expiração/revogação conforme o mecanismo escolhido e autorização.

### P1 — Domínio e caso de uso de criação de transação (TDD)

- [ ] Criar `internal/transaction` seguindo o padrão de `internal/user`: entidade/construtor, erros de domínio, contrato de repositório e serviço de criação; dependências externas entram por interfaces. Organizar os casos de uso por funcionalidade em pacotes Go.
- [ ] Escrever testes de domínio primeiro: valor estritamente positivo; `NecessityLevel` **inteiro de 1 a 5** quando aplicável; categoria/data/moeda conforme contrato; identidade titular válida. Uma transação inválida não deve chegar à persistência.
- [ ] Escrever testes do serviço com repositório fake: validação, repasse de contexto e titular autenticado, persistência única, retorno e propagação de erros. Implementar a lógica até os testes passarem (ciclo vermelho → verde → refatoração).

**Aceite:** testes unitários de casos válidos, bordas (0, negativo, 1, 5, fora de 1..5 e valores não inteiros no limite HTTP) e falha do repositório; nenhum pacote do domínio importa Gin ou GORM.

### P2 — Persistência e composição

- [ ] Criar migration incremental `000002_...up.sql` **e** correspondente `.down.sql` para transações (e categorias se o contrato exigir), com `user_id` relacionado a `users`, constraints de valor/necessidade e índices adequados às consultas por titular e data. Planejar impacto do rollback antes de executá-lo em ambiente com dados reais.
- [ ] Criar modelo/mapeamento e repositório em `internal/postgres`, respeitando precisão monetária, contexto, errors de domínio e filtro por titular nas leituras/escritas. Não depender apenas de validação Go: reforçar invariantes essenciais no banco.
- [ ] Compor o repositório e serviço em `cmd/api/main.go` pelo padrão de injeção explícita de dependências já existente.

**Aceite:** testes de integração com PostgreSQL provam criação, persistência exata, constraints e isolamento entre dois usuários; migrations aplicam em banco vazio e a composição inicia sem regressões.

### P3 — HTTP e contrato de transações

- [ ] Especificar `POST /transactions` e schemas/erros em `docs/openapi.yaml`; rodar `go generate ./internal/api` e versionar `internal/api/openapi.gen.go`. **Não editar o arquivo gerado manualmente.**
- [ ] Implementar handler Gin em `internal/api` e injetar o serviço no `Server` conforme o padrão atual. Validar corpo e Content-Type, limitar tamanho, usar contexto/timeout, mapear erros de domínio para `400` e falhas inesperadas para `500`, sem retornar stack trace. `201` só depois de persistir com sucesso.
- [ ] Cobrir com testes HTTP e de ponta a ponta o payload válido, inválido, malformado, sem autorização, acesso cruzado, falha de banco e formato de resposta; alinhar com o frontend o endpoint e exemplos reais.

**Aceite:** contrato gerado corresponde ao YAML, testes demonstram `201`/`400`/`401`/`500` e isolamento; `go test ./...` passa com banco de integração configurado.

### P4 — Completar o MVP financeiro

- [ ] Acrescentar listagem paginada/filtrada por período e categoria, consulta por ID e edição/exclusão se aprovadas; cada operação sempre restrita ao titular. Definir ordenação estável e comportamento de página vazia.
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
- **Privacidade:** cadastro/listagem/atualização de usuários atualmente não constituem login/autorização; tratar autenticação e acesso por titular como pré-requisitos de produção.
- **Contratos em aberto:** sem acordar dinheiro, moeda, receitas, categorias e Goals, schema e endpoints podem exigir migrations quebrando clientes.
- **Infraestrutura:** o CI publica imagem e faz deploy no homelab; “cloud-ready/AWS” requer arquitetura e validação próprias, não deve ser declarado concluído pelo Docker atual.
