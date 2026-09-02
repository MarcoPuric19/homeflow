package reminders

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"homeflow/backend/internal/outbox"
)

// A reminder is derived, not stored: List joins vaccinations and calendar
// events against the household's lead days. That is the right shape for a
// query, and it has one flaw that only shows in use — the join binds
//
//	rr.lead_days = (due_date - today)
//
// so a reminder exists on exactly one day per lead value. Nobody opens the app
// that day, nobody ever sees it. Thirty, seven and one day before an
// appointment are three windows of twenty-four hours each, and missing all
// three is not unlikely for a household of two.
//
// Emit closes that gap by writing the due reminders into the outbox, where they
// stay until something has handled them. It does not replace the live query;
// the query still answers "what is due right now", the outbox answers "what
// became due while nobody was looking".
type Emitter struct {
	pool *pgxpool.Pool
}

func NewEmitter(pool *pgxpool.Pool) *Emitter { return &Emitter{pool: pool} }

type dueReminder struct {
	householdID string
	kind        string
	sourceID    string
	petID       string
	title       string
	dueDate     string
	leadDays    int
}

// EmitDue writes one event per due reminder and returns how many were new.
// Running it twice for the same day writes nothing the second time: the
// uniqueness lives in workflow_effects' primary key, not in a check this code
// performs. A guard in code is a guard that a second process walks straight
// past.
func (e *Emitter) EmitDue(ctx context.Context, on time.Time) (int, error) {
	due, err := e.due(ctx, on)
	if err != nil {
		return 0, err
	}
	written := 0
	for _, item := range due {
		item := item
		created, err := outbox.Run(ctx, e.pool, func(tx pgx.Tx) (bool, error) {
			return e.emitOne(ctx, tx, item)
		})
		if err != nil {
			return written, fmt.Errorf("erinnerung %s/%s: %w", item.kind, item.sourceID, err)
		}
		if created {
			written++
		}
	}
	return written, nil
}

// emitOne claims the reminder and writes the event in the same transaction.
// The claim is an INSERT that either succeeds or hits the primary key; there is
// no read-then-write, so two runs at the same moment cannot both decide the
// event is new.
func (e *Emitter) emitOne(ctx context.Context, tx pgx.Tx, item dueReminder) (bool, error) {
	var target string
	err := tx.QueryRow(ctx, `
INSERT INTO workflow_effects(household_id, source_kind, source_id, target_kind, target_id)
VALUES ($1, $2, $3, 'reminder_event', gen_random_uuid())
ON CONFLICT (household_id, source_kind, source_id, target_kind) DO NOTHING
RETURNING target_id::text`, item.householdID, item.effectKind(), item.sourceID).Scan(&target)
	if errors.Is(err, pgx.ErrNoRows) {
		// Schon geschrieben. Das ist der Normalfall bei jedem Lauf nach dem
		// ersten und kein Fehler.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, outbox.WriteForPet(ctx, tx, item.eventName(), item.kind, item.sourceID, item.petID,
		map[string]any{
			"titel":     item.title,
			"faelligAm": item.dueDate,
			"vorlauf":   item.leadDays,
		})
}

// due asks the same question List asks, but for one day and with the household
// attached — the event needs it, a user-facing list does not.
func (e *Emitter) due(ctx context.Context, on time.Time) ([]dueReminder, error) {
	rows, err := e.pool.Query(ctx, `
SELECT household_id::text, kind, source_id::text, pet_id::text, title, due_date::text, lead_days FROM (
  SELECT v.household_id, 'vaccination' kind, v.id source_id, v.pet_id,
         ('Impfung fällig: '||v.vaccine_name) title, v.next_due_date due_date, rr.lead_days
    FROM vaccinations v
    JOIN household_reminder_rules rr
      ON rr.household_id = v.household_id AND rr.lead_days = (v.next_due_date - $1::date)
   WHERE v.next_due_date IS NOT NULL
  UNION ALL
  SELECT c.household_id, 'appointment', c.id, c.pet_id, c.title, c.starts_at::date, rr.lead_days
    FROM calendar_events c
    JOIN household_reminder_rules rr
      ON rr.household_id = c.household_id AND rr.lead_days = (c.starts_at::date - $1::date)
   WHERE c.pet_id IS NOT NULL AND c.status = 'planned'
) faellig ORDER BY due_date, lead_days, kind, source_id`, on)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []dueReminder{}
	for rows.Next() {
		var item dueReminder
		if err := rows.Scan(&item.householdID, &item.kind, &item.sourceID, &item.petID,
			&item.title, &item.dueDate, &item.leadDays); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// eventName keeps the two names in one place; a typo here would produce an
// event nothing listens for, and nothing would fail.
func (d dueReminder) eventName() string {
	if d.kind == "appointment" {
		return "appointment.upcoming"
	}
	return "vaccination.due"
}

// effectKind carries the lead days because workflow_effects.source_id is a
// uuid and cannot hold "id:30". Thirty days before and seven days before are
// two different reminders about the same vaccination.
func (d dueReminder) effectKind() string {
	return d.kind + "_due_" + strconv.Itoa(d.leadDays)
}
