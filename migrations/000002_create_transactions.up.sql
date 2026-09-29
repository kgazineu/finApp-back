CREATE TABLE transactions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id),
    amount_minor BIGINT NOT NULL,
    necessity_level SMALLINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT transactions_amount_positive CHECK (amount_minor > 0),
    CONSTRAINT transactions_necessity_level_valid CHECK (necessity_level BETWEEN 1 AND 5)
);

CREATE INDEX transactions_user_created_id_idx ON transactions (user_id, created_at DESC, id DESC);
