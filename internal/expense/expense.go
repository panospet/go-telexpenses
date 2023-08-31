package expense

import (
	"time"
)

type Expense struct {
	Id        int64
	Category  string
	Amount    float64
	Comment   string
	CreatedAt time.Time
}
