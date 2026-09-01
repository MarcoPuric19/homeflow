package pets

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"homeflow/backend/internal/outbox"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const petColumns = `p.id::text,p.name,p.species,p.breed,p.birth_date::text,p.approximate_birth_date,p.sex,p.microchip_number,p.color,p.insurance,p.insurance_number,p.veterinarian,p.notes,p.active,p.created_at,p.updated_at`
const insertedPetColumns = `id::text,name,species,breed,birth_date::text,approximate_birth_date,sex,microchip_number,color,insurance,insurance_number,veterinarian,notes,active,created_at,updated_at`

func scanPet(row pgx.Row) (Pet, error) {
	var pet Pet
	err := row.Scan(&pet.ID, &pet.Name, &pet.Species, &pet.Breed, &pet.BirthDate, &pet.ApproximateBirthDate, &pet.Sex, &pet.MicrochipNumber, &pet.Color, &pet.Insurance, &pet.InsuranceNumber, &pet.Veterinarian, &pet.Notes, &pet.Active, &pet.CreatedAt, &pet.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pet{}, ErrNotFound
	}
	return pet, err
}

func (r *PostgresRepository) Create(ctx context.Context, userID, createdBy string, pet Pet) (Pet, error) {
	return outbox.Run(ctx, r.pool, func(tx pgx.Tx) (Pet, error) {
		created, err := scanPet(tx.QueryRow(ctx, `INSERT INTO pets
      (id,household_id,name,species,breed,birth_date,approximate_birth_date,sex,microchip_number,color,insurance,insurance_number,veterinarian,notes,active,created_by,created_at,updated_at)
      SELECT $2,hm.household_id,$3,$4,$5,$6::date,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18
      FROM household_members hm WHERE hm.user_id=$1
		RETURNING `+insertedPetColumns,
			userID, pet.ID, pet.Name, pet.Species, pet.Breed, pet.BirthDate, pet.ApproximateBirthDate, pet.Sex,
			pet.MicrochipNumber, pet.Color, pet.Insurance, pet.InsuranceNumber, pet.Veterinarian, pet.Notes, pet.Active, createdBy, pet.CreatedAt, pet.UpdatedAt))
		if err != nil {
			return Pet{}, err
		}
		return created, outbox.WriteForPet(ctx, tx, "pet.created", "pet", created.ID, created.ID, nil)
	})
}

