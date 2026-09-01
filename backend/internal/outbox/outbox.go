package outbox

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"homeflow/backend/internal/platform/id"
)

// Write appends one versioned event using the caller's transaction. Payloads
// are deliberately assembled by repositories and must contain no private
// free-text or document content.
func Write(ctx context.Context, tx pgx.Tx, name, aggregateType, aggregateID string, payload map[string]any) error {
	eventID, err := id.New()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO event_outbox(id,event_name,event_version,aggregate_type,aggregate_id,payload) VALUES($1,$2,1,$3,$4,$5)`, eventID, name, aggregateType, aggregateID, encoded)
	return err
}

func Run[T any](ctx context.Context, pool *pgxpool.Pool, work func(pgx.Tx) (T, error)) (T, error) {
	var zero T
	tx, err := pool.Begin(ctx)
	if err != nil {
		return zero, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := work(tx)
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, err
	}
	return result, nil
}

func WriteForPet(ctx context.Context, tx pgx.Tx, name, aggregateType, aggregateID, petID string, payload map[string]any) error {
	var householdID string
	if err := tx.QueryRow(ctx, `SELECT household_id::text FROM pets WHERE id=$1`, petID).Scan(&householdID); err != nil {
		return err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["householdId"] = householdID
	payload["petId"] = petID
	return Write(ctx, tx, name, aggregateType, aggregateID, payload)
}
