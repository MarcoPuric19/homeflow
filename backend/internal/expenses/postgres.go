package expenses

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"homeflow/backend/internal/outbox"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const expenseColumns = `e.id::text,e.description,e.merchant,e.planned_amount_cents,e.actual_amount_cents,
e.currency,e.due_date::text,e.paid_at,e.category,COALESCE(e.recurrence_rule,''),e.status,e.user_id::text,
e.payment_method,e.notes,e.pet_id::text,e.budget_category,e.created_at,e.updated_at`

func scanExpense(row pgx.Row) (Expense, error) {
	var expense Expense
	err := row.Scan(&expense.ID, &expense.Description, &expense.Merchant, &expense.PlannedAmountCents,
		&expense.ActualAmountCents, &expense.Currency, &expense.DueDate, &expense.PaidAt,
		&expense.Category, &expense.RecurrenceRule, &expense.Status, &expense.UserID,
		&expense.PaymentMethod, &expense.Notes, &expense.PetID, &expense.BudgetCategory, &expense.CreatedAt, &expense.UpdatedAt)
	return expense, err
}

func (r *PostgresRepository) Create(ctx context.Context, userID string, expense Expense) (Expense, error) {
	return outbox.Run(ctx, r.pool, func(tx pgx.Tx) (Expense, error) {
		created, err := scanExpense(tx.QueryRow(ctx, `INSERT INTO expenses AS e
      (id,household_id,description,merchant,planned_amount_cents,currency,due_date,category,recurrence_rule,status,user_id,payment_method,notes,pet_id,budget_category,created_at,updated_at)
      SELECT $2,hm.household_id,$3,$4,$5,$6,$7,$8,NULLIF($9,''),$10,$11,$12,$13,$14,$15,$16,$17
      FROM household_members hm
      WHERE hm.user_id=$1 AND ($14::uuid IS NULL OR EXISTS(SELECT 1 FROM pets p WHERE p.id=$14 AND p.household_id=hm.household_id))
      RETURNING `+expenseColumns, userID, expense.ID, expense.Description, expense.Merchant,
			expense.PlannedAmountCents, expense.Currency, expense.DueDate, expense.Category,
			expense.RecurrenceRule, expense.Status, expense.UserID, expense.PaymentMethod, expense.Notes,
			expense.PetID, expense.BudgetCategory, expense.CreatedAt, expense.UpdatedAt))
		if errors.Is(err, pgx.ErrNoRows) {
			return Expense{}, ErrNotFound
		}
		if err != nil {
			return Expense{}, err
		}
		if created.PetID != nil {
			err = outbox.WriteForPet(ctx, tx, "pet_expense.created", "expense", created.ID, *created.PetID, nil)
		}
		return created, err
	})
}

