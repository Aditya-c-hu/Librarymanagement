# Library Book Management System API

A REST API for a college library management system built with Go. Librarians manage books and inventory, students check out and return books, and the system handles reservations when books are unavailable — with fine calculation for late returns and a FIFO reservation queue.

## Tech Stack

- **Language:** Go 1.22+
- **Router:** [chi](https://github.com/go-chi/chi) (lightweight, idiomatic)
- **Database:** SQLite (via `mattn/go-sqlite3`)
- **Auth:** JWT (`golang-jwt/jwt/v5`) + bcrypt password hashing
- **No ORM** — raw SQL for full control and transparency

## Getting Started

### Prerequisites

- Go 1.22 or later
- GCC (required by `go-sqlite3` CGO dependency)

### Install & Run

```bash
git clone https://github.com/Aditya-c-hu/Librarymanagement.git
cd library-api

# Download dependencies
go mod tidy

# Run the server
go run main.go
```

The server starts on `http://localhost:8080` by default.

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `SERVER_ADDR` | `:8080` | Server listen address |
| `DATABASE_PATH` | `library.db` | SQLite database file path |
| `JWT_SECRET` | `change-me-in-production-please` | Secret key for JWT signing |

### Build

```bash
go build -o library-api main.go
./library-api
```

## Database Schema

```
users            books              book_copies
├── id           ├── id             ├── id
├── email        ├── title          ├── book_id → books.id
├── password     ├── author         ├── copy_number
├── name         ├── isbn           ├── status (available/checked_out/reserved/lost/maintenance)
├── role         ├── publisher      ├── created_at
├── created_at   ├── published_year └── updated_at
└── updated_at   ├── genre
                 ├── description    checkouts
                 ├── created_at     ├── id
                 └── updated_at     ├── user_id → users.id
                                    ├── copy_id → book_copies.id
reservations                        ├── checked_out_at
├── id                              ├── due_date
├── user_id → users.id              ├── returned_at
├── book_id → books.id              ├── fine_amount
├── status (pending/fulfilled/...)  ├── fine_paid
├── queue_pos                       ├── renewals
├── created_at                      └── created_at
├── fulfilled_at
├── expires_at
└── copy_id → book_copies.id
```

## API Documentation

### Authentication

#### Register
```
POST /api/auth/register
```
```json
{
  "email": "student@college.edu",
  "password": "secret123",
  "name": "Ravi Desai",
  "role": "student"
}
```
Response `201`: `{ "token": "jwt...", "user": { ... } }`

#### Login
```
POST /api/auth/login
```
```json
{
  "email": "student@college.edu",
  "password": "secret123"
}
```
Response `200`: `{ "token": "jwt...", "user": { ... } }`

#### Get Profile
```
GET /api/auth/me
Authorization: Bearer <token>
```
Response `200`: `{ "id": 1, "email": "...", "name": "...", "role": "student" }`

---

### Books (Public Read, Librarian Write)

#### List Books
```
GET /api/books?page=1&per_page=20&search=algorithms
```
Response `200`: Paginated list with `total_copies` and `available_copies` per book.

#### Get Book
```
GET /api/books/:id
```
Response `200`: Book detail with copy counts.

#### Create Book (Librarian)
```
POST /api/books
Authorization: Bearer <librarian_token>
```
```json
{
  "title": "Introduction to Algorithms",
  "author": "Cormen, Leiserson, Rivest, Stein",
  "isbn": "978-0262033848",
  "publisher": "MIT Press",
  "published_year": 2009,
  "genre": "Computer Science",
  "description": "The bible of algorithms.",
  "copies": 3
}
```
Response `201`: Created book with copies.

#### Update Book (Librarian)
```
PUT /api/books/:id
Authorization: Bearer <librarian_token>
```
Partial update — only include the fields you want to change.

#### Delete Book (Librarian)
```
DELETE /api/books/:id
Authorization: Bearer <librarian_token>
```

#### Add Copies (Librarian)
```
POST /api/books/:id/copies
Authorization: Bearer <librarian_token>
```
```json
{ "count": 2 }
```

#### List Copies (Librarian)
```
GET /api/books/:id/copies
Authorization: Bearer <librarian_token>
```

#### Update Copy Status (Librarian)
```
PATCH /api/books/:id/copies/:copyId
Authorization: Bearer <librarian_token>
```
```json
{ "status": "maintenance" }
```

---

### Checkouts

#### Checkout a Book
```
POST /api/checkouts
Authorization: Bearer <token>
```
```json
{ "book_id": 1 }
```
Response `201`: Checkout detail with due date.
Response `409`: If no copies available — suggests placing a reservation.

#### Return a Book
```
POST /api/checkouts/:id/return
Authorization: Bearer <token>
```
Response `200`:
```json
{
  "checkout": { ... },
  "fine_amount": 2.50,
  "days_late": 5,
  "reservation_queued": true
}
```

#### Renew a Checkout
```
POST /api/checkouts/:id/renew
Authorization: Bearer <token>
```
Extends due date by the loan period. Max 2 renewals. Blocked if overdue or book has pending reservations.

#### My Checkouts
```
GET /api/checkouts/me?active=true
Authorization: Bearer <token>
```

#### All Checkouts (Librarian)
```
GET /api/checkouts?active=true
Authorization: Bearer <librarian_token>
```

#### Overdue Books (Librarian)
```
GET /api/checkouts/overdue
Authorization: Bearer <librarian_token>
```

#### Pay Fine (Librarian)
```
POST /api/checkouts/:id/pay-fine
Authorization: Bearer <librarian_token>
```

---

### Reservations

#### Reserve a Book
```
POST /api/reservations
Authorization: Bearer <token>
```
```json
{ "book_id": 1 }
```
Response `201`: Reservation with queue position.

#### Cancel Reservation
```
DELETE /api/reservations/:id
Authorization: Bearer <token>
```

#### My Reservations
```
GET /api/reservations/me
Authorization: Bearer <token>
```

#### All Reservations (Librarian)
```
GET /api/reservations
Authorization: Bearer <librarian_token>
```

#### Book's Reservation Queue (Librarian)
```
GET /api/books/:id/reservations
Authorization: Bearer <librarian_token>
```

---

### Health Check
```
GET /health
```
Response `200`: `{ "status": "ok" }`

## Business Rules

| Rule | Detail |
|---|---|
| Loan period | 14 days |
| Late fine | $0.50 per day |
| Max renewals | 2 per checkout |
| Renewal blocked | When overdue or book has pending reservations |
| Reservation TTL | 48 hours after fulfilled (before it expires) |
| Queue order | FIFO — first to reserve gets first available copy |
| Duplicate checkout | A student cannot check out a book they already have |
| Duplicate reservation | A student cannot have multiple active reservations for the same book |

## Project Structure

```
├── main.go                          # Entry point, router setup
├── go.mod
├── internal/
│   ├── config/config.go             # Environment & app configuration
│   ├── database/
│   │   ├── database.go              # SQLite connection
│   │   └── migrations.go            # Schema creation
│   ├── models/                      # Request/response structs
│   │   ├── user.go
│   │   ├── book.go
│   │   ├── checkout.go
│   │   ├── reservation.go
│   │   └── common.go
│   ├── services/                    # Business logic
│   │   ├── auth.go
│   │   ├── book.go
│   │   ├── checkout.go              # Checkout/return/fine logic
│   │   └── reservation.go           # Reservation queue logic
│   ├── middleware/auth.go           # JWT auth & role middleware
│   └── handlers/                    # HTTP handlers
│       ├── auth.go
│       ├── books.go
│       ├── checkouts.go
│       ├── reservations.go
│       └── helpers.go
├── DESIGN.md                        # Architecture & concurrency doc
├── README.md
└── .gitignore
```

## License

MIT
