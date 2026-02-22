package services

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Aditya-c-hu/Librarymanagement/internal/config"
	"github.com/Aditya-c-hu/Librarymanagement/internal/models"
)

var (
	ErrAlreadyReserved       = errors.New("you already have a pending reservation for this book")
	ErrReservationNotFound   = errors.New("reservation not found")
	ErrCannotCancelFulfilled = errors.New("cannot cancel a fulfilled reservation; check out or let it expire")
)

type ReservationService struct {
	db  *sql.DB
	cfg *config.Config
}

func NewReservationService(db *sql.DB, cfg *config.Config) *ReservationService {
	return &ReservationService{db: db, cfg: cfg}
}

// Reserve places a user into the reservation queue for a book.
// Queue position is determined by counting existing pending reservations + 1.
func (s *ReservationService) Reserve(userID, bookID int64) (*models.ReservationDetail, error) {
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

	// Check for existing pending/fulfilled reservation by this user
	var count int
	err = tx.QueryRow(`
		SELECT COUNT(*) FROM reservations
		WHERE user_id = ? AND book_id = ? AND status IN ('pending', 'fulfilled')
	`, userID, bookID).Scan(&count)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrAlreadyReserved
	}

	// Determine queue position
	var maxPos int
	tx.QueryRow(`
		SELECT COALESCE(MAX(queue_pos), 0) FROM reservations
		WHERE book_id = ? AND status = 'pending'
	`, bookID).Scan(&maxPos)

	pos := maxPos + 1

	result, err := tx.Exec(`
		INSERT INTO reservations (user_id, book_id, status, queue_pos)
		VALUES (?, ?, 'pending', ?)
	`, userID, bookID, pos)
	if err != nil {
		return nil, err
	}

	resID, _ := result.LastInsertId()

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetByID(resID)
}

func (s *ReservationService) Cancel(reservationID, userID int64, isLibrarian bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var res models.Reservation
	err = tx.QueryRow(`
		SELECT id, user_id, book_id, status, queue_pos, copy_id FROM reservations WHERE id = ?
	`, reservationID).Scan(&res.ID, &res.UserID, &res.BookID, &res.Status, &res.QueuePos, &res.CopyID)
	if err != nil {
		return ErrReservationNotFound
	}

	if !isLibrarian && res.UserID != userID {
		return ErrReservationNotFound
	}

	if res.Status == models.ReservationFulfilled {
		// If fulfilled, release the reserved copy back to available
		if res.CopyID != nil {
			_, err = tx.Exec(`UPDATE book_copies SET status = 'available', updated_at = ? WHERE id = ?`, time.Now(), *res.CopyID)
			if err != nil {
				return err
			}
		}
	} else if res.Status != models.ReservationPending {
		return fmt.Errorf("reservation is already %s", res.Status)
	}

	_, err = tx.Exec(`UPDATE reservations SET status = 'cancelled' WHERE id = ?`, reservationID)
	if err != nil {
		return err
	}

	// Re-number queue positions for remaining pending reservations of this book
	_, err = tx.Exec(`
		UPDATE reservations SET queue_pos = queue_pos - 1
		WHERE book_id = ? AND status = 'pending' AND queue_pos > ?
	`, res.BookID, res.QueuePos)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *ReservationService) GetByID(id int64) (*models.ReservationDetail, error) {
	var d models.ReservationDetail
	err := s.db.QueryRow(`
		SELECT r.id, r.user_id, r.book_id, r.status, r.queue_pos, r.created_at,
		       r.fulfilled_at, r.expires_at, r.copy_id,
		       b.title, b.author, u.name, u.email
		FROM reservations r
		JOIN books b ON b.id = r.book_id
		JOIN users u ON u.id = r.user_id
		WHERE r.id = ?
	`, id).Scan(
		&d.ID, &d.UserID, &d.BookID, &d.Status, &d.QueuePos, &d.CreatedAt,
		&d.FulfilledAt, &d.ExpiresAt, &d.CopyID,
		&d.BookTitle, &d.BookAuthor, &d.UserName, &d.UserEmail,
	)
	if err != nil {
		return nil, ErrReservationNotFound
	}
	return &d, nil
}

