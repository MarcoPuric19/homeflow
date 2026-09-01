package receipts

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

func (r *PostgresRepository) Create(ctx context.Context, receipt Receipt) (Receipt, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO receipts
      (id,expense_id,merchant,receipt_date,total_amount_cents,currency,category,original_filename,mime_type,sha256,storage_key,uploaded_at,uploaded_by)
      VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
      RETURNING uploaded_at`, receipt.ID, receipt.ExpenseID, receipt.Merchant, receipt.ReceiptDate,
		receipt.TotalAmountCents, receipt.Currency, receipt.Category, receipt.OriginalFilename,
		receipt.MIMEType, receipt.SHA256, receipt.StorageKey, receipt.UploadedAt, receipt.UploadedBy).Scan(&receipt.UploadedAt)
	return receipt, err
}

func (r *PostgresRepository) ListForExpense(ctx context.Context, expenseID string) ([]Receipt, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text,expense_id::text,merchant,receipt_date::text,total_amount_cents,
      currency,category,original_filename,mime_type,sha256,storage_key,uploaded_at,uploaded_by::text
      FROM receipts WHERE expense_id=$1 ORDER BY uploaded_at`, expenseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Receipt, 0)
	for rows.Next() {
		var receipt Receipt
		if err := rows.Scan(&receipt.ID, &receipt.ExpenseID, &receipt.Merchant, &receipt.ReceiptDate,
			&receipt.TotalAmountCents, &receipt.Currency, &receipt.Category, &receipt.OriginalFilename,
			&receipt.MIMEType, &receipt.SHA256, &receipt.StorageKey, &receipt.UploadedAt, &receipt.UploadedBy); err != nil {
			return nil, err
		}
		result = append(result, receipt)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) GetForExpense(ctx context.Context, expenseID, receiptID string) (Receipt, error) {
	var receipt Receipt
	err := r.pool.QueryRow(ctx, `SELECT id::text,expense_id::text,merchant,receipt_date::text,total_amount_cents,
      currency,category,original_filename,mime_type,sha256,storage_key,uploaded_at,uploaded_by::text
      FROM receipts WHERE id=$1 AND expense_id=$2`, receiptID, expenseID).Scan(&receipt.ID, &receipt.ExpenseID,
		&receipt.Merchant, &receipt.ReceiptDate, &receipt.TotalAmountCents, &receipt.Currency, &receipt.Category,
		&receipt.OriginalFilename, &receipt.MIMEType, &receipt.SHA256, &receipt.StorageKey, &receipt.UploadedAt,
		&receipt.UploadedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return Receipt{}, ErrNotFound
	}
	return receipt, err
}
