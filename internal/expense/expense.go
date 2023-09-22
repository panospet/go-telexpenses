package expense

import (
	"time"
)

type Expense struct {
	Id        int64
	UserId    int64
	Category  string
	Amount    float64
	Comment   string
	CreatedAt time.Time
}

func SumExpensesByCategory(expenses []Expense) map[string]float64 {
	sums := make(map[string]float64)
	for _, e := range expenses {
		sums[e.Category] += e.Amount
	}
	return sums
}
