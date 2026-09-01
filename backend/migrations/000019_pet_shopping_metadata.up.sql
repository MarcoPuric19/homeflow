ALTER TABLE shopping_items
    ADD COLUMN preferred_brand text NOT NULL DEFAULT '' CHECK (length(preferred_brand) <= 200),
    ADD COLUMN estimated_price_cents bigint CHECK (estimated_price_cents >= 0),
    ADD COLUMN estimated_price_currency char(3) NOT NULL DEFAULT 'EUR',
    ADD COLUMN recurring boolean NOT NULL DEFAULT false;
