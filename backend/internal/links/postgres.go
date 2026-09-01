package links

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetDocument(ctx context.Context, userID, petID, documentID string) (Set, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT true FROM documents d JOIN household_members hm ON hm.household_id=d.household_id WHERE hm.user_id=$1 AND d.pet_id=$2 AND d.id=$3`, userID, petID, documentID).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		return Set{}, ErrNotFound
	}
	if err != nil {
		return Set{}, err
	}
	return readSet(ctx, r.pool, "document", documentID)
}

func (r *PostgresRepository) GetExpense(ctx context.Context, userID, expenseID string) (Set, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT true FROM expenses e JOIN household_members hm ON hm.household_id=e.household_id WHERE hm.user_id=$1 AND e.id=$2 AND e.pet_id IS NOT NULL`, userID, expenseID).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		return Set{}, ErrNotFound
	}
	if err != nil {
		return Set{}, err
	}
	return readSet(ctx, r.pool, "expense", expenseID)
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readSet(ctx context.Context, q queryer, kind, sourceID string) (Set, error) {
	set := Set{MedicalRecordIDs: []string{}, AppointmentIDs: []string{}}
	medicalTable, appointmentTable, sourceColumn := "document_medical_records", "document_appointments", "document_id"
	if kind == "expense" {
		medicalTable, appointmentTable, sourceColumn = "expense_medical_records", "expense_appointments", "expense_id"
	}
	rows, err := q.Query(ctx, `SELECT medical_record_id::text FROM `+medicalTable+` WHERE `+sourceColumn+`=$1 ORDER BY medical_record_id`, sourceID)
	if err != nil {
		return Set{}, err
	}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			rows.Close()
			return Set{}, err
		}
		set.MedicalRecordIDs = append(set.MedicalRecordIDs, value)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return Set{}, err
	}
	rows.Close()
	rows, err = q.Query(ctx, `SELECT appointment_id::text FROM `+appointmentTable+` WHERE `+sourceColumn+`=$1 ORDER BY appointment_id`, sourceID)
	if err != nil {
		return Set{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return Set{}, err
		}
		set.AppointmentIDs = append(set.AppointmentIDs, value)
	}
	return set, rows.Err()
}

func (r *PostgresRepository) ReplaceDocument(ctx context.Context, userID, petID, documentID string, set Set) (Set, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Set{}, err
	}
	defer tx.Rollback(ctx)
	var householdID string
	if err = tx.QueryRow(ctx, `SELECT d.household_id::text FROM documents d JOIN household_members hm ON hm.household_id=d.household_id WHERE hm.user_id=$1 AND d.pet_id=$2 AND d.id=$3 FOR UPDATE`, userID, petID, documentID).Scan(&householdID); errors.Is(err, pgx.ErrNoRows) {
		return Set{}, ErrNotFound
	} else if err != nil {
		return Set{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM document_medical_records WHERE document_id=$1`, documentID); err != nil {
		return Set{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM document_appointments WHERE document_id=$1`, documentID); err != nil {
		return Set{}, err
	}
	for _, target := range set.MedicalRecordIDs {
		tag, e := tx.Exec(ctx, `INSERT INTO document_medical_records(household_id,pet_id,document_id,medical_record_id) SELECT $1,$2,$3,m.id FROM medical_records m WHERE m.household_id=$1 AND m.pet_id=$2 AND m.id=$4`, householdID, petID, documentID, target)
		if e != nil {
			return Set{}, e
		}
		if tag.RowsAffected() != 1 {
			return Set{}, ErrNotFound
		}
	}
	for _, target := range set.AppointmentIDs {
		tag, e := tx.Exec(ctx, `INSERT INTO document_appointments(household_id,pet_id,document_id,appointment_id) SELECT $1,$2,$3,c.id FROM calendar_events c WHERE c.household_id=$1 AND c.pet_id=$2 AND c.id=$4 AND (c.visibility='shared' OR c.owner_id=$5)`, householdID, petID, documentID, target, userID)
		if e != nil {
			return Set{}, e
		}
		if tag.RowsAffected() != 1 {
			return Set{}, ErrNotFound
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Set{}, err
	}
	return r.GetDocument(ctx, userID, petID, documentID)
}

func (r *PostgresRepository) ReplaceExpense(ctx context.Context, userID, expenseID string, set Set) (Set, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Set{}, err
	}
	defer tx.Rollback(ctx)
	var householdID, petID string
	if err = tx.QueryRow(ctx, `SELECT e.household_id::text,e.pet_id::text FROM expenses e JOIN household_members hm ON hm.household_id=e.household_id WHERE hm.user_id=$1 AND e.id=$2 AND e.pet_id IS NOT NULL FOR UPDATE`, userID, expenseID).Scan(&householdID, &petID); errors.Is(err, pgx.ErrNoRows) {
		return Set{}, ErrNotFound
	} else if err != nil {
		return Set{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM expense_medical_records WHERE expense_id=$1`, expenseID); err != nil {
		return Set{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM expense_appointments WHERE expense_id=$1`, expenseID); err != nil {
		return Set{}, err
	}
	for _, target := range set.MedicalRecordIDs {
		tag, e := tx.Exec(ctx, `INSERT INTO expense_medical_records(household_id,pet_id,expense_id,medical_record_id) SELECT $1,$2,$3,m.id FROM medical_records m WHERE m.household_id=$1 AND m.pet_id=$2 AND m.id=$4`, householdID, petID, expenseID, target)
		if e != nil {
			return Set{}, e
		}
		if tag.RowsAffected() != 1 {
			return Set{}, ErrNotFound
		}
	}
	for _, target := range set.AppointmentIDs {
		tag, e := tx.Exec(ctx, `INSERT INTO expense_appointments(household_id,pet_id,expense_id,appointment_id) SELECT $1,$2,$3,c.id FROM calendar_events c WHERE c.household_id=$1 AND c.pet_id=$2 AND c.id=$4 AND (c.visibility='shared' OR c.owner_id=$5)`, householdID, petID, expenseID, target, userID)
		if e != nil {
			return Set{}, e
		}
		if tag.RowsAffected() != 1 {
			return Set{}, ErrNotFound
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Set{}, err
	}
	return r.GetExpense(ctx, userID, expenseID)
}
