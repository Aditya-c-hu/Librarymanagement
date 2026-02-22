package models

import "time"

type Book struct {
	ID            int64     `json:"id"`
	Title         string    `json:"title"`
	Author        string    `json:"author"`
	ISBN          string    `json:"isbn"`
	Publisher     string    `json:"publisher"`
	PublishedYear int       `json:"published_year"`
	Genre         string    `json:"genre"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type BookDetail struct {
	Book
	TotalCopies     int `json:"total_copies"`
	AvailableCopies int `json:"available_copies"`
}

type CreateBookRequest struct {
	Title         string `json:"title"`
	Author        string `json:"author"`
	ISBN          string `json:"isbn"`
	Publisher     string `json:"publisher"`
	PublishedYear int    `json:"published_year"`
	Genre         string `json:"genre"`
	Description   string `json:"description"`
	Copies        int    `json:"copies"`
}

type UpdateBookRequest struct {
	Title         *string `json:"title"`
	Author        *string `json:"author"`
	ISBN          *string `json:"isbn"`
	Publisher     *string `json:"publisher"`
	PublishedYear *int    `json:"published_year"`
	Genre         *string `json:"genre"`
	Description   *string `json:"description"`
}

type CopyStatus string

const (
	StatusAvailable  CopyStatus = "available"
	StatusCheckedOut CopyStatus = "checked_out"
	StatusReserved   CopyStatus = "reserved"
	StatusLost       CopyStatus = "lost"
	StatusMaintenance CopyStatus = "maintenance"
)

type BookCopy struct {
	ID         int64      `json:"id"`
	BookID     int64      `json:"book_id"`
	CopyNumber int        `json:"copy_number"`
	Status     CopyStatus `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
