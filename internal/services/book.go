package services

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Aditya-c-hu/Librarymanagement/internal/models"
)

var (
	ErrBookNotFound = errors.New("book not found")
	ErrCopyNotFound = errors.New("book copy not found")
	ErrDuplicateISBN = errors.New("a book with this ISBN already exists")
)

type BookService struct {
	db *sql.DB
}

func NewBookService(db *sql.DB) *BookService {
	return &BookService{db: db}
}

func (s *BookService) Create(req models.CreateBookRequest) (*models.BookDetail, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`INSERT INTO books (title, author, isbn, publisher, published_year, genre, description)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		req.Title, req.Author, req.ISBN, req.Publisher, req.PublishedYear, req.Genre, req.Description,
	)
	if err != nil {
		return nil, ErrDuplicateISBN
	}

	bookID, _ := result.LastInsertId()

	copies := req.Copies
	if copies <= 0 {
		copies = 1
	}
	for i := 1; i <= copies; i++ {
		_, err := tx.Exec(
			`INSERT INTO book_copies (book_id, copy_number, status) VALUES (?, ?, 'available')`,
			bookID, i,
		)
		if err != nil {
			return nil, fmt.Errorf("creating copy %d: %w", i, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetByID(bookID)
}

func (s *BookService) GetByID(id int64) (*models.BookDetail, error) {
	var b models.BookDetail
	err := s.db.QueryRow(`
		SELECT b.id, b.title, b.author, b.isbn, b.publisher, b.published_year, b.genre, b.description,
		       b.created_at, b.updated_at,
		       COUNT(c.id) as total_copies,
		       COUNT(CASE WHEN c.status = 'available' THEN 1 END) as available_copies
		FROM books b
		LEFT JOIN book_copies c ON c.book_id = b.id
		WHERE b.id = ?
		GROUP BY b.id
	`, id).Scan(
		&b.ID, &b.Title, &b.Author, &b.ISBN, &b.Publisher, &b.PublishedYear, &b.Genre, &b.Description,
		&b.CreatedAt, &b.UpdatedAt, &b.TotalCopies, &b.AvailableCopies,
	)
	if err != nil {
		return nil, ErrBookNotFound
	}
	return &b, nil
}

func (s *BookService) List(page, perPage int, search string) (*models.PaginatedResponse, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	var totalItems int
	countQuery := `SELECT COUNT(*) FROM books`
	args := []interface{}{}

	if search != "" {
		countQuery += ` WHERE title LIKE ? OR author LIKE ? OR isbn LIKE ?`
		like := "%" + search + "%"
		args = append(args, like, like, like)
	}

	if err := s.db.QueryRow(countQuery, args...).Scan(&totalItems); err != nil {
		return nil, err
	}

	query := `
		SELECT b.id, b.title, b.author, b.isbn, b.publisher, b.published_year, b.genre, b.description,
		       b.created_at, b.updated_at,
		       COUNT(c.id) as total_copies,
		       COUNT(CASE WHEN c.status = 'available' THEN 1 END) as available_copies
		FROM books b
		LEFT JOIN book_copies c ON c.book_id = b.id
	`
	queryArgs := []interface{}{}
	if search != "" {
		query += ` WHERE b.title LIKE ? OR b.author LIKE ? OR b.isbn LIKE ?`
		like := "%" + search + "%"
		queryArgs = append(queryArgs, like, like, like)
	}
	query += ` GROUP BY b.id ORDER BY b.title LIMIT ? OFFSET ?`
	queryArgs = append(queryArgs, perPage, (page-1)*perPage)

	rows, err := s.db.Query(query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []models.BookDetail
	for rows.Next() {
		var b models.BookDetail
		if err := rows.Scan(
			&b.ID, &b.Title, &b.Author, &b.ISBN, &b.Publisher, &b.PublishedYear, &b.Genre, &b.Description,
			&b.CreatedAt, &b.UpdatedAt, &b.TotalCopies, &b.AvailableCopies,
		); err != nil {
			return nil, err
		}
		books = append(books, b)
	}

	return &models.PaginatedResponse{
		Data:       books,
		Page:       page,
		PerPage:    perPage,
		TotalItems: totalItems,
		TotalPages: int(math.Ceil(float64(totalItems) / float64(perPage))),
	}, nil
}

func (s *BookService) Update(id int64, req models.UpdateBookRequest) (*models.BookDetail, error) {
	book, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	if req.Title != nil {
		book.Title = *req.Title
	}
	if req.Author != nil {
		book.Author = *req.Author
	}
	if req.ISBN != nil {
		book.ISBN = *req.ISBN
	}
	if req.Publisher != nil {
		book.Publisher = *req.Publisher
	}
	if req.PublishedYear != nil {
		book.PublishedYear = *req.PublishedYear
	}
	if req.Genre != nil {
		book.Genre = *req.Genre
	}
	if req.Description != nil {
		book.Description = *req.Description
	}

	_, err = s.db.Exec(`
		UPDATE books SET title=?, author=?, isbn=?, publisher=?, published_year=?, genre=?, description=?, updated_at=?
		WHERE id=?`,
		book.Title, book.Author, book.ISBN, book.Publisher, book.PublishedYear, book.Genre, book.Description,
		time.Now(), id,
	)
	if err != nil {
		return nil, err
	}

	return s.GetByID(id)
}

func (s *BookService) Delete(id int64) error {
	result, err := s.db.Exec(`DELETE FROM books WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrBookNotFound
	}
	return nil
}

func (s *BookService) AddCopies(bookID int64, count int) ([]models.BookCopy, error) {
	if _, err := s.GetByID(bookID); err != nil {
		return nil, err
	}

	var maxCopy int
	s.db.QueryRow(`SELECT COALESCE(MAX(copy_number), 0) FROM book_copies WHERE book_id = ?`, bookID).Scan(&maxCopy)

	var copies []models.BookCopy
	for i := 1; i <= count; i++ {
		num := maxCopy + i
		result, err := s.db.Exec(
			`INSERT INTO book_copies (book_id, copy_number, status) VALUES (?, ?, 'available')`,
			bookID, num,
		)
		if err != nil {
			return nil, err
		}
		cid, _ := result.LastInsertId()
		copies = append(copies, models.BookCopy{
			ID:         cid,
			BookID:     bookID,
			CopyNumber: num,
			Status:     models.StatusAvailable,
		})
	}
	return copies, nil
}

func (s *BookService) GetCopies(bookID int64) ([]models.BookCopy, error) {
	rows, err := s.db.Query(
		`SELECT id, book_id, copy_number, status, created_at, updated_at FROM book_copies WHERE book_id = ? ORDER BY copy_number`,
		bookID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var copies []models.BookCopy
	for rows.Next() {
		var c models.BookCopy
		if err := rows.Scan(&c.ID, &c.BookID, &c.CopyNumber, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		copies = append(copies, c)
	}
	return copies, nil
}

func (s *BookService) UpdateCopyStatus(copyID int64, status models.CopyStatus) error {
	result, err := s.db.Exec(
		`UPDATE book_copies SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), time.Now(), copyID,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrCopyNotFound
	}
	return nil
}
