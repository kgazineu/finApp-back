-- um código ativo por usuário; o código em si nunca é guardado, só o SHA-256
CREATE TABLE password_resets (
    user_id    UUID PRIMARY KEY REFERENCES users (id),
    code_hash  BYTEA NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    attempts   INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
