package calendar

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"homeflow/backend/internal/outbox"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// visibleToCaller is the authorisation rule of this module, expressed once and
// reused: a shared event belongs to the household, a private event only to its
// owner. It is part of every statement that reads or writes an existing row.
const visibleToCaller = `(e.visibility = 'shared' OR e.owner_id = $1)`

const eventColumns = `e.id::text,e.title,e.description,e.starts_at,e.ends_at,e.all_day,e.category,e.owner_id::text,
      e.visibility,COALESCE(e.recurrence_rule,''),e.reminder_minutes,e.status,e.pet_id::text,e.appointment_type,e.created_at,e.updated_at`
const returnedEventColumns = `id::text,title,description,starts_at,ends_at,all_day,category,owner_id::text,
      visibility,COALESCE(recurrence_rule,''),reminder_minutes,status,pet_id::text,appointment_type,created_at,updated_at`

func scanEvent(row pgx.Row) (Event, error) {
	var event Event
	err := row.Scan(&event.ID, &event.Title, &event.Description, &event.StartsAt, &event.EndsAt,
		&event.AllDay, &event.Category, &event.OwnerID, &event.Visibility, &event.RecurrenceRule,
		&event.ReminderMinutes, &event.Status, &event.PetID, &event.AppointmentType, &event.CreatedAt, &event.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, ErrNotFound
	}
	return event, err
}

func (r *PostgresRepository) Create(ctx context.Context, event Event) (Event, error) {
	return outbox.Run(ctx, r.pool, func(tx pgx.Tx) (Event, error) {
		created, err := scanEvent(tx.QueryRow(ctx, `INSERT INTO calendar_events
      (id,household_id,title,description,starts_at,ends_at,all_day,category,owner_id,visibility,recurrence_rule,reminder_minutes,status,pet_id,appointment_type,created_at,updated_at)
      SELECT $1,hm.household_id,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),$11,$12,$13,$14,$15,$16
      FROM household_members hm WHERE hm.user_id=$8
        AND ($13::uuid IS NULL OR EXISTS (SELECT 1 FROM pets p WHERE p.id=$13 AND p.household_id=hm.household_id))
      RETURNING `+returnedEventColumns,
			event.ID, event.Title, event.Description, event.StartsAt, event.EndsAt, event.AllDay,
			event.Category, event.OwnerID, event.Visibility, event.RecurrenceRule, event.ReminderMinutes,
			event.Status, event.PetID, event.AppointmentType, event.CreatedAt, event.UpdatedAt))
		if err != nil {
			return Event{}, err
		}
		err = outbox.WriteForPet(ctx, tx, "appointment.created", "calendar_event", created.ID, created.PetID, nil)
		return created, err
	})
}

// ListVisible returns events that overlap the window, not only those starting
// inside it: an event running across the window boundary still belongs on the
// screen.
func (r *PostgresRepository) ListVisible(ctx context.Context, userID string, window Range) ([]Event, error) {
	var petID any
	if window.PetID != "" {
		petID = window.PetID
	}
	rows, err := r.pool.Query(ctx, `SELECT `+eventColumns+`
      FROM calendar_events e JOIN household_members hm ON hm.household_id=e.household_id
      WHERE hm.user_id=$1 AND `+visibleToCaller+` AND e.starts_at < $3 AND e.ends_at >= $2
        AND ($4::uuid IS NULL OR e.pet_id=$4)
      ORDER BY e.starts_at,e.id`, userID, window.From, window.To, petID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]Event, 0)
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (r *PostgresRepository) GetVisible(ctx context.Context, userID, eventID string) (Event, error) {
	return scanEvent(r.pool.QueryRow(ctx, `SELECT `+eventColumns+`
      FROM calendar_events e JOIN household_members hm ON hm.household_id=e.household_id
      WHERE hm.user_id=$1 AND e.id=$2 AND `+visibleToCaller, userID, eventID))
}

// Update never touches owner_id or created_at. A row the caller may not see is
// simply not matched, so a foreign private event is indistinguishable from a
// missing one.
func (r *PostgresRepository) Update(ctx context.Context, userID string, event Event) (Event, error) {
	return outbox.Run(ctx, r.pool, func(tx pgx.Tx) (Event, error) {
		var previous Status
		err := tx.QueryRow(ctx, `SELECT e.status FROM calendar_events e JOIN household_members hm ON hm.household_id=e.household_id WHERE hm.user_id=$1 AND e.id=$2 AND `+visibleToCaller+` FOR UPDATE`, userID, event.ID).Scan(&previous)
		if errors.Is(err, pgx.ErrNoRows) {
			return Event{}, ErrNotFound
		}
		if err != nil {
			return Event{}, err
		}
		updated, err := scanEvent(tx.QueryRow(ctx, `UPDATE calendar_events e SET
        title=$3, description=$4, starts_at=$5, ends_at=$6, all_day=$7, category=$8,
        visibility=$9, recurrence_rule=NULLIF($10,''), reminder_minutes=$11, status=$12,
        pet_id=$13,appointment_type=$14,updated_at=$15
      FROM household_members hm
      WHERE e.id=$2 AND hm.user_id=$1 AND hm.household_id=e.household_id AND `+visibleToCaller+`
        AND ($13::uuid IS NULL OR EXISTS (SELECT 1 FROM pets p WHERE p.id=$13 AND p.household_id=e.household_id))
      RETURNING `+eventColumns,
			userID, event.ID, event.Title, event.Description, event.StartsAt, event.EndsAt, event.AllDay,
			event.Category, event.Visibility, event.RecurrenceRule, event.ReminderMinutes, event.Status,
			event.PetID, event.AppointmentType, event.UpdatedAt))
		if err != nil {
			return Event{}, err
		}
		if previous != StatusCompleted && updated.Status == StatusCompleted {
			err = outbox.WriteForPet(ctx, tx, "appointment.completed", "calendar_event", updated.ID, updated.PetID, nil)
		}
		return updated, err
	})
}

func (r *PostgresRepository) Delete(ctx context.Context, userID, eventID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM calendar_events e USING household_members hm
      WHERE e.id=$2 AND hm.user_id=$1 AND hm.household_id=e.household_id AND `+visibleToCaller, userID, eventID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
