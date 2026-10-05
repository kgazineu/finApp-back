CREATE TYPE account_kind AS ENUM ('asset', 'liability');

CREATE TABLE accounts (
    id          BIGSERIAL PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users (id),
    name        TEXT NOT NULL,
    kind        account_kind NOT NULL,
    has_yield   BOOLEAN NOT NULL DEFAULT false,
    archived_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT liability_without_yield CHECK (kind = 'asset' OR has_yield = false)
);

CREATE INDEX accounts_user_id_idx ON accounts (user_id);
