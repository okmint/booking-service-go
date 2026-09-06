package postgres

const (
	queryInsertBooking = `
       INSERT INTO bookings (status, user_id, resource_id, start_date, end_date, created_at)
       VALUES ($1, $2, $3, $4, $5, $6)
       RETURNING id`

	queryGetBookingByID = `
       SELECT id, status, user_id, resource_id, start_date, end_date, created_at, previous_status, cancel_command_sent_at
       FROM bookings
       WHERE id = $1`

	queryUpdateBookingStatus = `
       UPDATE bookings
       SET status = $1, 
           previous_status = $2, 
           cancel_command_sent_at = $3
       WHERE id = $4`

	queryGetBookingsByFilter = `
       SELECT id, status, user_id, resource_id, start_date, end_date, created_at, previous_status, cancel_command_sent_at
       FROM bookings
       WHERE ($1::BIGINT IS NULL OR user_id = $1)
         AND ($2::BIGINT IS NULL OR resource_id = $2)
         AND ($3::VARCHAR IS NULL OR status = $3)
       ORDER BY id DESC
       LIMIT $4 OFFSET $5`

	queryCountBookingsByFilter = `
       SELECT COUNT(*)
       FROM bookings
       WHERE ($1::BIGINT IS NULL OR user_id = $1)
         AND ($2::BIGINT IS NULL OR resource_id = $2)
         AND ($3::VARCHAR IS NULL OR status = $3)`

	queryGetAwaitingConfirmation = `
       SELECT id, status, user_id, resource_id, start_date, end_date, created_at, previous_status, cancel_command_sent_at
       FROM bookings
       WHERE status = 'awaits_confirmation'
       ORDER BY created_at ASC
       LIMIT $1
       FOR UPDATE SKIP LOCKED`

	queryGetStatisticsByStatus = `
       SELECT status, COUNT(*)
       FROM bookings
       WHERE created_at >= $1 AND created_at < ($2::date + INTERVAL '1 day')
       GROUP BY status`

	queryGetStatisticsTopResources = `
       SELECT resource_id, COUNT(*)
       FROM bookings
       WHERE created_at >= $1 AND created_at < ($2::date + INTERVAL '1 day')
       GROUP BY resource_id
       ORDER BY COUNT(*) DESC
       LIMIT 5`

	queryGetStuckCancellations = `
       SELECT id, status, user_id, resource_id, start_date, end_date, created_at, previous_status, cancel_command_sent_at
       FROM bookings
       WHERE status = 'cancellation_pending' 
         AND cancel_command_sent_at < $1
       ORDER BY cancel_command_sent_at ASC
       LIMIT $2`

	queryInsertHistory = `
       INSERT INTO booking_history (booking_id, previous_status, new_status, initiator, reason)
       VALUES ($1, $2, $3, $4, $5)`

	queryCountHistory = `
       SELECT COUNT(*) 
       FROM booking_history 
       WHERE booking_id = $1`

	queryGetHistory = `
       SELECT id, booking_id, previous_status, new_status, initiator, reason, created_at
       FROM booking_history
       WHERE booking_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`

	queryInsertProcessedEvent = `
       INSERT INTO processed_events (event_id)
       VALUES ($1)`

	queryCheckProcessedEvent = `
       SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id = $1)`
)
