ALTER TABLE pet_medications
    ADD COLUMN package_size numeric(12,3) CHECK (package_size > 0),
    ADD COLUMN minimum_stock numeric(12,3) CHECK (minimum_stock >= 0),
    ADD COLUMN purchased_on date,
    ADD COLUMN expires_on date,
    ADD CONSTRAINT pet_medications_inventory_dates_check CHECK (expires_on IS NULL OR purchased_on IS NULL OR expires_on >= purchased_on),
    ADD CONSTRAINT pet_medications_inventory_unit_check CHECK ((package_size IS NULL AND minimum_stock IS NULL) OR length(stock_unit) > 0);
