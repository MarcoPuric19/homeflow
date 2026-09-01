package reminders

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"homeflow/backend/internal/calendar"
	"homeflow/backend/internal/outbox"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}
func (r *PostgresRepository) List(ctx context.Context, userID string, from, to time.Time, limit int) ([]Reminder, error) {
	rows, err := r.pool.Query(ctx, `SELECT kind,source_id::text,pet_id::text,title,due_date::text,days_until,lead_days FROM (
 SELECT 'vaccination' kind,v.id source_id,v.pet_id,('Impfung fällig: '||v.vaccine_name) title,v.next_due_date due_date
	,(v.next_due_date-$2::date)::int days_until,rr.lead_days
 FROM vaccinations v JOIN household_members hm ON hm.household_id=v.household_id JOIN household_reminder_rules rr ON rr.household_id=v.household_id AND rr.lead_days=(v.next_due_date-$2::date) WHERE hm.user_id=$1 AND v.next_due_date BETWEEN $2::date AND $3::date
 UNION ALL
 SELECT 'appointment',c.id,c.pet_id,c.title,c.starts_at::date,(c.starts_at::date-$2::date)::int,rr.lead_days FROM calendar_events c JOIN household_members hm ON hm.household_id=c.household_id JOIN household_reminder_rules rr ON rr.household_id=c.household_id AND rr.lead_days=(c.starts_at::date-$2::date)
 WHERE hm.user_id=$1 AND c.pet_id IS NOT NULL AND c.status='planned' AND (c.visibility='shared' OR c.owner_id=$1) AND c.starts_at::date BETWEEN $2::date AND $3::date
 ) due ORDER BY due_date,lead_days,kind,source_id LIMIT $4`, userID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Reminder, 0)
	for rows.Next() {
		var item Reminder
		var sourceID string
		if err := rows.Scan(&item.Kind, &sourceID, &item.PetID, &item.Title, &item.DueDate, &item.DaysUntil, &item.LeadDays); err != nil {
			return nil, err
		}
		item.SourceID = sourceID
		item.ID = item.Kind + ":" + sourceID + ":" + item.DueDate + ":" + strconv.Itoa(item.LeadDays)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) CreateAppointment(ctx context.Context, userID, sourceID string, event calendar.Event) (calendar.Event, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return calendar.Event{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var householdID, petID string
	err = tx.QueryRow(ctx, `SELECT v.household_id::text,v.pet_id::text FROM vaccinations v JOIN household_members hm ON hm.household_id=v.household_id WHERE hm.user_id=$1 AND v.id=$2`, userID, sourceID).Scan(&householdID, &petID)
	if errors.Is(err, pgx.ErrNoRows) {
		return calendar.Event{}, ErrNotFound
	}
	if err != nil {
		return calendar.Event{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO calendar_events(id,household_id,title,description,starts_at,ends_at,all_day,category,owner_id,visibility,status,pet_id,appointment_type,created_at,updated_at) VALUES($1,$2,$3,'',$4,$5,false,'Tiergesundheit',$6,$7,'planned',$8,'vaccination',$9,$10)`, event.ID, householdID, event.Title, event.StartsAt, event.EndsAt, userID, event.Visibility, petID, event.CreatedAt, event.UpdatedAt)
	if err != nil {
		return calendar.Event{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO reminder_appointments(household_id,source_kind,source_id,appointment_id,created_by) VALUES($1,'vaccination',$2,$3,$4)`, householdID, sourceID, event.ID, userID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return calendar.Event{}, ErrDuplicate
	}
	if err != nil {
		return calendar.Event{}, err
	}
	if err = outbox.WriteForPet(ctx, tx, "appointment.created", "calendar_event", event.ID, petID, nil); err != nil {
		return calendar.Event{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return calendar.Event{}, err
	}
	event.PetID = petID
	return event, nil
}

func (r *PostgresRepository) GetRules(ctx context.Context, userID string) (Rules, error) {
	rows, err := r.pool.Query(ctx, `SELECT rr.lead_days FROM household_reminder_rules rr JOIN household_members hm ON hm.household_id=rr.household_id WHERE hm.user_id=$1 ORDER BY rr.lead_days DESC`, userID)
	if err != nil {
		return Rules{}, err
	}
	defer rows.Close()
	rules := Rules{LeadDays: make([]int, 0)}
	for rows.Next() {
		var value int
		if err := rows.Scan(&value); err != nil {
			return Rules{}, err
		}
		rules.LeadDays = append(rules.LeadDays, value)
	}
	return rules, rows.Err()
}

func (r *PostgresRepository) ReplaceRules(ctx context.Context, userID string, values []int) (Rules, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Rules{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var householdID string
	if err := tx.QueryRow(ctx, `SELECT household_id::text FROM household_members WHERE user_id=$1`, userID).Scan(&householdID); err != nil {
		return Rules{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM household_reminder_rules WHERE household_id=$1`, householdID); err != nil {
		return Rules{}, err
	}
	for _, value := range values {
		if _, err = tx.Exec(ctx, `INSERT INTO household_reminder_rules(household_id,lead_days) VALUES($1,$2)`, householdID, value); err != nil {
			return Rules{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Rules{}, err
	}
	sort.Sort(sort.Reverse(sort.IntSlice(values)))
	return Rules{LeadDays: values}, nil
}
