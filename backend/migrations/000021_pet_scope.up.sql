-- HF-0116: HomeFlow wird fachlich eine Haustier-App. Ab hier gibt es keine
-- Ausgabe, keinen Kalendereintrag und keine Einkaufsliste mehr ohne Tierbezug.
--
-- households und household_members bleiben unverändert. Sie sind die
-- Mandantengrenze, an der die Zwei-Benutzer-Regel (000002) und jede
-- Autorisierungsabfrage hängen - das ist eine Schemafrage, keine Produktfrage.

-- Kein BEGIN/COMMIT hier: der Migrationslauf in postgres.Migrate umschließt
-- jede Datei bereits mit einer eigenen Transaktion. Ein zweites BEGIN würde
-- schachteln, und ein COMMIT mittendrin würde die Absicherung aufheben.
--
-- 1. Zuordnung vor Constraint.
--
-- Kein Default, kein "nimm das erste Tier": eine Ausgabe dem falschen Tier
-- zuzuschlagen ist schlimmer als eine Migration, die abbricht. Wer verwaiste
-- Zeilen hat, ordnet sie über die API zu oder löscht sie - und sieht in der
-- Meldung, wie viele es sind.
DO $$
DECLARE offen int;
BEGIN
    SELECT count(*) INTO offen FROM expenses WHERE pet_id IS NULL;
    IF offen > 0 THEN
        RAISE EXCEPTION
            'HF-0116: % Ausgabe(n) ohne Tierbezug. Vor der Migration über '
            'PATCH /api/v1/expenses/{id} einem Tier zuordnen oder löschen. '
            'Die Migration errät kein Tier.', offen;
    END IF;

    SELECT count(*) INTO offen FROM calendar_events WHERE pet_id IS NULL;
    IF offen > 0 THEN
        RAISE EXCEPTION
            'HF-0116: % Kalendereintrag/-einträge ohne Tierbezug. Vor der '
            'Migration über PATCH /api/v1/calendar/events/{id} zuordnen oder '
            'löschen.', offen;
    END IF;
END $$;

ALTER TABLE expenses        ALTER COLUMN pet_id SET NOT NULL;
ALTER TABLE calendar_events ALTER COLUMN pet_id SET NOT NULL;

-- 2. Der Termintyp hing bisher am Tier: ohne Tier kein Typ. Jetzt ist das Tier
--    Pflicht, also ist die halbe Bedingung tot - die Prüfung wird ersetzt statt
--    stehen gelassen, damit niemand sie später falsch liest.
ALTER TABLE calendar_events DROP CONSTRAINT IF EXISTS calendar_events_pet_appointment_check;

-- 3. Die Liste bleibt als Behälter und wird umbenannt; gelöscht wird sie NICHT.
--    Das Ticket sagte "die generische gemeinsame Liste verschwindet", und das
--    stimmt fachlich - technisch hängen aber alle Artikel per Fremdschlüssel
--    daran. Was verschwindet, ist der Artikel ohne Tierbezug, nicht der
--    Behälter. Eine gelöschte Liste hätte jeden vorhandenen Artikel mitgenommen.
UPDATE shopping_lists SET name = 'Tier-Einkaufsliste'
 WHERE id = '00000000-0000-4000-8000-000000000100';

--    Ein Artikel gehört ab jetzt
--    zu einem, zu mehreren oder ausdrücklich zu allen Tieren - über
--    shopping_item_pets. "Allen" ist eine Markierung und nicht die leere Menge,
--    sonst ist es von "vergessen" nicht zu unterscheiden.
ALTER TABLE shopping_items
    ADD COLUMN IF NOT EXISTS for_all_pets boolean NOT NULL DEFAULT false;

-- Bestehende Artikel ohne jeden Tierbezug gelten als "für alle Tiere". Das ist
-- die einzige Stelle mit einer Annahme, und sie ist verlustfrei: sie behauptet
-- kein bestimmtes Tier.
UPDATE shopping_items si
   SET for_all_pets = true
 WHERE NOT EXISTS (SELECT 1 FROM shopping_item_pets p WHERE p.item_id = si.id);

-- Kein CHECK auf "mindestens ein Tier oder for_all_pets": die Bedingung hängt
-- an shopping_item_pets, und eine Zeilenprüfung kann keine andere Tabelle
-- lesen. Ein Trigger wäre möglich und wäre auf einem Zwei-Personen-System mehr
-- Maschinerie als Nutzen. Durchgesetzt wird es im Dienst, und der Test dafür ist
-- Pflicht - hier steht ausdrücklich, dass das Schema an dieser Stelle nicht
-- hilft.
