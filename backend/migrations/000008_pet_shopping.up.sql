ALTER TABLE shopping_lists ADD COLUMN household_id uuid REFERENCES households(id);
UPDATE shopping_lists SET household_id='00000000-0000-4000-8000-000000000010' WHERE household_id IS NULL;
ALTER TABLE shopping_lists ALTER COLUMN household_id SET NOT NULL;
ALTER TABLE shopping_lists ADD CONSTRAINT shopping_lists_household_id_id_unique UNIQUE(household_id,id);
CREATE INDEX shopping_lists_household_idx ON shopping_lists(household_id,created_at,id);

ALTER TABLE shopping_items ADD COLUMN household_id uuid REFERENCES households(id);
UPDATE shopping_items i SET household_id=l.household_id FROM shopping_lists l WHERE l.id=i.list_id;
ALTER TABLE shopping_items ALTER COLUMN household_id SET NOT NULL;
ALTER TABLE shopping_items ADD CONSTRAINT shopping_items_household_id_id_unique UNIQUE(household_id,id);
ALTER TABLE shopping_items ADD CONSTRAINT shopping_items_list_same_household_fk
    FOREIGN KEY(household_id,list_id) REFERENCES shopping_lists(household_id,id) ON DELETE CASCADE;

CREATE TABLE shopping_item_pets(
    household_id uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    item_id uuid NOT NULL,
    pet_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(item_id,pet_id),
    FOREIGN KEY(household_id,item_id) REFERENCES shopping_items(household_id,id) ON DELETE CASCADE,
    FOREIGN KEY(household_id,pet_id) REFERENCES pets(household_id,id) ON DELETE CASCADE
);
CREATE INDEX shopping_item_pets_household_pet_item_idx ON shopping_item_pets(household_id,pet_id,item_id);
