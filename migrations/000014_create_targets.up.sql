-- metas: chegar a um valor até uma data, olhando o saldo total ou o de uma conta de saldo.
-- O progresso vem do último registro de saldos (calculado na tela); aqui fica só o alvo.
CREATE TABLE targets (
    id         BIGSERIAL PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users (id),
    name       TEXT NOT NULL CHECK (btrim(name) <> ''),
    amount     BIGINT NOT NULL CHECK (amount > 0),  -- quanto quer ter, em centavos
    deadline   DATE NOT NULL,                       -- prazo
    account_id BIGINT REFERENCES accounts (id),     -- null = saldo total; conta com meta vira arquivada ao remover
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX targets_user_deadline_idx ON targets (user_id, deadline, id);