func (r *PostgresRepository) List(ctx context.Context, userID, month, petID string, limit int) ([]Expense, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+expenseColumns+` FROM expenses e
	  JOIN household_members hm ON hm.household_id=e.household_id
	  WHERE hm.user_id=$1 AND ($2='' OR (e.due_date >= ($2||'-01')::date AND e.due_date < (($2||'-01')::date + interval '1 month')))
	  AND ($3::uuid IS NULL OR e.pet_id=$3::uuid) ORDER BY e.due_date,e.id LIMIT $4`, userID, month, nullableUUID(petID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Expense, 0)
	for rows.Next() {
		expense, err := scanExpense(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, expense)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) MarkPaid(ctx context.Context, expenseID, userID string, amount int64, paidAt time.Time) (Expense, error) {
	return outbox.Run(ctx, r.pool, func(tx pgx.Tx) (Expense, error) {
		expense, err := scanExpense(tx.QueryRow(ctx, `UPDATE expenses e SET actual_amount_cents=$2,paid_at=$3,status='paid',updated_at=$3
	  WHERE e.id=$1 AND e.status IN ('planned','open') AND e.household_id IN
	  (SELECT household_id FROM household_members WHERE user_id=$4) RETURNING `+expenseColumns, expenseID, amount, paidAt, userID))
		if err == nil {
			if expense.PetID != nil && expense.BudgetCategory != "" {
				var budgetID string
				var reachedBefore, reachedNow bool
				err = tx.QueryRow(ctx, `SELECT b.id::text,b.amount_cents-(COALESCE(sum(e.actual_amount_cents) FILTER (WHERE e.status='paid'),0)-$4) <= b.warning_threshold_cents,b.amount_cents-COALESCE(sum(e.actual_amount_cents) FILTER (WHERE e.status='paid'),0) <= b.warning_threshold_cents FROM pet_budgets b LEFT JOIN expenses e ON e.household_id=b.household_id AND e.pet_id=b.pet_id AND COALESCE(NULLIF(e.budget_category,''),NULLIF(e.category,''),'Unkategorisiert')=b.category AND date_trunc('month',e.due_date)=b.month WHERE b.pet_id=$1 AND b.month=date_trunc('month',$2::date) AND b.category=$3 AND b.warning_threshold_cents IS NOT NULL GROUP BY b.id,b.amount_cents,b.warning_threshold_cents`, *expense.PetID, expense.DueDate, expense.BudgetCategory, amount).Scan(&budgetID, &reachedBefore, &reachedNow)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return Expense{}, err
				}
				if err == nil && !reachedBefore && reachedNow {
					err = outbox.WriteForPet(ctx, tx, "pet_budget.threshold.reached", "pet_budget", budgetID, *expense.PetID, nil)
				}
			}
			return expense, err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Expense{}, err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM expenses e JOIN household_members hm ON hm.household_id=e.household_id WHERE e.id=$1 AND hm.user_id=$2)`, expenseID, userID).Scan(&exists); err != nil {
			return Expense{}, err
		}
		if !exists {
			return Expense{}, ErrNotFound
		}
		return Expense{}, ErrConflict
	})
}

func (r *PostgresRepository) MonthlySummary(ctx context.Context, userID, month string) (MonthlySummary, error) {
	var summary MonthlySummary
	summary.Month = month
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(sum(planned_amount_cents),0),
      COALESCE(sum(actual_amount_cents) FILTER (WHERE status='paid'),0),
      COALESCE(sum(planned_amount_cents) FILTER (WHERE status IN ('planned','open')),0)
      FROM expenses e JOIN household_members hm ON hm.household_id=e.household_id
	  WHERE hm.user_id=$1 AND e.due_date >= ($2||'-01')::date AND e.due_date < (($2||'-01')::date + interval '1 month')`, userID, month).Scan(
		&summary.PlannedAmountCents, &summary.ActualAmountCents, &summary.OpenAmountCents)
	summary.PlanActualDeltaCents = summary.PlannedAmountCents - summary.ActualAmountCents
	return summary, err
}

func (r *PostgresRepository) Exists(ctx context.Context, userID, expenseID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM expenses e JOIN household_members hm ON hm.household_id=e.household_id WHERE e.id=$1 AND hm.user_id=$2)`, expenseID, userID).Scan(&exists)
	return exists, err
}

func nullableUUID(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (r *PostgresRepository) UpsertBudget(ctx context.Context, userID, createdBy string, budget Budget) (Budget, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO pet_budgets(id,household_id,pet_id,month,category,amount_cents,currency,warning_threshold_cents,notes,created_by,created_at,updated_at)
	 SELECT $3,p.household_id,p.id,$4::date,$5,$6,$7,$8,$9,$10,$11,$11 FROM pets p
	 JOIN household_members hm ON hm.household_id=p.household_id WHERE hm.user_id=$1 AND p.id=$2
	 ON CONFLICT(household_id,pet_id,month,category) DO UPDATE SET amount_cents=EXCLUDED.amount_cents,currency=EXCLUDED.currency,warning_threshold_cents=EXCLUDED.warning_threshold_cents,notes=EXCLUDED.notes,updated_at=EXCLUDED.updated_at
	 RETURNING id::text,pet_id::text,to_char(month,'YYYY-MM'),category,amount_cents,currency,warning_threshold_cents,notes,updated_at`,
		userID, budget.PetID, budget.ID, budget.Month+"-01", budget.Category, budget.AmountCents, budget.Currency, budget.WarningThresholdCents, budget.Notes, createdBy, budget.UpdatedAt).
		Scan(&budget.ID, &budget.PetID, &budget.Month, &budget.Category, &budget.AmountCents, &budget.Currency, &budget.WarningThresholdCents, &budget.Notes, &budget.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Budget{}, ErrNotFound
	}
	return budget, err
}

