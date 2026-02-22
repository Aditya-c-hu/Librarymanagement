package models

import "time"

type Checkout struct {
	ID           int64      `json:"id"`
	UserID       int64      `json:"user_id"`
	CopyID       int64      `json:"copy_id"`
	CheckedOutAt time.Time  `json:"checked_out_at"`
	DueDate      time.Time  `json:"due_date"`
	ReturnedAt   *time.Time `json:"returned_at,omitempty"`
	FineAmount   float64    `json:"fine_amount"`
	FinePaid     bool       `json:"fine_paid"`
	Renewals     int        `json:"renewals"`
	CreatedAt    time.Time  `json:"created_at"`
}

type CheckoutDetail struct {
	Checkout
	BookTitle  string `json:"book_title"`
	BookAuthor string `json:"book_author"`
	BookISBN   string `json:"book_isbn"`
	CopyNumber int    `json:"copy_number"`
	UserName   string `json:"user_name"`
	UserEmail  string `json:"user_email"`
}

type CheckoutRequest struct {
	BookID int64 `json:"book_id"`
}

type ReturnResponse struct {
	Checkout          CheckoutDetail `json:"checkout"`
	FineAmount        float64        `json:"fine_amount"`
	DaysLate          int            `json:"days_late"`
	ReservationQueued bool           `json:"reservation_queued"`
}
