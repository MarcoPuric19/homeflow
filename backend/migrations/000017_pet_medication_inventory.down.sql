ALTER TABLE pet_medications
    DROP CONSTRAINT pet_medications_inventory_unit_check,
    DROP CONSTRAINT pet_medications_inventory_dates_check,
    DROP COLUMN expires_on,
    DROP COLUMN purchased_on,
    DROP COLUMN minimum_stock,
    DROP COLUMN package_size;
