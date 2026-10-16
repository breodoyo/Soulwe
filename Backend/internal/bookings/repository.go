package bookings

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Create(ctx context.Context, b *Booking) error

	GetByID(ctx context.Context, userID, bookingID string) (*Booking, error)

	ListByUserID(ctx context.Context, userID string) ([]Booking, error)

	HasActiveOverlap(ctx context.Context, userID string, start time.Time) (bool, error)

	TherapistStatus(ctx context.Context, therapistID string) (found, active bool, err error)

	SetStatus(ctx context.Context, userID, bookingID, expected, next string) (*Booking, error)
}

// PostgresRepository implements Repository on top of the shared pgx pool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const bookingColumns = `b.id, b.user_id, t.id, t.full_name, b.scheduled_at, b.status, b.created_at, b.updated_at`

const (
	createBookingSQL = `
		WITH new_booking AS (
			INSERT INTO bookings (user_id, therapist_id, scheduled_at, status)
			VALUES ($1, $2, $3, $4)
			RETURNING *
		)
		SELECT ` + bookingColumns + `
		FROM new_booking b
		JOIN therapists t ON t.id = b.therapist_id`

	listBookingsSQL = `
		SELECT ` + bookingColumns + `
		FROM bookings b
		JOIN therapists t ON t.id = b.therapist_id
		WHERE b.user_id = $1
		ORDER BY b.created_at DESC, b.id DESC`

	getBookingSQL = `
		SELECT ` + bookingColumns + `
		FROM bookings b
		JOIN therapists t ON t.id = b.therapist_id
		WHERE b.id = $2 AND b.user_id = $1`
	hasActiveOverlapSQL = `
		SELECT EXISTS (
			SELECT 1 FROM bookings
			WHERE user_id = $1
			  AND status IN ('pending', 'confirmed')
			  AND scheduled_at < $2
			  AND scheduled_at + INTERVAL '60 minutes' > $3
		)`

	therapistStatusSQL = `SELECT is_active FROM therapists WHERE id = $1`

	setBookingStatusSQL = `
		UPDATE bookings SET status = $4, updated_at = NOW()
		WHERE id = $2 AND user_id = $1 AND status = $3
		RETURNING id, updated_at`
)

func (r *PostgresRepository) Create(ctx context.Context, b *Booking) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		err := scanBooking(r.pool.QueryRow(ctx, createBookingSQL,
			b.UserID, b.TherapistID, b.ScheduledAt, b.Status,
		).Scan, b)
		switch {
		case err == nil:
			return nil
		case isConflict(err):
			return ErrBookingConflict
		case isDeadlock(err) && attempt < 2:
			lastErr = err
			continue
		default:
			return fmt.Errorf("bookings create: %w", err)
		}
	}
	return fmt.Errorf("bookings create: %w", lastErr)
}

func (r *PostgresRepository) GetByID(ctx context.Context, userID, bookingID string) (*Booking, error) {
	b := &Booking{}
	err := scanBooking(r.pool.QueryRow(ctx, getBookingSQL, userID, bookingID).Scan, b)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBookingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("bookings get: %w", err)
	}
	return b, nil
}

func (r *PostgresRepository) ListByUserID(ctx context.Context, userID string) ([]Booking, error) {
	rows, err := r.pool.Query(ctx, listBookingsSQL, userID)
	if err != nil {
		return nil, fmt.Errorf("bookings list: %w", err)
	}
	defer rows.Close()

	bookings := make([]Booking, 0)
	for rows.Next() {
		var b Booking
		if err := scanBooking(rows.Scan, &b); err != nil {
			return nil, fmt.Errorf("bookings list: scan: %w", err)
		}
		bookings = append(bookings, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("bookings list: %w", err)
	}
	return bookings, nil
}

func (r *PostgresRepository) HasActiveOverlap(ctx context.Context, userID string, start time.Time) (bool, error) {
	var overlaps bool
	err := r.pool.QueryRow(ctx, hasActiveOverlapSQL, userID,
		start.Add(SessionDuration), start).Scan(&overlaps)
	if err != nil {
		return false, fmt.Errorf("bookings overlap: %w", err)
	}
	return overlaps, nil
}

func (r *PostgresRepository) TherapistStatus(ctx context.Context, therapistID string) (found, active bool, err error) {
	var isActive bool
	err = r.pool.QueryRow(ctx, therapistStatusSQL, therapistID).Scan(&isActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("bookings therapist status: %w", err)
	}
	return true, isActive, nil
}

func (r *PostgresRepository) SetStatus(ctx context.Context, userID, bookingID, expected, next string) (*Booking, error) {
	var refreshed struct {
		id        string
		updatedAt time.Time
	}
	err := r.pool.QueryRow(ctx, setBookingStatusSQL, userID, bookingID, expected, next).
		Scan(&refreshed.id, &refreshed.updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, getErr := r.GetByID(ctx, userID, bookingID); errors.Is(getErr, ErrBookingNotFound) {
			return nil, ErrBookingNotFound
		}
		return nil, ErrBookingStatusConflict
	}
	if err != nil {
		return nil, fmt.Errorf("bookings set status: %w", err)
	}

	b, err := r.GetByID(ctx, userID, bookingID)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func isConflict(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == pgerrcode.UniqueViolation || pgErr.Code == pgerrcode.ExclusionViolation
}

func isDeadlock(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.DeadlockDetected
}

// scanBooking shares one column decoder between list and get.
func scanBooking(s func(dest ...any) error, b *Booking) error {
	return s(&b.ID, &b.UserID, &b.TherapistID, &b.DisplayName,
		&b.ScheduledAt, &b.Status, &b.CreatedAt, &b.UpdatedAt)
}
