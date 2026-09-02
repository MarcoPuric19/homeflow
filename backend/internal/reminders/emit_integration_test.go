package reminders

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"homeflow/backend/internal/postgres"
)

// reminderTestPool refuses any database whose name does not end in _test. The
// suite creates and drops rows; pointing it at a real database once would be
// one time too many.
func reminderTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("HOMEFLOW_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("HOMEFLOW_TEST_DATABASE_URL ist nicht gesetzt")
	}
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(config.ConnConfig.Database, "_test") {
		t.Fatalf("Datenbankname %q endet nicht auf _test", config.ConnConfig.Database)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := postgres.Migrate(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

// TestEmitDueIsIdempotentAndSkipsUndatedVaccinations covers the two properties
// the ticket turns on. The first is why the emitter exists at all: a reminder
// that is only derived lives for exactly one day, so it has to be written down.
// The second is the product rule that must not be softened — HomeFlow never
// invents a due date, so a vaccination without one produces nothing.
func TestEmitDueIsIdempotentAndSkipsUndatedVaccinations(t *testing.T) {
	ctx := context.Background()
	pool := reminderTestPool(t)

	var householdID, petID, userID string
	// pets und vaccinations verlangen created_by; die Zwei-Benutzer-Grenze aus
	// Migration 000002 wird nicht angetastet - ein Benutzer reicht.
	if err := pool.QueryRow(ctx, `INSERT INTO users(id,username,display_name,password_hash,role)
		VALUES(gen_random_uuid(),'erinnerungstest','Erinnerungstest','x','member')
		RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})
	err := pool.QueryRow(ctx, `INSERT INTO households(id,name) VALUES(gen_random_uuid(),'Erinnerungstest')
		RETURNING id::text`).Scan(&householdID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM households WHERE id=$1`, householdID)
	})
	if err := pool.QueryRow(ctx, `INSERT INTO pets(id,household_id,name,species,created_by)
		VALUES(gen_random_uuid(),$1,'Mila','cat',$2) RETURNING id::text`, householdID, userID).Scan(&petID); err != nil {
		t.Fatal(err)
	}

	heute := time.Now().Truncate(24 * time.Hour)
	faellig := heute.AddDate(0, 0, 30) // trifft die Standardregel 30 Tage

	// Eine Impfung MIT Fälligkeitsdatum und eine OHNE.
	if _, err := pool.Exec(ctx, `INSERT INTO vaccinations(id,household_id,pet_id,vaccine_name,administered_on,next_due_date,created_by)
		VALUES(gen_random_uuid(),$1,$2,'RCP',$3::date,$4::date,$5)`,
		householdID, petID, heute.AddDate(-1, 0, 0), faellig, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO vaccinations(id,household_id,pet_id,vaccine_name,administered_on,next_due_date,created_by)
		VALUES(gen_random_uuid(),$1,$2,'Tollwut',$3::date,NULL,$4)`,
		householdID, petID, heute.AddDate(-1, 0, 0), userID); err != nil {
		t.Fatal(err)
	}

	emitter := NewEmitter(pool)
	written, err := emitter.EmitDue(ctx, heute)
	if err != nil {
		t.Fatal(err)
	}
	if written != 1 {
		t.Fatalf("geschrieben=%d, erwartet 1 - die Impfung ohne Datum darf nichts erzeugen", written)
	}

	// Zweiter Lauf: derselbe Tag, dieselbe Lage. Nichts Neues.
	again, err := emitter.EmitDue(ctx, heute)
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Fatalf("zweiter Lauf schrieb %d Ereignisse - die Idempotenz greift nicht", again)
	}

	var events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM event_outbox WHERE event_name='vaccination.due'
		AND payload->>'householdId'=$1`, householdID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("event_outbox enthaelt %d Ereignisse, erwartet 1", events)
	}
}
