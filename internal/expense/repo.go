package expense

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

type Repo struct {
	conn *pgxpool.Pool
}

type Filter struct {
	Year     int
	Month    int
	Category string
	Date     time.Time
}

func NewRepo(
	ctx context.Context,
	conn *pgxpool.Pool,
) *Repo {
	return &Repo{
		conn: conn,
	}
}

func (r *Repo) AddExpense(
	ctx context.Context,
	e Expense,
) error {
	sql := `
		INSERT INTO expense	(user_id, category, amount, comment)
		VALUES ($1, $2, $3, $4)
		`
	_, err := r.conn.Exec(ctx, sql, e.UserId, e.Category, e.Amount, e.Comment)
	if err != nil {
		return fmt.Errorf("error adding expense: %v", err)
	}

	return nil
}

func (r *Repo) GetExpenses(
	ctx context.Context,
	filter Filter,
) ([]Expense, error) {
	sql := `
		SELECT id, user_id, category, amount, comment, created_at
		FROM expense
		WHERE 1=1
		%s
		ORDER BY created_at DESC
		`
	var args []interface{}
	var where string
	if filter.Year != 0 {
		args = append(args, filter.Year)
		where += fmt.Sprintf(" AND EXTRACT(YEAR FROM created_at) = $%d ", len(args))
	}
	if filter.Month != 0 {
		args = append(args, filter.Month)
		where += fmt.Sprintf(" AND EXTRACT(MONTH FROM created_at) = $%d ", len(args))
	}
	if filter.Category != "" {
		args = append(args, filter.Category)
		where += fmt.Sprintf(" AND category = $%d ", len(args))
	}
	if !filter.Date.IsZero() {
		args = append(args, filter.Date)
		where += fmt.Sprintf(" AND created_at::date = $%d ", len(args))
	}

	rows, err := r.conn.Query(
		ctx,
		sql,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("error getting month expenses: %v", err)
	}
	defer rows.Close()

	var expenses []Expense
	for rows.Next() {
		var e Expense
		err := rows.Scan(
			&e.Id,
			&e.UserId,
			&e.Category,
			&e.Amount,
			&e.Comment,
			&e.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning row: %v", err)
		}
		expenses = append(expenses, e)
	}

	return expenses, nil
}