func (s *ReservationService) ListByUser(userID int64) ([]models.ReservationDetail, error) {
	rows, err := s.db.Query(`
		SELECT r.id, r.user_id, r.book_id, r.status, r.queue_pos, r.created_at,
		       r.fulfilled_at, r.expires_at, r.copy_id,
		       b.title, b.author, u.name, u.email
		FROM reservations r
		JOIN books b ON b.id = r.book_id
		JOIN users u ON u.id = r.user_id
		WHERE r.user_id = ?
		ORDER BY r.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanReservations(rows)
}

func (s *ReservationService) ListByBook(bookID int64) ([]models.ReservationDetail, error) {
	rows, err := s.db.Query(`
		SELECT r.id, r.user_id, r.book_id, r.status, r.queue_pos, r.created_at,
		       r.fulfilled_at, r.expires_at, r.copy_id,
		       b.title, b.author, u.name, u.email
		FROM reservations r
		JOIN books b ON b.id = r.book_id
		JOIN users u ON u.id = r.user_id
		WHERE r.book_id = ? AND r.status = 'pending'
		ORDER BY r.queue_pos ASC
	`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanReservations(rows)
}

func (s *ReservationService) ListAll() ([]models.ReservationDetail, error) {
	rows, err := s.db.Query(`
		SELECT r.id, r.user_id, r.book_id, r.status, r.queue_pos, r.created_at,
		       r.fulfilled_at, r.expires_at, r.copy_id,
		       b.title, b.author, u.name, u.email
		FROM reservations r
		JOIN books b ON b.id = r.book_id
		JOIN users u ON u.id = r.user_id
		ORDER BY r.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanReservations(rows)
}

// ExpireStaleReservations marks fulfilled reservations as expired if their TTL has passed.
// Returns the number of expired reservations.
func (s *ReservationService) ExpireStaleReservations() (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Find fulfilled reservations past their expiry
	rows, err := tx.Query(`
		SELECT id, copy_id, book_id FROM reservations
		WHERE status = 'fulfilled' AND expires_at < ?
	`, time.Now())
	if err != nil {
		return 0, err
	}

	type expiredRes struct {
		id     int64
		copyID *int64
		bookID int64
	}
	var expired []expiredRes
	for rows.Next() {
		var e expiredRes
		if err := rows.Scan(&e.id, &e.copyID, &e.bookID); err != nil {
			rows.Close()
			return 0, err
		}
		expired = append(expired, e)
	}
	rows.Close()

	for _, e := range expired {
		_, err = tx.Exec(`UPDATE reservations SET status = 'expired' WHERE id = ?`, e.id)
		if err != nil {
			return 0, err
		}

		if e.copyID != nil {
			// Try to fulfill the next pending reservation, or release the copy
			var nextID int64
			err = tx.QueryRow(`
				SELECT id FROM reservations
				WHERE book_id = ? AND status = 'pending'
				ORDER BY queue_pos ASC LIMIT 1
			`, e.bookID).Scan(&nextID)

			if err == nil {
				expiresAt := time.Now().Add(s.cfg.ReservationTTL)
				tx.Exec(`
					UPDATE reservations SET status = 'fulfilled', copy_id = ?, fulfilled_at = ?, expires_at = ?
					WHERE id = ?
				`, *e.copyID, time.Now(), expiresAt, nextID)
			} else {
				tx.Exec(`UPDATE book_copies SET status = 'available', updated_at = ? WHERE id = ?`, time.Now(), *e.copyID)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(expired), nil
}

func (s *ReservationService) scanReservations(rows *sql.Rows) ([]models.ReservationDetail, error) {
	var results []models.ReservationDetail
	for rows.Next() {
		var d models.ReservationDetail
		if err := rows.Scan(
			&d.ID, &d.UserID, &d.BookID, &d.Status, &d.QueuePos, &d.CreatedAt,
			&d.FulfilledAt, &d.ExpiresAt, &d.CopyID,
			&d.BookTitle, &d.BookAuthor, &d.UserName, &d.UserEmail,
		); err != nil {
			return nil, err
		}
		results = append(results, d)
	}
	return results, nil
}
