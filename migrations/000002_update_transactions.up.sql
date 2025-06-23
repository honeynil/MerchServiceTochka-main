ALTER TABLE transactions
    ALTER COLUMN related_id DROP NOT NULL,
    DROP CONSTRAINT transactions_type_check,
    ADD CONSTRAINT transactions_type_check CHECK (type IN ('purchase', 'transfer', 'initial'));