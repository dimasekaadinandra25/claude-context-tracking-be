package dto

import "time"

type Budget struct {
	UserID            string    `json:"user_id"`
	FullName          string    `json:"full_name"`
	Email             string    `json:"email"`
	MonthlyNewContext *int64    `json:"monthly_new_context"`
	UsedNewContext    int64     `json:"used_new_context"`
	MonthStart        time.Time `json:"month_start"`
}

type SetBudgetRequest struct {
	MonthlyNewContext int64 `json:"monthly_new_context" binding:"required,gt=0"`
}
