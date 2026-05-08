package booking

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type bookingCompactRow struct {
	id      string
	oldPos  int
	blocked bool
	avail   int
}

func nextFreeQueuePos(used map[int]bool, start int) int {
	p := start
	for used[p] {
		p++
	}
	return p
}

// compactQueuePositionsBookingTx renumbers queue_position for one destination after a booking:
// - Garage-blocked rows keep their current positions (reserved).
// - Rows with available_seats > 0 take the smallest free integers in stable (old position) order.
// - Full rows (not blocked, no seats) fill remaining free integers in stable order (usually tail).
func (r *RepositoryImpl) compactQueuePositionsBookingTx(ctx context.Context, tx pgx.Tx, destinationID string) error {
	rows, err := tx.Query(ctx, `
		SELECT id, queue_position, COALESCE(is_garage_blocked, false), available_seats
		FROM vehicle_queue
		WHERE destination_id = $1
		ORDER BY queue_position ASC
		FOR UPDATE`, destinationID)
	if err != nil {
		return err
	}

	var list []bookingCompactRow
	for rows.Next() {
		var rw bookingCompactRow
		if scanErr := rows.Scan(&rw.id, &rw.oldPos, &rw.blocked, &rw.avail); scanErr != nil {
			rows.Close()
			return scanErr
		}
		list = append(list, rw)
	}
	rows.Close()

	used := make(map[int]bool)
	var eligible, full []*bookingCompactRow
	for i := range list {
		rw := &list[i]
		if rw.blocked {
			used[rw.oldPos] = true
			continue
		}
		if rw.avail > 0 {
			eligible = append(eligible, rw)
			continue
		}
		full = append(full, rw)
	}

	for _, rw := range eligible {
		np := nextFreeQueuePos(used, 1)
		used[np] = true
		if np != rw.oldPos {
			if _, err := tx.Exec(ctx, `UPDATE vehicle_queue SET queue_position = $2 WHERE id = $1`, rw.id, np); err != nil {
				return err
			}
		}
	}
	for _, rw := range full {
		np := nextFreeQueuePos(used, 1)
		used[np] = true
		if np != rw.oldPos {
			if _, err := tx.Exec(ctx, `UPDATE vehicle_queue SET queue_position = $2 WHERE id = $1`, rw.id, np); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *RepositoryImpl) applyBookingQueueServingAndCompactTx(ctx context.Context, tx pgx.Tx, destinationID, bookedQueueEntryID string, availAfter int) error {
	if availAfter > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO queue_destination_booking_state (destination_id, serving_queue_entry_id)
			VALUES ($1, $2)
			ON CONFLICT (destination_id) DO UPDATE SET serving_queue_entry_id = EXCLUDED.serving_queue_entry_id`,
			destinationID, bookedQueueEntryID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE queue_destination_booking_state SET serving_queue_entry_id = NULL WHERE destination_id = $1`,
			destinationID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE vehicle_queue SET prioritize_after_blocked_unblock = FALSE WHERE id = $1`, bookedQueueEntryID); err != nil {
		return err
	}

	return r.compactQueuePositionsBookingTx(ctx, tx, destinationID)
}
