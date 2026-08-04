-- external_payments indicates that the swap and prepay invoices are paid by
-- the caller instead of through lnd's router. Existing swaps retain the
-- original internal-payment behavior.
ALTER TABLE loopout_swaps
ADD COLUMN external_payments BOOLEAN NOT NULL DEFAULT FALSE;
