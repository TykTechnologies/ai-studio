package services

import (
	"testing"
	"time"
)

func TestCalculateBudgetPeriodStart(t *testing.T) {
	service := &BudgetService{}

	// Helper to create time.Time with a specific date
	createDate := func(year int, month time.Month, day, hour int) time.Time {
		return time.Date(year, month, day, hour, 0, 0, 0, time.UTC)
	}

	tests := []struct {
		name          string
		budgetDate    *time.Time
		now           time.Time
		expectedStart time.Time
	}{
		{
			name:          "No budget date set - use 1st of current month",
			budgetDate:    nil,
			now:           createDate(2024, 2, 15, 12),
			expectedStart: createDate(2024, 2, 1, 0),
		},
		{
			name: "Before budget day in current month - use previous month's budget day",
			budgetDate: func() *time.Time {
				d := createDate(2024, 1, 14, 0)
				return &d
			}(),
			now:           createDate(2024, 2, 13, 12),
			expectedStart: createDate(2024, 1, 14, 0),
		},
		{
			name: "On budget day - use current month's budget day",
			budgetDate: func() *time.Time {
				d := createDate(2024, 1, 14, 0)
				return &d
			}(),
			now:           createDate(2024, 2, 14, 12),
			expectedStart: createDate(2024, 2, 14, 0),
		},
		{
			name: "After budget day - use current month's budget day",
			budgetDate: func() *time.Time {
				d := createDate(2024, 1, 14, 0)
				return &d
			}(),
			now:           createDate(2024, 2, 15, 12),
			expectedStart: createDate(2024, 2, 14, 0),
		},
		{
			name: "Before budget day in January - use December's budget day",
			budgetDate: func() *time.Time {
				d := createDate(2024, 1, 14, 0)
				return &d
			}(),
			now:           createDate(2024, 1, 13, 12),
			expectedStart: createDate(2023, 12, 14, 0),
		},
		{
			name: "Budget day on last day of month",
			budgetDate: func() *time.Time {
				d := createDate(2024, 1, 31, 0)
				return &d
			}(),
			now:           createDate(2024, 2, 15, 12),
			expectedStart: createDate(2024, 1, 31, 0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.calculateBudgetPeriodStart(tt.budgetDate, tt.now)
			if !got.Equal(tt.expectedStart) {
				t.Errorf("calculateBudgetPeriodStart() = %v, want %v", got, tt.expectedStart)
			}
		})
	}
}
