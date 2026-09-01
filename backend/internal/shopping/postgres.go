package shopping

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

const SharedListID = "00000000-0000-4000-8000-000000000100"

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const itemColumns = `i.id::text,i.list_id::text,i.name,i.quantity,i.unit,i.category,i.note,i.priority,i.store,i.added_by::text,i.completed,COALESCE(i.purchased_by::text,''),i.completed_at,i.created_at,i.updated_at,COALESCE(ARRAY(SELECT sip.pet_id::text FROM shopping_item_pets sip WHERE sip.item_id=i.id ORDER BY sip.pet_id),'{}'::text[])`

func scanItem(row pgx.Row) (Item, error) {
	var x Item
	err := row.Scan(&x.ID, &x.ListID, &x.Name, &x.Quantity, &x.Unit, &x.Category, &x.Note, &x.Priority, &x.Store, &x.AddedBy, &x.Completed, &x.PurchasedBy, &x.CompletedAt, &x.CreatedAt, &x.UpdatedAt, &x.PetIDs)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	return x, err
}
func (r *PostgresRepository) Create(ctx context.Context, userID string, item Item) (Item, error) {
	listID := normalizeListID(item.ListID)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Item{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var householdID string
	err = tx.QueryRow(ctx, `SELECT l.household_id::text FROM shopping_lists l JOIN household_members hm ON hm.household_id=l.household_id WHERE hm.user_id=$1 AND l.id=$2 AND (l.visibility='shared' OR l.owner_id=$1)`, userID, listID).Scan(&householdID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}
	if len(item.PetIDs) > 0 {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM pets WHERE household_id=$1 AND id=ANY($2::uuid[])`, householdID, item.PetIDs).Scan(&count); err != nil {
			return Item{}, err
		}
		if count != len(item.PetIDs) {
			return Item{}, ErrInvalid
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO shopping_items(id,household_id,list_id,name,quantity,unit,category,note,priority,store,added_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, item.ID, householdID, listID, item.Name, item.Quantity, item.Unit, item.Category, item.Note, item.Priority, item.Store, item.AddedBy, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return Item{}, err
	}
	for _, petID := range item.PetIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO shopping_item_pets(household_id,item_id,pet_id) VALUES($1,$2,$3)`, householdID, item.ID, petID); err != nil {
			return Item{}, err
		}
	}
	created, err := scanItem(tx.QueryRow(ctx, `SELECT `+itemColumns+` FROM shopping_items i WHERE i.id=$1`, item.ID))
	if err != nil {
		return Item{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Item{}, err
	}
	return created, nil
}
func (r *PostgresRepository) List(ctx context.Context, userID, listID string, includeCompleted bool, petID string, limit int) ([]Item, error) {
	var pet any
	if petID != "" {
		pet = petID
	}
	rows, err := r.pool.Query(ctx, `SELECT `+itemColumns+` FROM shopping_items i JOIN shopping_lists l ON l.id=i.list_id JOIN household_members hm ON hm.household_id=i.household_id WHERE hm.user_id=$1 AND i.list_id=$2 AND (l.visibility='shared' OR l.owner_id=$1) AND ($3 OR NOT i.completed) AND ($4::uuid IS NULL OR EXISTS(SELECT 1 FROM shopping_item_pets sip WHERE sip.item_id=i.id AND sip.pet_id=$4)) ORDER BY i.created_at,i.id LIMIT $5`, userID, normalizeListID(listID), includeCompleted, pet, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Item, 0)
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (r *PostgresRepository) Lists(ctx context.Context, userID string) ([]List, error) {
	rows, err := r.pool.Query(ctx, `SELECT l.id::text,l.name,l.visibility FROM shopping_lists l JOIN household_members hm ON hm.household_id=l.household_id WHERE hm.user_id=$1 AND (l.visibility='shared' OR l.owner_id=$1) ORDER BY l.created_at,l.id LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lists := make([]List, 0)
	for rows.Next() {
		var list List
		if err := rows.Scan(&list.ID, &list.Name, &list.Visibility); err != nil {
			return nil, err
		}
		lists = append(lists, list)
	}
	return lists, rows.Err()
}
func (r *PostgresRepository) Complete(ctx context.Context, itemID, userID string, at time.Time) (Item, error) {
	item, err := scanItem(r.pool.QueryRow(ctx, `UPDATE shopping_items i SET completed=true,purchased_by=$2,completed_at=$3,updated_at=$3 FROM shopping_lists l,household_members hm WHERE i.id=$1 AND NOT i.completed AND l.id=i.list_id AND hm.user_id=$2 AND hm.household_id=i.household_id AND (l.visibility='shared' OR l.owner_id=$2) RETURNING `+itemColumns, itemID, userID, at))
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Item{}, err
	}
	var done bool
	err = r.pool.QueryRow(ctx, `SELECT i.completed FROM shopping_items i JOIN shopping_lists l ON l.id=i.list_id JOIN household_members hm ON hm.household_id=i.household_id WHERE hm.user_id=$2 AND i.id=$1 AND (l.visibility='shared' OR l.owner_id=$2)`, itemID, userID).Scan(&done)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}
	return Item{}, ErrAlreadyDone
}
func normalizeListID(v string) string {
	if v == "" || v == "shared" {
		return SharedListID
	}
	return v
}
