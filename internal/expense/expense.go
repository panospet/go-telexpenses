package expense

import (
	"time"
)

type Expense struct {
	Id        int64
	User      string
	Category  string
	Amount    float64
	Comment   string
	CreatedAt time.Time
}

func SumExpensesByCategory(expenses []Expense) map[string]float64 {
	sums := make(map[string]float64)
	total := float64(0)
	for _, e := range expenses {
		sums[e.Category] += e.Amount
		total += e.Amount
	}
	sums["Σύνολο"] = total
	return sums
}
