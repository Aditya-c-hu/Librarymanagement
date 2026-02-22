package database

import "database/sql"

func runMigrations(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		email       TEXT    NOT NULL UNIQUE,
		password    TEXT    NOT NULL,
		name        TEXT    NOT NULL,
		role        TEXT    NOT NULL CHECK(role IN ('librarian','student')),
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS books (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		title           TEXT    NOT NULL,
		author          TEXT    NOT NULL,
		isbn            TEXT    NOT NULL UNIQUE,
		publisher       TEXT    NOT NULL DEFAULT '',
		published_year  INTEGER NOT NULL DEFAULT 0,
		genre           TEXT    NOT NULL DEFAULT '',
		description     TEXT    NOT NULL DEFAULT '',
		created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS book_copies (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		book_id     INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
		copy_number INTEGER NOT NULL,
		status      TEXT    NOT NULL DEFAULT 'available'
		                    CHECK(status IN ('available','checked_out','reserved','lost','maintenance')),
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(book_id, copy_number)
	);

	CREATE TABLE IF NOT EXISTS checkouts (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id        INTEGER  NOT NULL REFERENCES users(id),
		copy_id        INTEGER  NOT NULL REFERENCES book_copies(id),
		checked_out_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		due_date       DATETIME NOT NULL,
		returned_at    DATETIME,
		fine_amount    REAL     NOT NULL DEFAULT 0,
		fine_paid      INTEGER  NOT NULL DEFAULT 0,
		renewals       INTEGER  NOT NULL DEFAULT 0,
		created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS reservations (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id      INTEGER  NOT NULL REFERENCES users(id),
		book_id      INTEGER  NOT NULL REFERENCES books(id),
		status       TEXT     NOT NULL DEFAULT 'pending'
		                      CHECK(status IN ('pending','fulfilled','cancelled','expired')),
		queue_pos    INTEGER  NOT NULL DEFAULT 0,
		created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
		fulfilled_at DATETIME,
		expires_at   DATETIME,
		copy_id      INTEGER  REFERENCES book_copies(id)
	);

	CREATE INDEX IF NOT EXISTS idx_checkouts_user    ON checkouts(user_id);
	CREATE INDEX IF NOT EXISTS idx_checkouts_copy    ON checkouts(copy_id);
	CREATE INDEX IF NOT EXISTS idx_reservations_book ON reservations(book_id, status, queue_pos);
	CREATE INDEX IF NOT EXISTS idx_reservations_user ON reservations(user_id, status);
	CREATE INDEX IF NOT EXISTS idx_book_copies_book  ON book_copies(book_id, status);
	`

	_, err := db.Exec(schema)
	return err
}
