-- valores a receber com parcela fixa passam a ser lidos como "total" (as parcelas em si não mudam)
ALTER TABLE receivables
    DROP CONSTRAINT receivables_installment_mode_without_interest,
    DROP CONSTRAINT receivables_amount_mode_valid,
    DROP COLUMN amount_mode;
