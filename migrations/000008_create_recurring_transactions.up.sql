CREATE TYPE recurring_transaction_kind AS ENUM ('income', 'expense');

CREATE TABLE recurring_transactions (
    id              BIGSERIAL PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES users (id),
    description     TEXT NOT NULL,
    kind            recurring_transaction_kind NOT NULL,  -- entrada ou despesa
    is_fixed        BOOLEAN NOT NULL,                     -- fixa ou variável
    amount          BIGINT NOT NULL,                      -- centavos
    start_month     DATE NOT NULL,                        -- primeiro mês em que cai (sempre dia 1)
    interval_months INT NOT NULL DEFAULT 1 CHECK (interval_months > 0),  -- a cada X meses
    end_month       DATE,                                 -- último mês; NULL = sem fim
    day_of_month    INT CHECK (day_of_month BETWEEN 1 AND 31),           -- NULL = sem dia certo
    archived_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (end_month IS NULL OR end_month >= start_month)
);

CREATE INDEX recurring_transactions_user_id_idx ON recurring_transactions (user_id);

CREATE TABLE recurring_transaction_installments (
    id             BIGSERIAL PRIMARY KEY,
    transaction_id BIGINT NOT NULL REFERENCES recurring_transactions (id),  -- sem cascade: parcela paga segura a transaction (vira arquivada)
    number         INT NOT NULL,                                            -- 1ª, 2ª, 3ª vez que cai
    amount         BIGINT NOT NULL,                                         -- cópia do valor na hora de gerar
    due_date       DATE NOT NULL,                                           -- sem dia certo = último dia do mês
    paid_at        TIMESTAMPTZ,                                             -- a checkbox: NULL = não pago/recebido
    UNIQUE (transaction_id, number)
);
