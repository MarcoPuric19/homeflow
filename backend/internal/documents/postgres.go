package documents

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

const columns = `d.id::text,d.pet_id::text,d.title,d.document_type,d.occurred_on::text,d.notes,d.original_filename,d.mime_type,d.sha256,d.storage_key,d.uploaded_by::text,d.uploaded_at`

func scan(row pgx.Row) (Document, error) {
	var d Document
	err := row.Scan(&d.ID, &d.PetID, &d.Title, &d.DocumentType, &d.OccurredOn, &d.Notes, &d.OriginalFilename, &d.MIMEType, &d.SHA256, &d.StorageKey, &d.UploadedBy, &d.UploadedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	return d, err
}

func (r *PostgresRepository) Create(ctx context.Context, userID string, d Document) (Document, error) {
	return scan(r.pool.QueryRow(ctx, `INSERT INTO documents AS d(id,household_id,pet_id,title,document_type,occurred_on,notes,original_filename,mime_type,sha256,storage_key,uploaded_by,uploaded_at)
 SELECT $3,p.household_id,p.id,$4,$5,$6::date,$7,$8,$9,$10,$11,$12,$13 FROM pets p JOIN household_members hm ON hm.household_id=p.household_id WHERE hm.user_id=$1 AND p.id=$2 RETURNING `+columns, userID, d.PetID, d.ID, d.Title, d.DocumentType, d.OccurredOn, d.Notes, d.OriginalFilename, d.MIMEType, d.SHA256, d.StorageKey, d.UploadedBy, d.UploadedAt))
}

func (r *PostgresRepository) List(ctx context.Context, userID, petID string, limit int) ([]Document, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pets p JOIN household_members hm ON hm.household_id=p.household_id WHERE hm.user_id=$1 AND p.id=$2)`, userID, petID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `SELECT `+columns+` FROM documents d JOIN household_members hm ON hm.household_id=d.household_id WHERE hm.user_id=$1 AND d.pet_id=$2 ORDER BY d.occurred_on DESC NULLS LAST,d.id DESC LIMIT $3`, userID, petID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Document, 0)
	for rows.Next() {
		d, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) Get(ctx context.Context, userID, petID, documentID string) (Document, error) {
	return scan(r.pool.QueryRow(ctx, `SELECT `+columns+` FROM documents d JOIN household_members hm ON hm.household_id=d.household_id WHERE hm.user_id=$1 AND d.pet_id=$2 AND d.id=$3`, userID, petID, documentID))
}
