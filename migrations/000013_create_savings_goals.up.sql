-- meta de guardar por mês: porcentagem do que sobra no mês ou valor fixo (um dos dois).
-- Só aparece na tela de projeção; nenhum cálculo da API usa.
CREATE TABLE savings_goals (
    user_id    UUID PRIMARY KEY REFERENCES users (id),
    percent    INT CHECK (percent BETWEEN 1 AND 100),
    amount     BIGINT CHECK (amount > 0),  -- centavos
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT savings_goals_one_kind CHECK ((percent IS NULL) <> (amount IS NULL))
);
