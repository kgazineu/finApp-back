DROP INDEX transactions_user_occurred_id_idx;

ALTER TABLE transactions DROP CONSTRAINT transactions_necessity_level_valid;
ALTER TABLE transactions DROP CONSTRAINT transactions_installments_valid;
ALTER TABLE transactions DROP CONSTRAINT transactions_payment_method_valid;
ALTER TABLE transactions DROP CONSTRAINT transactions_category_valid;
ALTER TABLE transactions DROP CONSTRAINT transactions_description_valid;
ALTER TABLE transactions DROP CONSTRAINT transactions_kind_valid;

-- The old schema cannot represent income; downgrade maps its NULL necessity to 1.
UPDATE transactions SET necessity_level = 1 WHERE necessity_level IS NULL;

ALTER TABLE transactions
    ALTER COLUMN necessity_level SET NOT NULL,
    DROP COLUMN occurred_at,
    DROP COLUMN installments,
    DROP COLUMN payment_method,
    DROP COLUMN category,
    DROP COLUMN description,
    DROP COLUMN kind,
    ADD CONSTRAINT transactions_necessity_level_valid CHECK (necessity_level BETWEEN 1 AND 5);
