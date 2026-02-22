package services

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/ravidesai/library-api/internal/config"
	"github.com/ravidesai/library-api/internal/models"
)

var (
	ErrNoCopiesAvailable    = errors.New("no copies available for checkout")
	ErrAlreadyCheckedOut    = errors.New("you already have this book checked out")
	ErrCheckoutNotFound     = errors.New("checkout record not found")
	ErrAlreadyReturned      = errors.New("book already returned")
	ErrMaxRenewalsReached   = errors.New("maximum renewals reached")
	ErrCannotRenewOverdue   = errors.New("cannot renew an overdue book")
)

type CheckoutService struct {
	db  *sql.DB
	cfg *config.Config
}

func NewCheckoutService(db *sql.DB, cfg *config.Config) *CheckoutService {
	return &CheckoutService{db: db, cfg: cfg}
}

// CheckoutBook attempts to check out a book for a user.
// It uses a transaction to atomically find an available copy and mark it as checked out.
// If no copies are available, it returns ErrNoCopiesAvailable so the handler can suggest a reservation.
func (s *CheckoutService) CheckoutBook(userID, bookID int64) (*models.CheckoutDetail, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Verify book exists
	var exists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM books WHERE id = ?)`, bookID).Scan(&exists); err != nil || !exists {
		return nil, ErrBookNotFound
	}

	// Check if user already has an active checkout of this book
	var activeCount int
	err = tx.QueryRow(`
		SELECT COUNT(*) FROM checkouts co
		JOIN book_copies bc ON bc.id = co.copy_id
		WHERE co.user_id = ? AND bc.book_id = ? AND co.returned_at IS NULL
	`, userID, bookID).Scan(&activeCount)
	if err != nil {
		return nil, err
	}
	if activeCount > 0 {
		return nil, ErrAlreadyCheckedOut
	}

	// Check if the user has a fulfilled reservation for this book — use that assigned copy
	var reservationID int64
	var reservedCopyID sql.NullInt64
	err = tx.QueryRow(`
		SELECT id, copy_id FROM reservations
		WHERE user_id = ? AND book_id = ? AND status = 'fulfilled' AND expires_at > ?
		ORDER BY fulfilled_at ASC LIMIT 1
	`, userID, bookID, time.Now()).Scan(&reservationID, &reservedCopyID)

	var copyID int64

	if err == nil && reservedCopyID.Valid {
		// Use the reserved copy
		copyID = reservedCopyID.Int64
		_, err = tx.Exec(`UPDATE reservations SET status = 'cancelled' WHERE id = ?`, reservationID)
		if err != nil {
			return nil, err
		}
	} else {
		// Find any available copy (SELECT with implicit lock via single-writer SQLite)
		err = tx.QueryRow(`
			SELECT id FROM book_copies
			WHERE book_id = ? AND status = 'available'
			LIMIT 1
		`, bookID).Scan(&copyID)
		if err != nil {
			return nil, ErrNoCopiesAvailable
		}
	}

	// Mark copy as checked_out
	_, err = tx.Exec(
		`UPDATE book_copies SET status = 'checked_out', updated_at = ? WHERE id = ?`,
		time.Now(), copyID,
	)
	if err != nil {
		return nil, err
	}

	dueDate := time.Now().AddDate(0, 0, s.cfg.LoanPeriodDays)
	result, err := tx.Exec(`
		INSERT INTO checkouts (user_id, copy_id, checked_out_at, due_date)
		VALUES (?, ?, ?, ?)
	`, userID, copyID, time.Now(), dueDate)
	if err != nil {
		return nil, err
	}

	checkoutID, _ := result.LastInsertId()

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetByID(checkoutID)
}

// ReturnBook processes a book return, calculates fines, and triggers reservation fulfillment.
func (s *CheckoutService) ReturnBook(checkoutID, userID int64, isLibrarian bool) (*models.ReturnResponse, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var co models.Checkout
	var copyID int64
	err = tx.QueryRow(`
		SELECT id, user_id, copy_id, checked_out_at, due_date, returned_at, fine_amount, fine_paid, renewals
		FROM checkouts WHERE id = ?
	`, checkoutID).Scan(
		&co.ID, &co.UserID, &copyID, &co.CheckedOutAt, &co.DueDate, &co.ReturnedAt,
		&co.FineAmount, &co.FinePaid, &co.Renewals,
	)
	if err != nil {
		return nil, ErrCheckoutNotFound
	}

	if !isLibrarian && co.UserID != userID {
		return nil, ErrCheckoutNotFound
	}
	if co.ReturnedAt != nil {
		return nil, ErrAlreadyReturned
	}

	now := time.Now()
	var fineAmount float64
	var daysLate int

	if now.After(co.DueDate) {
		hoursLate := now.Sub(co.DueDate).Hours()
		daysLate = int(math.Ceil(hoursLate / 24))
		fineAmount = float64(daysLate) * s.cfg.FinePerDay
	}

	_, err = tx.Exec(`
		UPDATE checkouts SET returned_at = ?, fine_amount = ? WHERE id = ?
	`, now, fineAmount, checkoutID)
	if err != nil {
		return nil, err
	}

	// Find the book_id for this copy to check reservations
	var bookID int64
	tx.QueryRow(`SELECT book_id FROM book_copies WHERE id = ?`, copyID).Scan(&bookID)

	// Check if there is a pending reservation for this book
	var resID int64
	err = tx.QueryRow(`
		SELECT id FROM reservations
		WHERE book_id = ? AND status = 'pending'
		ORDER BY queue_pos ASC
		LIMIT 1
	`, bookID).Scan(&resID)

	reservationQueued := false
	if err == nil {
		// Fulfill the reservation: assign this copy to the next person in the queue
		expiresAt := now.Add(s.cfg.ReservationTTL)
		_, err = tx.Exec(`
			UPDATE reservations SET status = 'fulfilled', copy_id = ?, fulfilled_at = ?, expires_at = ?
			WHERE id = ?
		`, copyID, now, expiresAt, resID)
		if err != nil {
			return nil, err
		}
		_, err = tx.Exec(
			`UPDATE book_copies SET status = 'reserved', updated_at = ? WHERE id = ?`,
			now, copyID,
		)
		if err != nil {
			return nil, err
		}
		reservationQueued = true
	} else {
		// No reservation — mark copy as available
		_, err = tx.Exec(
			`UPDATE book_copies SET status = 'available', updated_at = ? WHERE id = ?`,
			now, copyID,
		)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	detail, err := s.GetByID(checkoutID)
	if err != nil {
		return nil, err
	}

	return &models.ReturnResponse{
		Checkout:          *detail,
		FineAmount:        fineAmount,
		DaysLate:          daysLate,
		ReservationQueued: reservationQueued,
	}, nil
}

func (s *CheckoutService) RenewCheckout(checkoutID, userID int64) (*models.CheckoutDetail, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var co models.Checkout
	var copyID int64
	err = tx.QueryRow(`
		SELECT id, user_id, copy_id, checked_out_at, due_date, returned_at, renewals
		FROM checkouts WHERE id = ? AND user_id = ?
	`, checkoutID, userID).Scan(
		&co.ID, &co.UserID, &copyID, &co.CheckedOutAt, &co.DueDate, &co.ReturnedAt, &co.Renewals,
	)
	if err != nil {
		return nil, ErrCheckoutNotFound
	}
	if co.ReturnedAt != nil {
		return nil, ErrAlreadyReturned
	}
	if co.Renewals >= s.cfg.MaxRenewals {
		return nil, ErrMaxRenewalsReached
	}
	if time.Now().After(co.DueDate) {
		return nil, ErrCannotRenewOverdue
	}

	// Check no pending reservations for this book
	var bookID int64
	tx.QueryRow(`SELECT book_id FROM book_copies WHERE id = ?`, copyID).Scan(&bookID)

	var pendingRes int
	tx.QueryRow(`SELECT COUNT(*) FROM reservations WHERE book_id = ? AND status = 'pending'`, bookID).Scan(&pendingRes)
	if pendingRes > 0 {
		return nil, fmt.Errorf("cannot renew: there are pending reservations for this book")
	}

	newDue := co.DueDate.AddDate(0, 0, s.cfg.LoanPeriodDays)
	_, err = tx.Exec(`
		UPDATE checkouts SET due_date = ?, renewals = renewals + 1 WHERE id = ?
	`, newDue, checkoutID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetByID(checkoutID)
}

func (s *CheckoutService) GetByID(id int64) (*models.CheckoutDetail, error) {
	var d models.CheckoutDetail
	err := s.db.QueryRow(`
		SELECT co.id, co.user_id, co.copy_id, co.checked_out_at, co.due_date, co.returned_at,
		       co.fine_amount, co.fine_paid, co.renewals, co.created_at,
		       b.title, b.author, b.isbn, bc.copy_number,
		       u.name, u.email
		FROM checkouts co
		JOIN book_copies bc ON bc.id = co.copy_id
		JOIN books b ON b.id = bc.book_id
		JOIN users u ON u.id = co.user_id
		WHERE co.id = ?
	`, id).Scan(
		&d.ID, &d.UserID, &d.CopyID, &d.CheckedOutAt, &d.DueDate, &d.ReturnedAt,
		&d.FineAmount, &d.FinePaid, &d.Renewals, &d.CreatedAt,
		&d.BookTitle, &d.BookAuthor, &d.BookISBN, &d.CopyNumber,
		&d.UserName, &d.UserEmail,
	)
	if err != nil {
		return nil, ErrCheckoutNotFound
	}
	return &d, nil
}

func (s *CheckoutService) ListByUser(userID int64, activeOnly bool) ([]models.CheckoutDetail, error) {
	query := `
		SELECT co.id, co.user_id, co.copy_id, co.checked_out_at, co.due_date, co.returned_at,
		       co.fine_amount, co.fine_paid, co.renewals, co.created_at,
		       b.title, b.author, b.isbn, bc.copy_number,
		       u.name, u.email
		FROM checkouts co
		JOIN book_copies bc ON bc.id = co.copy_id
		JOIN books b ON b.id = bc.book_id
		JOIN users u ON u.id = co.user_id
		WHERE co.user_id = ?
	`
	if activeOnly {
		query += ` AND co.returned_at IS NULL`
	}
	query += ` ORDER BY co.checked_out_at DESC`

	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.CheckoutDetail
	for rows.Next() {
		var d models.CheckoutDetail
		if err := rows.Scan(
			&d.ID, &d.UserID, &d.CopyID, &d.CheckedOutAt, &d.DueDate, &d.ReturnedAt,
			&d.FineAmount, &d.FinePaid, &d.Renewals, &d.CreatedAt,
			&d.BookTitle, &d.BookAuthor, &d.BookISBN, &d.CopyNumber,
			&d.UserName, &d.UserEmail,
		); err != nil {
			return nil, err
		}
		results = append(results, d)
	}
	return results, nil
}

func (s *CheckoutService) ListAll(activeOnly bool) ([]models.CheckoutDetail, error) {
	query := `
		SELECT co.id, co.user_id, co.copy_id, co.checked_out_at, co.due_date, co.returned_at,
		       co.fine_amount, co.fine_paid, co.renewals, co.created_at,
		       b.title, b.author, b.isbn, bc.copy_number,
		       u.name, u.email
		FROM checkouts co
		JOIN book_copies bc ON bc.id = co.copy_id
		JOIN books b ON b.id = bc.book_id
		JOIN users u ON u.id = co.user_id
	`
	if activeOnly {
		query += ` WHERE co.returned_at IS NULL`
	}
	query += ` ORDER BY co.checked_out_at DESC`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.CheckoutDetail
	for rows.Next() {
		var d models.CheckoutDetail
		if err := rows.Scan(
			&d.ID, &d.UserID, &d.CopyID, &d.CheckedOutAt, &d.DueDate, &d.ReturnedAt,
			&d.FineAmount, &d.FinePaid, &d.Renewals, &d.CreatedAt,
			&d.BookTitle, &d.BookAuthor, &d.BookISBN, &d.CopyNumber,
			&d.UserName, &d.UserEmail,
		); err != nil {
			return nil, err
		}
		results = append(results, d)
	}
	return results, nil
}

func (s *CheckoutService) ListOverdue() ([]models.CheckoutDetail, error) {
	query := `
		SELECT co.id, co.user_id, co.copy_id, co.checked_out_at, co.due_date, co.returned_at,
		       co.fine_amount, co.fine_paid, co.renewals, co.created_at,
		       b.title, b.author, b.isbn, bc.copy_number,
		       u.name, u.email
		FROM checkouts co
		JOIN book_copies bc ON bc.id = co.copy_id
		JOIN books b ON b.id = bc.book_id
		JOIN users u ON u.id = co.user_id
		WHERE co.returned_at IS NULL AND co.due_date < ?
		ORDER BY co.due_date ASC
	`
	rows, err := s.db.Query(query, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.CheckoutDetail
	for rows.Next() {
		var d models.CheckoutDetail
		if err := rows.Scan(
			&d.ID, &d.UserID, &d.CopyID, &d.CheckedOutAt, &d.DueDate, &d.ReturnedAt,
			&d.FineAmount, &d.FinePaid, &d.Renewals, &d.CreatedAt,
			&d.BookTitle, &d.BookAuthor, &d.BookISBN, &d.CopyNumber,
			&d.UserName, &d.UserEmail,
		); err != nil {
			return nil, err
		}
		results = append(results, d)
	}
	return results, nil
}

func (s *CheckoutService) PayFine(checkoutID int64) error {
	result, err := s.db.Exec(`
		UPDATE checkouts SET fine_paid = 1 WHERE id = ? AND fine_amount > 0 AND fine_paid = 0
	`, checkoutID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no unpaid fine found for this checkout")
	}
	return nil
}
