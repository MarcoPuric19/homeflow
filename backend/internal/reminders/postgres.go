package reminders

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}
func (r *PostgresRepository) List(ctx context.Context, userID string, from, to time.Time, limit int) ([]Reminder, error) {
	rows, err := r.pool.Query(ctx, `SELECT kind,source_id::text,pet_id::text,title,due_date::text,(due_date-$2::date)::int days_until FROM (
 SELECT 'vaccination' kind,v.id source_id,v.pet_id,('Impfung fällig: '||v.vaccine_name) title,v.next_due_date due_date
 FROM vaccinations v JOIN household_members hm ON hm.household_id=v.household_id WHERE hm.user_id=$1 AND v.next_due_date BETWEEN $2::date AND $3::date
 UNION ALL
 SELECT 'appointment',c.id,c.pet_id,c.title,c.starts_at::date FROM calendar_events c JOIN household_members hm ON hm.household_id=c.household_id
 WHERE hm.user_id=$1 AND c.pet_id IS NOT NULL AND c.status='planned' AND (c.visibility='shared' OR c.owner_id=$1) AND c.starts_at::date BETWEEN $2::date AND $3::date
 ) due ORDER BY due_date,kind,source_id LIMIT $4`, userID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Reminder, 0)
	for rows.Next() {
		var item Reminder
		var sourceID string
		if err := rows.Scan(&item.Kind, &sourceID, &item.PetID, &item.Title, &item.DueDate, &item.DaysUntil); err != nil {
			return nil, err
		}
		item.ID = item.Kind + ":" + sourceID
		result = append(result, item)
	}
	return result, rows.Err()
}
