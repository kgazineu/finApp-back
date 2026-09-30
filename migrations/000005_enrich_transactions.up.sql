ALTER TABLE transactions
    ADD COLUMN kind TEXT NOT NULL DEFAULT 'expense',
    ADD COLUMN description TEXT NOT NULL DEFAULT 'Lançamento',
    ADD COLUMN category TEXT NOT NULL DEFAULT 'Outros',
    ADD COLUMN payment_method TEXT,
    ADD COLUMN installments INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN occurred_at TIMESTAMPTZ NOT NULL DEFAULT now();

UPDATE transactions SET occurred_at = created_at;

ALTER TABLE transactions
    ALTER COLUMN necessity_level DROP NOT NULL,
    ADD CONSTRAINT transactions_kind_valid CHECK (kind IN ('income', 'expense')),
    ADD CONSTRAINT transactions_description_valid CHECK (length(btrim(description)) BETWEEN 1 AND 200),
    ADD CONSTRAINT transactions_category_valid CHECK (length(btrim(category)) BETWEEN 1 AND 80),
    ADD CONSTRAINT transactions_payment_method_valid CHECK (
        (kind = 'income' AND payment_method IS NULL)
        OR (kind = 'expense' AND (payment_method IS NULL OR payment_method IN ('card', 'pix', 'debit')))
    ),
    ADD CONSTRAINT transactions_installments_valid CHECK (
        installments BETWEEN 1 AND 60 AND (installments = 1 OR (payment_method IS NOT NULL AND payment_method = 'card'))
    );

ALTER TABLE transactions DROP CONSTRAINT transactions_necessity_level_valid;
ALTER TABLE transactions ADD CONSTRAINT transactions_necessity_level_valid CHECK (
    (kind = 'income' AND necessity_level IS NULL)
    OR (kind = 'expense' AND necessity_level IS NOT NULL AND necessity_level BETWEEN 1 AND 5)
);

CREATE INDEX transactions_user_occurred_id_idx ON transactions (user_id, occurred_at DESC, id DESC);
