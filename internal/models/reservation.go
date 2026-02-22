package models

import "time"

type ReservationStatus string

const (
	ReservationPending   ReservationStatus = "pending"
	ReservationFulfilled ReservationStatus = "fulfilled"
	ReservationCancelled ReservationStatus = "cancelled"
	ReservationExpired   ReservationStatus = "expired"
)

type Reservation struct {
	ID          int64             `json:"id"`
	UserID      int64             `json:"user_id"`
	BookID      int64             `json:"book_id"`
	Status      ReservationStatus `json:"status"`
	QueuePos    int               `json:"queue_position"`
	CreatedAt   time.Time         `json:"created_at"`
	FulfilledAt *time.Time        `json:"fulfilled_at,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	CopyID      *int64            `json:"copy_id,omitempty"`
}

type ReservationDetail struct {
	Reservation
	BookTitle  string `json:"book_title"`
	BookAuthor string `json:"book_author"`
	UserName   string `json:"user_name"`
	UserEmail  string `json:"user_email"`
}

type ReservationRequest struct {
	BookID int64 `json:"book_id"`
}
