package vets

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const columns = `v.id::text,v.practice,v.vet_name,v.street,v.postal_code,v.city,v.phone,v.email,v.notes,v.created_at,v.updated_at`

func scan(row pgx.Row) (Veterinarian, error) {
	var v Veterinarian
	err := row.Scan(&v.ID, &v.Practice, &v.VetName, &v.Street, &v.PostalCode, &v.City, &v.Phone, &v.Email, &v.Notes, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Veterinarian{}, ErrNotFound
	}
	return v, err
}
func (r *PostgresRepository) Create(ctx context.Context, userID string, v Veterinarian) (Veterinarian, error) {
	return scan(r.pool.QueryRow(ctx, `INSERT INTO veterinarians(id,household_id,practice,vet_name,street,postal_code,city,phone,email,notes,created_by,created_at,updated_at) SELECT $2,hm.household_id,$3,$4,$5,$6,$7,$8,$9,$10,$1,$11,$12 FROM household_members hm WHERE hm.user_id=$1 RETURNING id::text,practice,vet_name,street,postal_code,city,phone,email,notes,created_at,updated_at`, userID, v.ID, v.Practice, v.VetName, v.Street, v.PostalCode, v.City, v.Phone, v.Email, v.Notes, v.CreatedAt, v.UpdatedAt))
}
func (r *PostgresRepository) List(ctx context.Context, userID, search string, limit int, after string) (Page, error) {
	var cursor any
	if after != "" {
		cursor = after
	}
	pattern := "%" + search + "%"
	rows, err := r.pool.Query(ctx, `SELECT `+columns+` FROM veterinarians v JOIN household_members hm ON hm.household_id=v.household_id WHERE hm.user_id=$1 AND ($2='' OR v.practice ILIKE $3 OR v.vet_name ILIKE $3) AND ($4::uuid IS NULL OR v.id>$4::uuid) ORDER BY v.id LIMIT $5`, userID, search, pattern, cursor, limit+1)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	page := Page{Veterinarians: make([]Veterinarian, 0, limit)}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return Page{}, err
		}
		page.Veterinarians = append(page.Veterinarians, v)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if len(page.Veterinarians) > limit {
		page.NextCursor = page.Veterinarians[limit-1].ID
		page.Veterinarians = page.Veterinarians[:limit]
	}
	return page, nil
}
func (r *PostgresRepository) Get(ctx context.Context, userID, vetID string) (Veterinarian, error) {
	return scan(r.pool.QueryRow(ctx, `SELECT `+columns+` FROM veterinarians v JOIN household_members hm ON hm.household_id=v.household_id WHERE hm.user_id=$1 AND v.id=$2`, userID, vetID))
}
func (r *PostgresRepository) Update(ctx context.Context, userID string, v Veterinarian) (Veterinarian, error) {
	return scan(r.pool.QueryRow(ctx, `UPDATE veterinarians v SET practice=$3,vet_name=$4,street=$5,postal_code=$6,city=$7,phone=$8,email=$9,notes=$10,updated_at=$11 FROM household_members hm WHERE hm.user_id=$1 AND v.id=$2 AND hm.household_id=v.household_id RETURNING `+columns, userID, v.ID, v.Practice, v.VetName, v.Street, v.PostalCode, v.City, v.Phone, v.Email, v.Notes, v.UpdatedAt))
}
func (r *PostgresRepository) Delete(ctx context.Context, userID, vetID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM veterinarians v USING household_members hm WHERE hm.user_id=$1 AND v.id=$2 AND hm.household_id=v.household_id AND NOT EXISTS (SELECT 1 FROM pets p WHERE p.household_id=v.household_id AND p.primary_vet_id=v.id)`, userID, vetID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return ErrReferenced
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var referenced bool
		err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM veterinarians v JOIN household_members hm ON hm.household_id=v.household_id JOIN pets p ON p.household_id=v.household_id AND p.primary_vet_id=v.id WHERE hm.user_id=$1 AND v.id=$2)`, userID, vetID).Scan(&referenced)
		if err != nil {
			return err
		}
		if referenced {
			return ErrReferenced
		}
		return ErrNotFound
	}
	return nil
}
func (r *PostgresRepository) Candidates(ctx context.Context, userID, postalCode string) ([]Veterinarian, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+columns+` FROM veterinarians v JOIN household_members hm ON hm.household_id=v.household_id WHERE hm.user_id=$1 AND ($2='' OR v.postal_code=$2) ORDER BY v.id LIMIT 100`, userID, postalCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Veterinarian{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