func (r *PostgresRepository) List(ctx context.Context, userID string, includeInactive bool, limit int, after string) (Page, error) {
	var cursor any
	if after != "" {
		cursor = after
	}
	rows, err := r.pool.Query(ctx, `SELECT `+petColumns+`
      FROM pets p JOIN household_members hm ON hm.household_id=p.household_id
      WHERE hm.user_id=$1 AND ($2 OR p.active) AND ($3::uuid IS NULL OR p.id > $3::uuid)
      ORDER BY p.id LIMIT $4`, userID, includeInactive, cursor, limit+1)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	page := Page{Pets: make([]Pet, 0, limit)}
	for rows.Next() {
		pet, err := scanPet(rows)
		if err != nil {
			return Page{}, err
		}
		page.Pets = append(page.Pets, pet)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if len(page.Pets) > limit {
		page.NextCursor = page.Pets[limit-1].ID
		page.Pets = page.Pets[:limit]
	}
	return page, nil
}

func (r *PostgresRepository) Get(ctx context.Context, userID, petID string) (Pet, error) {
	return scanPet(r.pool.QueryRow(ctx, `SELECT `+petColumns+`
      FROM pets p JOIN household_members hm ON hm.household_id=p.household_id
      WHERE hm.user_id=$1 AND p.id=$2`, userID, petID))
}

func (r *PostgresRepository) Update(ctx context.Context, userID string, pet Pet) (Pet, error) {
	return outbox.Run(ctx, r.pool, func(tx pgx.Tx) (Pet, error) {
		updated, err := scanPet(tx.QueryRow(ctx, `UPDATE pets p SET name=$3,species=$4,breed=$5,birth_date=$6::date,
      approximate_birth_date=$7,sex=$8,microchip_number=$9,color=$10,insurance=$11,insurance_number=$12,veterinarian=$13,notes=$14,active=$15,updated_at=$16
      FROM household_members hm
      WHERE p.id=$2 AND hm.user_id=$1 AND hm.household_id=p.household_id
		RETURNING `+petColumns, userID, pet.ID, pet.Name, pet.Species, pet.Breed, pet.BirthDate,
			pet.ApproximateBirthDate, pet.Sex, pet.MicrochipNumber, pet.Color, pet.Insurance, pet.InsuranceNumber, pet.Veterinarian, pet.Notes, pet.Active, pet.UpdatedAt))
		if err != nil {
			return Pet{}, err
		}
		return updated, outbox.WriteForPet(ctx, tx, "pet.updated", "pet", updated.ID, updated.ID, nil)
	})
}

func (r *PostgresRepository) ListTimeline(ctx context.Context, userID, petID string, limit int, cursor *timelineCursor) (TimelinePage, error) {
	var cursorDate, cursorType, cursorID any
	if cursor != nil {
		cursorDate, cursorType, cursorID = cursor.Date, cursor.Type, cursor.SourceID
	}
	rows, err := r.pool.Query(ctx, `WITH pet_scope AS (
	  SELECT p.household_id,p.id FROM pets p JOIN household_members hm ON hm.household_id=p.household_id WHERE hm.user_id=$1 AND p.id=$2
	), timeline AS (
	  SELECT 'medical_record'::text AS type,m.occurred_on AS event_date,m.title,m.id FROM medical_records m JOIN pet_scope p ON p.household_id=m.household_id AND p.id=m.pet_id
	  UNION ALL SELECT 'vaccination',v.administered_on,v.vaccine_name,v.id FROM vaccinations v JOIN pet_scope p ON p.household_id=v.household_id AND p.id=v.pet_id
	  UNION ALL SELECT 'medication',pm.starts_on,m.name,pm.id FROM pet_medications pm JOIN medications m ON m.household_id=pm.household_id AND m.id=pm.medication_id JOIN pet_scope p ON p.household_id=pm.household_id AND p.id=pm.pet_id
	  UNION ALL SELECT 'weight',w.measured_on,'Gewicht: '||w.weight_kg::text||' kg',w.id FROM weight_entries w JOIN pet_scope p ON p.household_id=w.household_id AND p.id=w.pet_id
	  UNION ALL SELECT 'lab_result',l.measured_on,l.test_name,l.id FROM lab_results l JOIN pet_scope p ON p.household_id=l.household_id AND p.id=l.pet_id
	  UNION ALL SELECT 'document',COALESCE(d.occurred_on,d.uploaded_at::date),d.title,d.id FROM documents d JOIN pet_scope p ON p.household_id=d.household_id AND p.id=d.pet_id
	  UNION ALL SELECT 'appointment',c.starts_at::date,c.title,c.id FROM calendar_events c JOIN pet_scope p ON p.household_id=c.household_id AND p.id=c.pet_id
	)
	SELECT type,event_date::text,title,id::text FROM timeline
	WHERE ($3::date IS NULL OR (event_date,type,id) < ($3::date,$4::text,$5::uuid))
	ORDER BY event_date DESC,type DESC,id DESC LIMIT $6`, userID, petID, cursorDate, cursorType, cursorID, limit+1)
	if err != nil {
		return TimelinePage{}, err
	}
	defer rows.Close()
	page := TimelinePage{Items: make([]TimelineItem, 0, limit)}
	for rows.Next() {
		var item TimelineItem
		if err := rows.Scan(&item.Type, &item.Date, &item.Title, &item.SourceID); err != nil {
			return TimelinePage{}, err
		}
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return TimelinePage{}, err
	}
	if len(page.Items) == 0 {
		if _, err := r.Get(ctx, userID, petID); err != nil {
			return TimelinePage{}, err
		}
	}
	if len(page.Items) > limit {
		last := page.Items[limit-1]
		page.NextCursor = encodeTimelineCursor(timelineCursor{Date: last.Date, Type: last.Type, SourceID: last.SourceID})
		page.Items = page.Items[:limit]
	}
	return page, nil
}

const medicalColumns = `m.id::text,m.pet_id::text,m.record_type,m.occurred_on::text,m.title,m.summary,m.veterinarian,m.details,m.created_at,m.updated_at`
const insertedMedicalColumns = `id::text,pet_id::text,record_type,occurred_on::text,title,summary,veterinarian,details,created_at,updated_at`

func scanMedicalRecord(row pgx.Row) (MedicalRecord, error) {
	var record MedicalRecord
	err := row.Scan(&record.ID, &record.PetID, &record.RecordType, &record.OccurredOn, &record.Title,
		&record.Summary, &record.Veterinarian, &record.Details, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return MedicalRecord{}, ErrNotFound
	}
	return record, err
}

func (r *PostgresRepository) CreateMedicalRecord(ctx context.Context, userID, createdBy string, record MedicalRecord) (MedicalRecord, error) {
	return outbox.Run(ctx, r.pool, func(tx pgx.Tx) (MedicalRecord, error) {
		created, err := scanMedicalRecord(tx.QueryRow(ctx, `INSERT INTO medical_records
	  (id,household_id,pet_id,record_type,occurred_on,title,summary,veterinarian,details,created_by,created_at,updated_at)
	  SELECT $3,p.household_id,p.id,$4,$5::date,$6,$7,$8,$9,$10,$11,$12
	  FROM pets p JOIN household_members hm ON hm.household_id=p.household_id
	  WHERE hm.user_id=$1 AND p.id=$2
	  RETURNING `+insertedMedicalColumns, userID, record.PetID, record.ID, record.RecordType,
			record.OccurredOn, record.Title, record.Summary, record.Veterinarian, record.Details,
			createdBy, record.CreatedAt, record.UpdatedAt))
		if err != nil {
			return MedicalRecord{}, err
		}
		return created, outbox.WriteForPet(ctx, tx, "medical_record.created", "medical_record", created.ID, created.PetID, nil)
	})
}

func (r *PostgresRepository) ListMedicalRecords(ctx context.Context, userID, petID string, limit int, cursor *medicalCursor) (MedicalRecordPage, error) {
	if _, err := r.Get(ctx, userID, petID); err != nil {
		return MedicalRecordPage{}, err
	}
	var cursorDate, cursorID any
	if cursor != nil {
		cursorDate, cursorID = cursor.OccurredOn, cursor.ID
	}
	rows, err := r.pool.Query(ctx, `SELECT `+medicalColumns+`
	  FROM medical_records m JOIN household_members hm ON hm.household_id=m.household_id
	  WHERE hm.user_id=$1 AND m.pet_id=$2
	    AND ($3::date IS NULL OR (m.occurred_on,m.id) < ($3::date,$4::uuid))
	  ORDER BY m.occurred_on DESC,m.id DESC LIMIT $5`, userID, petID, cursorDate, cursorID, limit+1)
	if err != nil {
		return MedicalRecordPage{}, err
	}
	defer rows.Close()
	page := MedicalRecordPage{Records: make([]MedicalRecord, 0, limit)}
	for rows.Next() {
		record, err := scanMedicalRecord(rows)
		if err != nil {
			return MedicalRecordPage{}, err
		}
		page.Records = append(page.Records, record)
	}
	if err := rows.Err(); err != nil {
		return MedicalRecordPage{}, err
	}
	if len(page.Records) > limit {
		last := page.Records[limit-1]
		page.NextCursor = encodeMedicalCursor(medicalCursor{OccurredOn: last.OccurredOn, ID: last.ID})
		page.Records = page.Records[:limit]
	}
	return page, nil
}

const vaccinationColumns = `v.id::text,v.pet_id::text,v.vaccine_name,v.manufacturer,v.diseases,v.administered_on::text,v.valid_until::text,v.next_due_date::text,v.document_id::text,v.veterinarian,v.batch_number,v.notes,v.created_at,v.updated_at`
const insertedVaccinationColumns = `id::text,pet_id::text,vaccine_name,manufacturer,diseases,administered_on::text,valid_until::text,next_due_date::text,document_id::text,veterinarian,batch_number,notes,created_at,updated_at`

func scanVaccination(row pgx.Row) (Vaccination, error) {
	var vaccination Vaccination
	err := row.Scan(&vaccination.ID, &vaccination.PetID, &vaccination.VaccineName, &vaccination.Manufacturer, &vaccination.Diseases, &vaccination.AdministeredOn,
		&vaccination.ValidUntil, &vaccination.NextDueDate, &vaccination.DocumentID, &vaccination.Veterinarian, &vaccination.BatchNumber, &vaccination.Notes,
		&vaccination.CreatedAt, &vaccination.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Vaccination{}, ErrNotFound
	}
	return vaccination, err
}

func (r *PostgresRepository) CreateVaccination(ctx context.Context, userID, createdBy string, vaccination Vaccination) (Vaccination, error) {
	return outbox.Run(ctx, r.pool, func(tx pgx.Tx) (Vaccination, error) {
		created, err := scanVaccination(tx.QueryRow(ctx, `INSERT INTO vaccinations
	  (id,household_id,pet_id,vaccine_name,manufacturer,diseases,administered_on,valid_until,next_due_date,document_id,veterinarian,batch_number,notes,created_by,created_at,updated_at)
	  SELECT $3,p.household_id,p.id,$4,$5,$6,$7::date,$8::date,$9::date,$10::uuid,$11,$12,$13,$14,$15,$16
	  FROM pets p JOIN household_members hm ON hm.household_id=p.household_id
	  WHERE hm.user_id=$1 AND p.id=$2
	    AND ($10::uuid IS NULL OR EXISTS(SELECT 1 FROM documents d WHERE d.household_id=p.household_id AND d.pet_id=p.id AND d.id=$10))
	  RETURNING `+insertedVaccinationColumns,
			userID, vaccination.PetID, vaccination.ID, vaccination.VaccineName, vaccination.Manufacturer, vaccination.Diseases, vaccination.AdministeredOn,
			vaccination.ValidUntil, vaccination.NextDueDate, vaccination.DocumentID, vaccination.Veterinarian, vaccination.BatchNumber, vaccination.Notes,
			createdBy, vaccination.CreatedAt, vaccination.UpdatedAt))
		if err != nil {
			return Vaccination{}, err
		}
		return created, outbox.WriteForPet(ctx, tx, "vaccination.recorded", "vaccination", created.ID, created.PetID, nil)
	})
}

func (r *PostgresRepository) ListVaccinations(ctx context.Context, userID, petID string, limit int, cursor *vaccinationCursor) (VaccinationPage, error) {
	if _, err := r.Get(ctx, userID, petID); err != nil {
		return VaccinationPage{}, err
	}
	var cursorDate, cursorID any
	if cursor != nil {
		cursorDate, cursorID = cursor.AdministeredOn, cursor.ID
	}
	rows, err := r.pool.Query(ctx, `SELECT `+vaccinationColumns+` FROM vaccinations v
	  JOIN household_members hm ON hm.household_id=v.household_id
	  WHERE hm.user_id=$1 AND v.pet_id=$2
	    AND ($3::date IS NULL OR (v.administered_on,v.id) < ($3::date,$4::uuid))
	  ORDER BY v.administered_on DESC,v.id DESC LIMIT $5`, userID, petID, cursorDate, cursorID, limit+1)
	if err != nil {
		return VaccinationPage{}, err
	}
	defer rows.Close()
	page := VaccinationPage{Vaccinations: make([]Vaccination, 0, limit)}
	for rows.Next() {
		vaccination, err := scanVaccination(rows)
		if err != nil {
			return VaccinationPage{}, err
		}
		page.Vaccinations = append(page.Vaccinations, vaccination)
	}
	if err := rows.Err(); err != nil {
		return VaccinationPage{}, err
	}
	if len(page.Vaccinations) > limit {
		last := page.Vaccinations[limit-1]
		page.NextCursor = encodeVaccinationCursor(vaccinationCursor{AdministeredOn: last.AdministeredOn, ID: last.ID})
		page.Vaccinations = page.Vaccinations[:limit]
	}
	return page, nil
}

const petMedicationColumns = `pm.id::text,pm.pet_id::text,pm.medication_id::text,m.name,m.active_ingredient,m.dosage_form,
      pm.dose_text,pm.frequency_text,pm.starts_on::text,pm.ends_on::text,pm.stock_quantity::text,
      pm.stock_unit,pm.package_size::text,pm.minimum_stock::text,pm.purchased_on::text,pm.expires_on::text,pm.notes,pm.active,pm.created_at,pm.updated_at`

func scanPetMedication(row pgx.Row) (PetMedication, error) {
	var medication PetMedication
	err := row.Scan(&medication.ID, &medication.PetID, &medication.MedicationID, &medication.Name,
		&medication.ActiveIngredient, &medication.DosageForm, &medication.DoseText, &medication.FrequencyText,
		&medication.StartsOn, &medication.EndsOn, &medication.StockQuantity, &medication.StockUnit,
		&medication.PackageSize, &medication.MinimumStock, &medication.PurchasedOn, &medication.ExpiresOn, &medication.Notes, &medication.Active, &medication.CreatedAt, &medication.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PetMedication{}, ErrNotFound
	}
	return medication, err
}

func (r *PostgresRepository) CreatePetMedication(ctx context.Context, userID, createdBy, medicationID string, medication PetMedication) (PetMedication, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return PetMedication{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var householdID string
	if err := tx.QueryRow(ctx, `SELECT p.household_id::text FROM pets p JOIN household_members hm ON hm.household_id=p.household_id WHERE hm.user_id=$1 AND p.id=$2`, userID, medication.PetID).Scan(&householdID); errors.Is(err, pgx.ErrNoRows) {
		return PetMedication{}, ErrNotFound
	} else if err != nil {
		return PetMedication{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO medications(id,household_id,name,active_ingredient,dosage_form) VALUES($1,$2,$3,$4,$5)`, medicationID, householdID, medication.Name, medication.ActiveIngredient, medication.DosageForm); err != nil {
		return PetMedication{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO pet_medications(id,household_id,pet_id,medication_id,dose_text,frequency_text,starts_on,ends_on,stock_quantity,stock_unit,package_size,minimum_stock,purchased_on,expires_on,notes,active,created_by,created_at,updated_at)
	 VALUES($1,$2,$3,$4,$5,$6,$7::date,$8::date,$9::numeric,$10,$11::numeric,$12::numeric,$13::date,$14::date,$15,$16,$17,$18,$19)`, medication.ID, householdID, medication.PetID, medicationID, medication.DoseText, medication.FrequencyText, medication.StartsOn, medication.EndsOn, medication.StockQuantity, medication.StockUnit, medication.PackageSize, medication.MinimumStock, medication.PurchasedOn, medication.ExpiresOn, medication.Notes, medication.Active, createdBy, medication.CreatedAt, medication.UpdatedAt); err != nil {
		return PetMedication{}, err
	}
	created, err := scanPetMedication(tx.QueryRow(ctx, `SELECT `+petMedicationColumns+` FROM pet_medications pm JOIN medications m ON m.id=pm.medication_id WHERE pm.id=$1`, medication.ID))
	if err != nil {
		return PetMedication{}, err
	}
	eventName := "medication.started"
	if !created.Active {
		eventName = "medication.stopped"
	}
	if err := outbox.WriteForPet(ctx, tx, eventName, "pet_medication", created.ID, created.PetID, nil); err != nil {
		return PetMedication{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PetMedication{}, err
	}
	return created, nil
}

func (r *PostgresRepository) ListPetMedications(ctx context.Context, userID, petID string, includeInactive bool, limit int) (PetMedicationPage, error) {
	if _, err := r.Get(ctx, userID, petID); err != nil {
		return PetMedicationPage{}, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+petMedicationColumns+` FROM pet_medications pm
	 JOIN medications m ON m.id=pm.medication_id JOIN household_members hm ON hm.household_id=pm.household_id
	 WHERE hm.user_id=$1 AND pm.pet_id=$2 AND ($3 OR pm.active) ORDER BY pm.starts_on DESC,pm.id DESC LIMIT $4`, userID, petID, includeInactive, limit)
	if err != nil {
		return PetMedicationPage{}, err
	}
	defer rows.Close()
	page := PetMedicationPage{Medications: make([]PetMedication, 0)}
	for rows.Next() {
		item, err := scanPetMedication(rows)
		if err != nil {
			return PetMedicationPage{}, err
		}
		page.Medications = append(page.Medications, item)
	}
	return page, rows.Err()
}

func (r *PostgresRepository) UpdateMedicationStock(ctx context.Context, userID, petID, medicationID, value string, at time.Time) (PetMedication, error) {
	return outbox.Run(ctx, r.pool, func(tx pgx.Tx) (PetMedication, error) {
		var wasLow bool
		err := tx.QueryRow(ctx, `SELECT pm.minimum_stock IS NOT NULL AND pm.stock_quantity<pm.minimum_stock FROM pet_medications pm JOIN household_members hm ON hm.household_id=pm.household_id WHERE hm.user_id=$1 AND pm.pet_id=$2 AND pm.id=$3 FOR UPDATE`, userID, petID, medicationID).Scan(&wasLow)
		if errors.Is(err, pgx.ErrNoRows) {
			return PetMedication{}, ErrNotFound
		}
		if err != nil {
			return PetMedication{}, err
		}
		updated, err := scanPetMedication(tx.QueryRow(ctx, `UPDATE pet_medications pm SET stock_quantity=$4::numeric,updated_at=$5 FROM medications m,household_members hm WHERE pm.id=$3 AND pm.pet_id=$2 AND m.id=pm.medication_id AND m.household_id=pm.household_id AND hm.user_id=$1 AND hm.household_id=pm.household_id RETURNING `+petMedicationColumns, userID, petID, medicationID, value, at))
		if err != nil {
			return PetMedication{}, err
		}
		if updated.MinimumStock != nil {
			var low bool
			if err := tx.QueryRow(ctx, `SELECT $1::numeric < $2::numeric`, updated.StockQuantity, *updated.MinimumStock).Scan(&low); err != nil {
				return PetMedication{}, err
			}
			if !wasLow && low {
				err = outbox.WriteForPet(ctx, tx, "medication.stock.low", "pet_medication", updated.ID, updated.PetID, nil)
			}
		}
		return updated, err
	})
}

func (r *PostgresRepository) CreateWeight(ctx context.Context, userID, createdBy string, e WeightEntry) (WeightEntry, error) {
	return outbox.Run(ctx, r.pool, func(tx pgx.Tx) (WeightEntry, error) {
		err := tx.QueryRow(ctx, `INSERT INTO weight_entries(id,household_id,pet_id,measured_on,weight_kg,notes,created_by,created_at) SELECT $3,p.household_id,p.id,$4::date,$5::numeric,$6,$7,$8 FROM pets p JOIN household_members hm ON hm.household_id=p.household_id WHERE hm.user_id=$1 AND p.id=$2 RETURNING id::text,pet_id::text,measured_on::text,weight_kg::text,notes,created_at`, userID, e.PetID, e.ID, e.MeasuredOn, e.WeightKg, e.Notes, createdBy, e.CreatedAt).Scan(&e.ID, &e.PetID, &e.MeasuredOn, &e.WeightKg, &e.Notes, &e.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return WeightEntry{}, ErrNotFound
		}
		if err != nil {
			return WeightEntry{}, err
		}
		return e, outbox.WriteForPet(ctx, tx, "pet.weight.recorded", "weight_entry", e.ID, e.PetID, nil)
	})
}
func (r *PostgresRepository) ListWeights(ctx context.Context, userID, petID string, limit int) ([]WeightEntry, error) {
	if _, err := r.Get(ctx, userID, petID); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT w.id::text,w.pet_id::text,w.measured_on::text,w.weight_kg::text,w.notes,w.created_at FROM weight_entries w JOIN household_members hm ON hm.household_id=w.household_id WHERE hm.user_id=$1 AND w.pet_id=$2 ORDER BY w.measured_on DESC,w.id DESC LIMIT $3`, userID, petID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]WeightEntry, 0)
	for rows.Next() {
		var e WeightEntry
		if err := rows.Scan(&e.ID, &e.PetID, &e.MeasuredOn, &e.WeightKg, &e.Notes, &e.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
func (r *PostgresRepository) CreateLabResult(ctx context.Context, userID, createdBy string, e LabResult) (LabResult, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO lab_results(id,household_id,pet_id,measured_on,test_name,value,unit,reference_low,reference_high,notes,created_by,created_at) SELECT $3,p.household_id,p.id,$4::date,$5,$6::numeric,$7,$8::numeric,$9::numeric,$10,$11,$12 FROM pets p JOIN household_members hm ON hm.household_id=p.household_id WHERE hm.user_id=$1 AND p.id=$2 RETURNING id::text,pet_id::text,measured_on::text,test_name,value::text,unit,reference_low::text,reference_high::text,notes,created_at`, userID, e.PetID, e.ID, e.MeasuredOn, e.TestName, e.Value, e.Unit, e.ReferenceLow, e.ReferenceHigh, e.Notes, createdBy, e.CreatedAt).Scan(&e.ID, &e.PetID, &e.MeasuredOn, &e.TestName, &e.Value, &e.Unit, &e.ReferenceLow, &e.ReferenceHigh, &e.Notes, &e.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return LabResult{}, ErrNotFound
	}
	return e, err
}
func (r *PostgresRepository) ListLabResults(ctx context.Context, userID, petID string, limit int) ([]LabResult, error) {
	if _, err := r.Get(ctx, userID, petID); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT l.id::text,l.pet_id::text,l.measured_on::text,l.test_name,l.value::text,l.unit,l.reference_low::text,l.reference_high::text,l.notes,l.created_at FROM lab_results l JOIN household_members hm ON hm.household_id=l.household_id WHERE hm.user_id=$1 AND l.pet_id=$2 ORDER BY l.measured_on DESC,l.id DESC LIMIT $3`, userID, petID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]LabResult, 0)
	for rows.Next() {
		var e LabResult
		if err := rows.Scan(&e.ID, &e.PetID, &e.MeasuredOn, &e.TestName, &e.Value, &e.Unit, &e.ReferenceLow, &e.ReferenceHigh, &e.Notes, &e.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) Dashboard(ctx context.Context, userID, petID, month string, now time.Time) (Dashboard, error) {
	var d Dashboard
	d.Month = month
	err := r.pool.QueryRow(ctx, `SELECT p.id::text,p.name,
 (SELECT count(*) FROM medical_records m WHERE m.household_id=p.household_id AND m.pet_id=p.id),
 (SELECT count(*) FROM documents x WHERE x.household_id=p.household_id AND x.pet_id=p.id),
 (SELECT count(*) FROM pet_medications pm WHERE pm.household_id=p.household_id AND pm.pet_id=p.id AND pm.active),
 (SELECT count(*) FROM shopping_item_pets sip JOIN shopping_items si ON si.id=sip.item_id AND si.household_id=sip.household_id WHERE sip.household_id=p.household_id AND sip.pet_id=p.id AND NOT si.completed),
 w.weight_kg::text,w.measured_on::text,v.vaccine_name,v.next_due_date::text,c.title,c.starts_at,
	 COALESCE((SELECT sum(b.amount_cents) FROM pet_budgets b WHERE b.household_id=p.household_id AND b.pet_id=p.id AND b.month >= ($3||'-01')::date AND b.month < (($3||'-01')::date + interval '1 month')),0),
	 COALESCE((SELECT sum(e.planned_amount_cents) FROM expenses e WHERE e.household_id=p.household_id AND e.pet_id=p.id AND e.due_date >= ($3||'-01')::date AND e.due_date < (($3||'-01')::date + interval '1 month')),0),
	 COALESCE((SELECT sum(e.actual_amount_cents) FROM expenses e WHERE e.household_id=p.household_id AND e.pet_id=p.id AND e.status='paid' AND e.due_date >= ($3||'-01')::date AND e.due_date < (($3||'-01')::date + interval '1 month')),0)
 FROM pets p JOIN household_members hm ON hm.household_id=p.household_id
 LEFT JOIN LATERAL(SELECT weight_kg,measured_on FROM weight_entries WHERE household_id=p.household_id AND pet_id=p.id ORDER BY measured_on DESC,id DESC LIMIT 1)w ON true
 LEFT JOIN LATERAL(SELECT vaccine_name,next_due_date FROM vaccinations WHERE household_id=p.household_id AND pet_id=p.id AND next_due_date >= $4::date ORDER BY next_due_date,id LIMIT 1)v ON true
 LEFT JOIN LATERAL(SELECT title,starts_at FROM calendar_events WHERE household_id=p.household_id AND pet_id=p.id AND status='planned' AND starts_at >= $4 AND (visibility='shared' OR owner_id=$1) ORDER BY starts_at,id LIMIT 1)c ON true
 WHERE hm.user_id=$1 AND p.id=$2`, userID, petID, month, now).Scan(&d.PetID, &d.PetName, &d.MedicalRecordCount, &d.DocumentCount, &d.ActiveMedicationCount, &d.OpenShoppingItemCount, &d.LatestWeightKg, &d.LatestWeightDate, &d.NextVaccinationName, &d.NextVaccinationDate, &d.NextAppointmentTitle, &d.NextAppointmentAt, &d.BudgetAmountCents, &d.PlannedAmountCents, &d.ActualAmountCents)
	if errors.Is(err, pgx.ErrNoRows) {
		return Dashboard{}, ErrNotFound
	}
	return d, err
}
