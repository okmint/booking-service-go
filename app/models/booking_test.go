package models_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"booking-service/app/models"
)

func TestNewBooking_Success(t *testing.T) {
	// Arrange
	userID := int64(1)
	resourceID := int64(10)
	startDate := time.Now().AddDate(0, 0, 7)
	endDate := time.Now().AddDate(0, 0, 14)

	// Act
	booking, err := models.NewBooking(userID, resourceID, startDate, endDate)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusAwaitsConfirmation, booking.Status())
	assert.Equal(t, userID, booking.UserID())
	assert.Equal(t, resourceID, booking.ResourceID())
}

func TestNewBooking_InvalidUserID(t *testing.T) {
	_, err := models.NewBooking(0, 10, time.Now(), time.Now().AddDate(0, 0, 1))
	assert.ErrorIs(t, err, models.ErrInvalidUserID)
}

func TestNewBooking_EndDateBeforeStartDate(t *testing.T) {
	start := time.Now().AddDate(0, 0, 7)
	end := time.Now().AddDate(0, 0, 1)
	_, err := models.NewBooking(1, 10, start, end)
	assert.ErrorIs(t, err, models.ErrEndDateBeforeStartDate)
}

func TestConfirm_FromAwaitsConfirmation(t *testing.T) {
	booking := createTestBooking(t)

	err := booking.Confirm()

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusConfirmed, booking.Status())
}

func TestConfirm_FromConfirmed_Error(t *testing.T) {
	booking := createTestBooking(t)
	_ = booking.Confirm()

	err := booking.Confirm()

	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func TestInitiateCancellation_FromAwaitsConfirmation(t *testing.T) {
	booking := createTestBooking(t)
	err := booking.InitiateCancellation(time.Now())
	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusCancellationPending, booking.Status())
	require.NotNil(t, booking.PreviousStatus())
	assert.Equal(t, models.BookingStatusAwaitsConfirmation, *booking.PreviousStatus())
	assert.NotNil(t, booking.CancelCommandSentAt())
}

func TestInitiateCancellation_FromConfirmed_FutureStartDate(t *testing.T) {
	booking := createTestBooking(t)
	_ = booking.Confirm()
	today := time.Now()
	err := booking.InitiateCancellation(today)
	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusCancellationPending, booking.Status())
	require.NotNil(t, booking.PreviousStatus())
	assert.Equal(t, models.BookingStatusConfirmed, *booking.PreviousStatus())
}

func TestInitiateCancellation_FromConfirmed_PastStartDate_Error(t *testing.T) {
	b := models.RestoreBooking(
		1,
		models.BookingStatusConfirmed,
		1, 10,
		time.Now().AddDate(0, 0, -3),
		time.Now().AddDate(0, 0, -1),
		time.Now().AddDate(0, 0, -5),
		nil, nil,
	)
	err := b.InitiateCancellation(time.Now())
	assert.ErrorIs(t, err, models.ErrCannotCancelPastBooking)
}

func TestInitiateCancellation_FromCancelled_Error(t *testing.T) {
	booking := createTestBooking(t)
	_ = booking.InitiateCancellation(time.Now())
	_ = booking.CompleteCancellation()
	err := booking.InitiateCancellation(time.Now())
	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func TestCompleteCancellation_Success(t *testing.T) {
	booking := createTestBooking(t)
	_ = booking.InitiateCancellation(time.Now())
	err := booking.CompleteCancellation()
	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusCancelled, booking.Status())
	assert.Nil(t, booking.PreviousStatus())
	assert.Nil(t, booking.CancelCommandSentAt())
}

func TestCompleteCancellation_FromInvalidStatus_Error(t *testing.T) {
	booking := createTestBooking(t)
	err := booking.CompleteCancellation()
	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func TestRollbackCancellation_Success(t *testing.T) {
	booking := createTestBooking(t)
	_ = booking.Confirm()
	_ = booking.InitiateCancellation(time.Now())
	err := booking.RollbackCancellation()
	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusConfirmed, booking.Status())
	assert.Nil(t, booking.PreviousStatus())
	assert.Nil(t, booking.CancelCommandSentAt())
}

func TestRollbackCancellation_FromInvalidStatus_Error(t *testing.T) {
	booking := createTestBooking(t)
	err := booking.RollbackCancellation()
	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func createTestBooking(t *testing.T) *models.Booking {
	t.Helper()
	b, err := models.NewBooking(1, 10, time.Now().AddDate(0, 0, 7), time.Now().AddDate(0, 0, 14))
	require.NoError(t, err)
	return b
}
