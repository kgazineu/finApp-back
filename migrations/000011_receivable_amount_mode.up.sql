-- como o valor informado vira parcelas:
--   'total'       = valor total, dividido entre as parcelas (com juros simples)
--   'installment' = valor de cada parcela (ex.: assinatura mensal); não tem juros para dividir
ALTER TABLE receivables
    ADD COLUMN amount_mode TEXT NOT NULL DEFAULT 'total',
    ADD CONSTRAINT receivables_amount_mode_valid CHECK (amount_mode IN ('total', 'installment')),
    ADD CONSTRAINT receivables_installment_mode_without_interest CHECK (amount_mode = 'total' OR interest_rate = 0);