func (r *PostgresRepository) ListBudgets(ctx context.Context, userID, petID, month string) ([]Budget, error) {
	if err := r.requirePet(ctx, userID, petID); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT b.id::text,b.pet_id::text,to_char(b.month,'YYYY-MM'),b.category,b.amount_cents,b.currency,b.warning_threshold_cents,b.notes,b.updated_at
	 FROM pet_budgets b JOIN household_members hm ON hm.household_id=b.household_id
	 WHERE hm.user_id=$1 AND b.pet_id=$2 AND b.month=$3::date ORDER BY b.category LIMIT 100`, userID, petID, month+"-01")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Budget, 0)
	for rows.Next() {
		var b Budget
		if err := rows.Scan(&b.ID, &b.PetID, &b.Month, &b.Category, &b.AmountCents, &b.Currency, &b.WarningThresholdCents, &b.Notes, &b.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) requirePet(ctx context.Context, userID, petID string) error {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pets p JOIN household_members hm ON hm.household_id=p.household_id WHERE hm.user_id=$1 AND p.id=$2)`, userID, petID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) PetSummary(ctx context.Context, userID, petID, period string) (PetSummary, error) {
	if err := r.requirePet(ctx, userID, petID); err != nil {
		return PetSummary{}, err
	}
	rows, err := r.pool.Query(ctx, `WITH expense_totals AS (
	 SELECT COALESCE(NULLIF(e.budget_category,''),NULLIF(e.category,''),'Unkategorisiert') category,
	 COALESCE(sum(e.planned_amount_cents),0) planned,COALESCE(sum(e.actual_amount_cents) FILTER(WHERE e.status='paid'),0) actual
	 FROM expenses e JOIN household_members hm ON hm.household_id=e.household_id
	 WHERE hm.user_id=$1 AND e.pet_id=$2
	 AND e.due_date >= CASE WHEN length($3)=4 THEN ($3||'-01-01')::date ELSE ($3||'-01')::date END
	 AND e.due_date < CASE WHEN length($3)=4 THEN ($3||'-01-01')::date + interval '1 year' ELSE ($3||'-01')::date + interval '1 month' END GROUP BY 1),
	 budget_totals AS (SELECT category,sum(amount_cents) budget,sum(warning_threshold_cents) warning_threshold FROM pet_budgets b JOIN household_members hm ON hm.household_id=b.household_id
	 WHERE hm.user_id=$1 AND b.pet_id=$2
	 AND b.month >= CASE WHEN length($3)=4 THEN ($3||'-01-01')::date ELSE ($3||'-01')::date END
	 AND b.month < CASE WHEN length($3)=4 THEN ($3||'-01-01')::date + interval '1 year' ELSE ($3||'-01')::date + interval '1 month' END GROUP BY category)
	 SELECT COALESCE(e.category,b.category),COALESCE(e.planned,0),COALESCE(e.actual,0),COALESCE(b.budget,0),b.warning_threshold,
	 COALESCE(b.budget,0)-COALESCE(e.actual,0),b.warning_threshold IS NOT NULL AND COALESCE(b.budget,0)-COALESCE(e.actual,0) <= b.warning_threshold
	 FROM expense_totals e FULL JOIN budget_totals b ON b.category=e.category ORDER BY 1 LIMIT 100`, userID, petID, period)
	if err != nil {
		return PetSummary{}, err
	}
	defer rows.Close()
	summary := PetSummary{PetID: petID, Period: period, Categories: make([]CategorySummary, 0)}
	for rows.Next() {
		var c CategorySummary
		if err := rows.Scan(&c.Category, &c.PlannedAmountCents, &c.ActualAmountCents, &c.BudgetAmountCents, &c.WarningThresholdCents, &c.RemainingBudgetCents, &c.WarningThresholdReached); err != nil {
			return PetSummary{}, err
		}
		summary.Categories = append(summary.Categories, c)
	}
	return summary, rows.Err()
}
