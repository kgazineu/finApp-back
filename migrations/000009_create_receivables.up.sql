CREATE TYPE receivable_kind AS ENUM ('split', 'loan');

CREATE TABLE receivables (
    id            BIGSERIAL PRIMARY KEY,
    user_id       UUID NOT NULL REFERENCES users (id),
    kind          receivable_kind NOT NULL,
    debtor        TEXT NOT NULL,             -- quem te deve
    description   TEXT NOT NULL,             -- "pizza", "relógio", "bicicleta"
    amount        BIGINT NOT NULL,           -- valor sem juros, em centavos
    interest_rate INT NOT NULL DEFAULT 0,    -- 5 = 5%
    archived_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (kind = 'loan' OR interest_rate = 0)  -- divisão não tem juros
);

CREATE INDEX receivables_user_id_idx ON receivables (user_id);

-- as parcelas: criadas junto com a dívida
CREATE TABLE receivable_installments (
    id            BIGSERIAL PRIMARY KEY,
    receivable_id BIGINT NOT NULL REFERENCES receivables (id),  -- sem cascade: parcela recebida segura o receivable (vira arquivado)
    number        INT NOT NULL,                                 -- parcela 1, 2, 3...
    amount        BIGINT NOT NULL,                              -- valor da parcela, já com juros
    due_date      DATE NOT NULL,                                -- quando vence
    paid_at       TIMESTAMPTZ,                                  -- a checkbox: NULL = não recebi
    UNIQUE (receivable_id, number)
);
