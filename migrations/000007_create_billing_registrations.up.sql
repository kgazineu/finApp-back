-- valores em centavos: R$ 6.505,50 = 650550
CREATE TABLE billing_registrations (
    id         BIGSERIAL PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users (id),
    delta      BIGINT NOT NULL,
    total      BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX billing_registrations_user_id_idx ON billing_registrations (user_id, id);

CREATE TABLE billing_entries (
    id                      BIGSERIAL PRIMARY KEY,
    billing_registration_id BIGINT NOT NULL REFERENCES billing_registrations (id) ON DELETE CASCADE,
    account_id              BIGINT NOT NULL REFERENCES accounts (id),  -- sem cascade: conta com histórico vira arquivada
    account_name            TEXT   NOT NULL,                           -- cópia do nome na época do registro
    amount                  BIGINT NOT NULL,
    UNIQUE (billing_registration_id, account_id)
);
