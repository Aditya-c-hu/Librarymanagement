# Design Document: Reservation Queue & Concurrent Checkout Handling

## 1. Overview

This document explains the two most complex aspects of the Library Book Management System:

1. **Reservation Queue** — how students reserve books when no copies are available, and how the queue is processed.
2. **Concurrent Checkout Handling** — how the system prevents race conditions when multiple students try to check out the last copy simultaneously.

---

## 2. Reservation Queue Design

### 2.1 Queue Model

Reservations form a **FIFO (First-In, First-Out) queue** per book. Each reservation has:

- `queue_pos`: integer position in the queue (1 = next in line)
- `status`: one of `pending`, `fulfilled`, `cancelled`, `expired`
- `copy_id`: assigned when fulfilled (which specific copy is held for the student)
- `expires_at`: deadline for the student to pick up the book after fulfillment

### 2.2 Lifecycle

```
Student requests reservation
        │
        ▼
   ┌─────────┐
   │ PENDING  │  queue_pos = N (next available position)
   └────┬─────┘
        │
        │  Book returned → copy assigned to position 1
        ▼
  ┌───────────┐
  │ FULFILLED │  copy_id set, expires_at = now + 48h
  └─────┬─────┘
        │
   ┌────┴────┐
   │         │
   ▼         ▼
CHECKED OUT  EXPIRED (student didn't pick up in time)
   │              │
   │              ▼
   │         Next pending reservation is fulfilled
   │         (or copy returns to 'available')
   ▼
RETURNED → cycle continues
```

### 2.3 Queue Position Management

When a reservation is **created**:
- `queue_pos = MAX(queue_pos for this book's pending reservations) + 1`

When a reservation is **cancelled**:
- All pending reservations with `queue_pos > cancelled.queue_pos` are decremented by 1
- This maintains a contiguous sequence with no gaps

When a book is **returned**:
- The system queries the pending reservation with the **lowest** `queue_pos` for this book
- If found: that reservation becomes `fulfilled`, the returned copy's status changes to `reserved`, and a 48-hour pickup window starts
- If not found: the copy status returns to `available`

### 2.4 Expiration Handling

A background goroutine runs every 5 minutes to check for fulfilled reservations past their `expires_at`:

1. Mark the reservation as `expired`
2. Check if there's another pending reservation for the same book
3. If yes: assign the same copy to the next person in the queue
4. If no: release the copy back to `available`

This cascading behavior ensures the queue always progresses even if students don't pick up their reserved books.

### 2.5 Checkout with Reservation

When a student checks out a book:

1. The system first checks if the student has a **fulfilled** (not expired) reservation for that book
2. If yes: the assigned `copy_id` from the reservation is used, and the reservation is marked `cancelled` (consumed)
3. If no: the system looks for any `available` copy
4. If no copies at all: returns an error suggesting the student place a reservation

This design ensures that reserved copies cannot be "stolen" by walk-in checkouts.

---

## 3. Concurrent Checkout Handling

### 3.1 The Problem

Consider: Book "Algorithms" has 1 available copy. Students A and B both click "Checkout" at the same time.

Without protection:
1. Both read the copy as `available`
2. Both create checkout records
3. One physical copy, two logical owners — a data integrity violation

### 3.2 Solution: SQLite Serialized Transactions

We use SQLite's **single-writer model** combined with **database transactions** to guarantee atomicity:

```go
db.SetMaxOpenConns(1)  // Only one connection at a time
```

The checkout flow runs inside a transaction:

```
BEGIN TRANSACTION
  1. SELECT available copy (WHERE status = 'available' LIMIT 1)
  2. UPDATE copy SET status = 'checked_out'
  3. INSERT INTO checkouts
COMMIT
```

Because SQLite allows only **one writer at a time** (enforced by `_busy_timeout=5000` and single max connection), concurrent checkout attempts are **serialized**:

- Student A's transaction runs first, finds the copy, marks it `checked_out`
- Student B's transaction runs next, finds **no available copies**, gets `ErrNoCopiesAvailable`
- Student B is told to place a reservation instead

### 3.3 Why This Works

| Property | How It's Achieved |
|---|---|
| **Atomicity** | SQL transaction — all steps succeed or all are rolled back |
| **Isolation** | SQLite single-writer ensures sequential execution |
| **No phantom reads** | Copy status is checked and updated in the same transaction |
| **No double-checkout** | We check `user_id + book_id` for active checkouts before proceeding |

### 3.4 Busy Timeout

The connection string includes `_busy_timeout=5000` (5 seconds). If a transaction is waiting for a lock, SQLite will retry for up to 5 seconds before returning a `BUSY` error. This handles transient contention gracefully.

### 3.5 WAL Mode

We enable Write-Ahead Logging (`_journal_mode=WAL`), which allows **concurrent readers** while a write is in progress. This means:

- Students browsing the book catalog are never blocked by a checkout in progress
- Only write operations (checkout, return, reserve) are serialized

### 3.6 Trade-offs and Scaling

**Current design (SQLite) is appropriate for:**
- College library with hundreds of concurrent users
- Single-server deployments
- Simplicity and zero-ops database

**For higher scale (thousands of concurrent checkouts per second), migrate to PostgreSQL:**
- Replace `_busy_timeout` with `SELECT ... FOR UPDATE SKIP LOCKED`
- Use row-level locking instead of database-level
- Connection pooling with multiple concurrent writers
- The service layer abstracts the database, so the migration would only affect `database/` and SQL queries

---

## 4. Fine Calculation

Fines are calculated at return time, not accrued continuously:

```
fine_amount = ceil((now - due_date) / 24 hours) × $0.50/day
```

- If returned on time: `fine_amount = 0`
- If returned 1 hour late: 1 day × $0.50 = $0.50
- If returned 3 days, 2 hours late: 4 days × $0.50 = $2.00

Fines are stored on the checkout record and must be paid separately by a librarian (`POST /checkouts/:id/pay-fine`).

---

## 5. Renewal Rules

A student can renew (extend the due date) under these conditions:

1. The book is not yet overdue
2. The student has not already renewed the maximum number of times (default: 2)
3. There are no pending reservations for this book

Condition #3 prevents a student from indefinitely holding a book that others are waiting for.

---

## 6. Security Model

| Layer | Mechanism |
|---|---|
| Authentication | JWT tokens (HS256, 24h expiry) |
| Password storage | bcrypt hash |
| Authorization | Role-based middleware (`librarian` / `student`) |
| Data isolation | Students can only see/modify their own checkouts and reservations |
| Librarian access | Full read/write on all resources |

---

## 7. API Error Handling

All errors return consistent JSON:

```json
{
  "error": "human-readable error message"
}
```

HTTP status codes follow REST conventions:
- `400` — bad request / validation error
- `401` — missing or invalid token
- `403` — insufficient role
- `404` — resource not found
- `409` — conflict (duplicate checkout, no copies, etc.)
- `500` — internal server error
